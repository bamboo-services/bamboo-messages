package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Chat 流式对话。
//
// 无系统提示的流式对话，内部调用 ChatWithSystem 并传入空 systemPrompt。
// 返回 StreamEvent channel，按需消费流式输出。
func (p *ResponsesProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 将统一 provider.Message 转换为 OpenAI Responses 协议格式，
// 通过 HTTPClient 发起流式请求（SSE），返回 StreamEvent channel。
//
// 去 SDK 化实现：使用 httpClient.DoWithDebug 发送 HTTP 请求，
// 通过 provider.SSEScanner 解析 SSE 帧并分发到 handleStreamEvent，
// 不依赖 openai-go SDK。
func (p *ResponsesProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		if config == nil {
			config = &provider.ChatConfig{}
		}

		// 构建请求参数（map[string]any）
		params := p.buildParams(config.Model, systemPrompt, messages, config, true)

		body, err := json.Marshal(params)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses 请求参数序列化失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		// 发送 HTTP 请求（含 debug 日志）
		resp, err := p.httpClient.DoWithDebug(ctx, "POST", "/responses", body, "openai-responses", "POST /responses (streaming, model="+config.Model+")")
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses 流式对话请求失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		// 检查 HTTP 状态码，错误响应直接读取 body 并返回错误事件
		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			var apiErr openaiError
			_ = json.Unmarshal(respBody, &apiErr)
			errMsg := "OpenAI Responses"
			if apiErr.Error.Message != "" {
				errMsg += ": " + apiErr.Error.Message
			}

			select {
			case eventCh <- provider.StreamEvent{
				Type:       provider.StreamTypeError,
				Err:        pkgErrors.NewBambooError("上游", errMsg, resp.StatusCode),
				StatusCode: resp.StatusCode,
			}:
			case <-ctx.Done():
				return
			}

			select {
			case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
			case <-ctx.Done():
			}
			return
		}

		// 使用 SSEScanner 解析 SSE 流
		scanner := provider.NewSSEScanner(resp.Body)
		provider.DebugSSEResponse("openai-responses", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header))
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = scanner.Close()
			_ = resp.Body.Close()
		}()

		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false
		stopSent := false

		for {
			eventType, data, done, scanErr := scanner.Next()
			provider.DebugSSEFrame("openai-responses", eventType, data)
			if done {
				break
			}
			if scanErr != nil {
				// io.EOF 表示流正常耗尽，不作为错误处理
				if scanErr == io.EOF {
					break
				}
				// 已有内容时优雅降级：后续 Done → converter 发出完整终止序列。
				if startSent {
					break
				}
				select {
				case eventCh <- provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses SSE 流读取失败: %v", scanErr), 0),
				}:
				case <-ctx.Done():
					return
				}
				break
			}

			// 解析 SSE 帧数据为 responseStreamEvent
			var event responseStreamEvent
			if jsonErr := json.Unmarshal(data, &event); jsonErr != nil {
				// JSON 解析失败：跳过该帧，继续读取（容错）
				continue
			}

			// 如果 event: 行有类型但 JSON 中无 type 字段，用 eventType 补充
			if event.Type == "" && eventType != "" {
				event.Type = eventType
			}

			// 首次成功解析后发送 StreamTypeStart。若首帧是 response.created，
			// 把上游真实 response.id 挂到 Start 上，供出口 codec 原样使用，
			// 避免伪造 resp_<nano> 打断 Grok previous_response_id 链路。
			if !startSent {
				startSent = true
				startEv := provider.StreamEvent{Type: provider.StreamTypeStart}
				if event.Type == "response.created" && event.Response != nil && event.Response.ID != "" {
					startEv.Delta = provider.NewMetadataDelta(event.Response.ID, "", "")
				}
				select {
				case eventCh <- startEv:
				case <-ctx.Done():
					return
				}
			}

			// 分发事件到处理函数
			events := p.handleStreamEvent(ctx, event, &textBlockStarted, &thinkingBlockStarted)
			for _, e := range events {
				if e.Type == provider.StreamTypeStop {
					stopSent = true
				}
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		// 流中断但未收到 response.completed/incomplete，补发 Stop 携带降级完成原因
		if !stopSent && startSent {
			finishReason := provider.ResolveDegradedReason(
				p.degradedReason,
				config != nil && len(config.Tools) > 0,
			)
			select {
			case eventCh <- provider.StreamEvent{
				Type:         provider.StreamTypeStop,
				FinishReason: finishReason,
			}:
			case <-ctx.Done():
				return
			}
		}

		// 如果因 ctx 取消等原因未发送过 Start，补发一个 Start 以保证 channel 语义完整
		if !startSent {
			select {
			case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
			case <-ctx.Done():
				return
			}
		}

		// 发送 StreamTypeDone 结束流
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

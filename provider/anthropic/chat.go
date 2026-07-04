package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Chat 流式对话。
//
// 无系统提示的流式对话，内部调用 ChatWithSystem 并传入空 systemPrompt。
// 返回 StreamEvent channel，按需消费流式输出。
func (p *Provider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 将统一 provider.Message 转换为 Anthropic 协议格式，
// 通过 httpClient 发起流式 HTTP 请求，使用 SSEScanner 解析 SSE 事件流，
// 返回 StreamEvent channel。不依赖 anthropic-sdk-go。
//
// 支持系统提示、温度、TopP、Stop 序列、工具调用、Thinking 配置、TopK、ToolChoice 等参数。
func (p *Provider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 构建请求参数
		params := p.buildParams(systemPrompt, messages, config)
		params.Stream = true

		body, err := json.Marshal(params)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("Anthropic 请求参数序列化失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		endpoint := "POST /v1/messages (streaming, model=" + config.Model + ")"
		resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, "/v1/messages", body, "anthropic", endpoint)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("Anthropic 流式对话请求失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		// HTTP 状态码检查：>= 400 时读取 body 解析错误
		if resp.StatusCode >= 400 {
			errBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			select {
			case eventCh <- provider.StreamEvent{
				Type:       provider.StreamTypeError,
				Err:        pkgErrors.NewBambooError("上游", formatAnthropicError(resp.StatusCode, errBody), resp.StatusCode),
				StatusCode: resp.StatusCode,
			}:
			case <-ctx.Done():
			}
			return
		}

		// 创建 SSE 扫描器
		scanner := provider.NewSSEScanner(resp.Body)
		provider.DebugSSEResponse("anthropic", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header))
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = scanner.Close()
			_ = resp.Body.Close()
		}()

		// 追踪完成原因，从 message_delta 提取，供 message_stop 使用
		var finishReason provider.FinishReason
		startSent := false

		// SSE 事件循环 — Anthropic SSE 有 event: 行，eventType 由 scanner 返回
		for {
			eventType, data, done, scanErr := scanner.Next()
			provider.DebugSSEFrame("anthropic", eventType, data)
			if done {
				break
			}
			if scanErr != nil {
				// io.EOF 表示流正常耗尽，不视为错误
				if errors.Is(scanErr, io.EOF) {
					break
				}
				select {
				case eventCh <- provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("Anthropic 流读取错误: %v", scanErr), 0),
				}:
				case <-ctx.Done():
					return
				}
				break
			}

			// 发送 Start 事件（首个有效帧时）
			if !startSent {
				startSent = true
				select {
				case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
				case <-ctx.Done():
					return
				}
			}

			// 构建 messageStreamEvent 并交给 handleStreamEvent 分发
			// Anthropic SSE 的 data 行包含完整的事件 JSON（含 type 字段），
			// eventType 来自 event: 行，两者一致
			event := messageStreamEvent{
				Type:  eventType,
				Delta: data,
			}

			// 对于有额外字段的事件，需要从 data 中解析
			// message_start: data 包含 message 对象
			// content_block_start: data 包含 index 和 content_block
			// content_block_stop: data 包含 index
			// content_block_delta: data 包含 index 和 delta
			// message_delta: data 包含 delta
			// error: data 包含 error 对象
			if len(data) > 0 {
				// 使用临时结构解析外层字段，保留 Delta 为原始 JSON
				var raw struct {
					Type         string           `json:"type"`
					Index        *int             `json:"index,omitempty"`
					ContentBlock *contentBlock    `json:"content_block,omitempty"`
					Message      *messageResponse `json:"message,omitempty"`
					Error        *anthropicError  `json:"error,omitempty"`
				}
				if jsonErr := json.Unmarshal(data, &raw); jsonErr == nil {
					event.Index = raw.Index
					event.ContentBlock = raw.ContentBlock
					event.Message = raw.Message
					event.Error = raw.Error
					// 对于 content_block_delta 和 message_delta，Delta 字段需要是 delta 子对象
					// 但 handleStreamEvent 中 contentBlockDelta 会重新从 event.Delta 解析 contentBlockDelta
					// 所以这里需要提取 delta 子对象
					if eventType == "content_block_delta" || eventType == "message_delta" {
						var wrapper struct {
							Delta json.RawMessage `json:"delta,omitempty"`
						}
						if jsonErr := json.Unmarshal(data, &wrapper); jsonErr == nil && len(wrapper.Delta) > 0 {
							event.Delta = wrapper.Delta
						}
					}
				}
			}

			events := p.handleStreamEvent(event, &finishReason)
			for _, e := range events {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		// 发送 Done 事件
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

// formatAnthropicError 格式化 Anthropic API 错误响应为可读字符串。
//
// 尝试解析 Anthropic 标准错误响应结构，提取 error.message 字段；
// 解析失败时返回原始响应体摘要。
func formatAnthropicError(statusCode int, body []byte) string {
	var errResp anthropicErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		return fmt.Sprintf("Anthropic API 错误 [%d]: %s", statusCode, errResp.Error.Message)
	}
	// 截断过长的响应体
	bodyStr := string(body)
	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200] + "..."
	}
	return fmt.Sprintf("Anthropic API 返回错误状态码 %d: %s", statusCode, bodyStr)
}

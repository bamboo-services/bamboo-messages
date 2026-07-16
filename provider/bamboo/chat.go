package bamboo

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
// 将统一 provider.Message 转换为 bamboo 原生协议格式，
// 通过 httpClient 发起流式 HTTP 请求，使用 SSEScanner 解析 SSE 事件流，
// 返回 StreamEvent channel。
func (p *Provider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 构建请求参数（buildParams 不设置 Stream，由 chat.go 设置）
		params := buildParams(systemPrompt, messages, config)
		params.Stream = true

		body, err := json.Marshal(params)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("bamboo 请求参数序列化失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		endpoint := "POST /v1/bamboo (streaming, model=" + config.Model + ")"
		resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, "/v1/bamboo", body, "bamboo", endpoint)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("bamboo 流式对话请求失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		// HTTP >= 400 时读取 body 解析错误
		if resp.StatusCode >= 400 {
			errBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			select {
			case eventCh <- provider.StreamEvent{
				Type:       provider.StreamTypeError,
				Err:        pkgErrors.NewBambooError("上游", formatBambooError(resp.StatusCode, errBody), resp.StatusCode),
				StatusCode: resp.StatusCode,
			}:
			case <-ctx.Done():
			}
			return
		}

		// 创建 SSE 扫描器
		scanner := provider.NewSSEScanner(resp.Body)
		provider.DebugSSEResponse("bamboo", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header))
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = scanner.Close()
			_ = resp.Body.Close()
		}()

		// 追踪完成原因
		var finishReason provider.FinishReason
		startSent := false

		// SSE 事件循环
		for {
			eventType, data, done, scanErr := scanner.Next()
			provider.DebugSSEFrame("bamboo", eventType, data)
			if done {
				break
			}
			if scanErr != nil {
				if errors.Is(scanErr, io.EOF) {
					break
				}
				// 已有内容时优雅降级
				if startSent {
					break
				}
				select {
				case eventCh <- provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("bamboo 流读取错误: %v", scanErr), 0),
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

			events := p.handleStreamEvent(eventType, data, &finishReason)
			for _, e := range events {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		// 降级场景：未收到 message_stop（上游中断），补发 Stop 事件
		if finishReason == "" && startSent {
			finishReason = provider.FinishReasonStop
			select {
			case eventCh <- provider.StreamEvent{
				Type:         provider.StreamTypeStop,
				FinishReason: finishReason,
			}:
			case <-ctx.Done():
				return
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

// formatBambooError 格式化 bamboo API 错误响应为可读字符串。
//
// 尝试解析 bamboo 错误响应结构，提取 error.message 字段；
// 解析失败时返回原始响应体摘要。
func formatBambooError(statusCode int, body []byte) string {
	var errResp wireErrorPayload
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return fmt.Sprintf("bamboo: %s", errResp.Error.Message)
	}
	bodyStr := string(body)
	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200] + "..."
	}
	return fmt.Sprintf("bamboo: %s", bodyStr)
}

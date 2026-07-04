package gemini

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
// 将统一 provider.Message 转换为 Gemini 协议格式，
// 通过 httpClient 直接调用 Gemini REST API（streamGenerateContent?alt=sse 端点），
// 返回 StreamEvent channel。不依赖 genai SDK。
//
// 支持系统提示、温度、TopP、MaxTokens、Stop 序列、工具调用、Thinking 配置、ToolChoice 等参数。
func (p *Provider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		if config == nil {
			config = &provider.ChatConfig{}
		}

		// 构建请求体
		reqBody := p.buildRequestBody(messages, systemPrompt, config, true)
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("Gemini 流式对话请求序列化失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		// Gemini 流式端点：/v1beta/models/{model}:streamGenerateContent?alt=sse
		endpoint := fmt.Sprintf("/v1beta/models/%s:streamGenerateContent?alt=sse", config.Model)

		resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, endpoint, bodyBytes, "gemini", endpoint)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("Gemini 流式对话请求失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		// HTTP 状态码检查：>= 400 时读取 body 解析错误
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			select {
			case eventCh <- provider.StreamEvent{
				Type:       provider.StreamTypeError,
				Err:        pkgErrors.NewBambooError("上游", formatGeminiError(resp.StatusCode, body), resp.StatusCode),
				StatusCode: resp.StatusCode,
			}:
			case <-ctx.Done():
			}
			return
		}

		// 创建 SSE 扫描器
		scanner := provider.NewSSEScanner(resp.Body)
		provider.DebugSSEResponse("gemini", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header))
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = scanner.Close()
			_ = resp.Body.Close()
		}()

		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false
		stopSent := false

		// SSE 事件循环
		for {
			eventType, data, done, scanErr := scanner.Next()
			provider.DebugSSEFrame("gemini", eventType, data)
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
					Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("Gemini 流读取错误: %v", scanErr), 0),
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

			// 反序列化 Gemini generateContentResponse
			var geminiResp generateContentResponse
			if jsonErr := json.Unmarshal(data, &geminiResp); jsonErr != nil {
				// 跳过无法解析的帧（SSEScanner 已做 json.Valid 校验，此处为业务层兜底）
				continue
			}

			// 处理响应 → 事件
			events := p.handleStreamEvent(&geminiResp, &textBlockStarted, &thinkingBlockStarted)
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

		// 流正常结束但未收到 FinishReason，补发 Stop 事件
		if !stopSent {
			select {
			case eventCh <- provider.StreamEvent{
				Type:         provider.StreamTypeStop,
				FinishReason: provider.FinishReasonStop,
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

// formatGeminiError 格式化 Gemini API 错误响应为可读消息。
//
// 尝试解析 Gemini 错误响应结构（{error: {code, message, status}}），
// 解析失败时回退为原始 HTTP 状态码 + body。
func formatGeminiError(statusCode int, body []byte) string {
	var errResp geminiErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil && errResp.Error.Message != "" {
		return fmt.Sprintf("Gemini: %s", errResp.Error.Message)
	}
	return fmt.Sprintf("Gemini: %s", string(body))
}

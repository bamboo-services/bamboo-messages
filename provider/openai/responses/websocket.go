package responses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/pkg/ws"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// isWebSocketMode 判断当前请求是否应走 WebSocket 传输（wss://.../v1/responses）。
//
// 触发条件（满足其一）：
//  1. 构造 Provider 时启用了 WithWebSocketMode(true)
//  2. BaseURL 以 ws:// 或 wss:// 开头
//  3. 请求级 ProviderExtra["websocket_mode"] 为 true
func (p *ResponsesProvider) isWebSocketMode(config *provider.ChatConfig) bool {
	if p.websocketMode {
		return true
	}
	base := strings.ToLower(strings.TrimSpace(p.httpClient.GetBaseURL()))
	if strings.HasPrefix(base, "ws://") || strings.HasPrefix(base, "wss://") {
		return true
	}
	if config != nil {
		if v, ok := provider.GetExtraBool(config.ProviderExtra, "websocket_mode"); ok && v {
			return true
		}
	}
	return false
}

// buildWebSocketPayload 构建 Responses WebSocket 模式的客户端事件负载（type: "response.create"）。
//
// 根据 OpenAI 官方规范：
//   - 顶层必须设置 `"type": "response.create"`
//   - WebSocket 模式下 `stream` 隐式生效，必须省略 `stream` 与 `background` 字段
//   - 支持可选的 `stream_id`、`generate`、`context_management` 等字段
func (p *ResponsesProvider) buildWebSocketPayload(model, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) map[string]any {
	params := p.buildParams(model, systemPrompt, messages, config, false)
	delete(params, "stream")
	delete(params, "background")
	params["type"] = "response.create"

	if config != nil {
		if streamID, ok := provider.GetExtraString(config.ProviderExtra, "stream_id"); ok && streamID != "" {
			params["stream_id"] = streamID
		}
		if gen, ok := provider.GetExtraBool(config.ProviderExtra, "generate"); ok {
			params["generate"] = gen
		}
		if cm, ok := provider.GetExtraAny(config.ProviderExtra, "context_management"); ok && cm != nil {
			params["context_management"] = cm
		}
	}
	return params
}

// chatViaWebSocket 通过 WebSocket 持久连接协议向上游发送请求并返回流事件 channel。
//
// 核心逻辑：
//  1. 将 messages 与 config 参数转为官方 {"type":"response.create", ...} 格式，强制移除 stream 与 background 字段；
//  2. 运行请求拦截器链（如果有注册），支持对序列化后的 JSON 负载进行自定义改写；
//  3. 发起 [ws.Dial] 连接目标服务，记录 RequestSentAt 与 ResponseHeaderAt 物理时间锚点；
//  4. 循环读取服务端返回的事件帧，针对 error、response.created、response.output_item.* 等事件转换为对应的 [provider.StreamEvent]；
//  5. 若首帧遇到连接或认证级错误，直接作为带有精准 HTTP 状态码的 StreamTypeError 发出并提前终止。
func (p *ResponsesProvider) chatViaWebSocket(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		if config == nil {
			config = &provider.ChatConfig{}
		}

		payloadMap := p.buildWebSocketPayload(config.Model, systemPrompt, messages, config)
		body, err := json.Marshal(payloadMap)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses WebSocket 请求序列化失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		if interceptors := p.httpClient.Interceptors(); len(interceptors) > 0 {
			body, err = provider.ApplyInterceptors(ctx, body, interceptors)
			if err != nil {
				select {
				case eventCh <- provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses WebSocket 请求拦截器执行失败: %v", err), 0),
				}:
				case <-ctx.Done():
				}
				return
			}
		}

		wsURL := p.httpClient.BuildURL("/responses")
		headers := p.httpClient.BuildHeaders()
		provider.DebugRequest("openai-responses-ws", "WS "+wsURL+" (model="+config.Model+")", headers, json.RawMessage(body))

		requestSentAt := time.Now()
		conn, err := ws.Dial(ctx, wsURL, headers)
		responseHeaderAt := time.Now()
		if err != nil {
			statusCode := 0
			errMsg := fmt.Sprintf("OpenAI Responses WebSocket 连接失败: %v", err)
			var hsErr *ws.HandshakeError
			if errors.As(err, &hsErr) {
				statusCode = hsErr.StatusCode
				errMsg = formatResponsesError(hsErr.StatusCode, hsErr.Body)
			}
			select {
			case eventCh <- provider.StreamEvent{
				Type:       provider.StreamTypeError,
				Err:        pkgErrors.NewBambooError("上游", errMsg, statusCode),
				StatusCode: statusCode,
				ReceivedAt: time.Now(),
			}:
			case <-ctx.Done():
			}
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		// 监听 ctx 取消，及时关闭底层连接解除 ReadMessage 阻塞
		doneGuard := make(chan struct{})
		defer close(doneGuard)
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-doneGuard:
			}
		}()

		if err := conn.WriteText(body); err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type:       provider.StreamTypeError,
				Err:        pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses WebSocket 发送请求失败: %v", err), 0),
				ReceivedAt: time.Now(),
			}:
			case <-ctx.Done():
			}
			return
		}

		timingAnchor := &provider.TimingAnchor{
			RequestSentAt:    requestSentAt,
			ResponseHeaderAt: responseHeaderAt,
		}

		wantStreamID, _ := provider.GetExtraString(config.ProviderExtra, "stream_id")
		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false
		stopSent := false

		for {
			_, data, readErr := conn.ReadMessage()
			frameAt := time.Now()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) || stopSent {
					break
				}
				if startSent {
					break
				}
				select {
				case eventCh <- provider.StreamEvent{
					Type:       provider.StreamTypeError,
					Err:        pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses WebSocket 读取失败: %v", readErr), 0),
					ReceivedAt: frameAt,
				}:
				case <-ctx.Done():
					return
				}
				break
			}

			provider.DebugSSEFrame("openai-responses-ws", "ws_message", data)

			// 检查 stream_id 过滤（多路复用下若指定了 stream_id，忽略其他通道事件）
			if wantStreamID != "" {
				var env struct {
					StreamID string `json:"stream_id"`
				}
				if json.Unmarshal(data, &env) == nil && env.StreamID != "" && env.StreamID != wantStreamID {
					continue
				}
			}

			var event responseStreamEvent
			if jsonErr := json.Unmarshal(data, &event); jsonErr != nil {
				continue
			}

			// 若首帧即为 error 或 response.failed，且尚未发送 Start，直接发送 StreamTypeError 并退出，
			// 使上层 bamboo.Chat 首事件 peek 机制能同步捕获并返回错误！
			if !startSent && (event.Type == "error" || event.Type == "response.failed") {
				errEvents := p.handleStreamEvent(ctx, event, &textBlockStarted, &thinkingBlockStarted)
				for _, e := range errEvents {
					e.ReceivedAt = frameAt
					select {
					case eventCh <- e:
					case <-ctx.Done():
						return
					}
				}
				return
			}

			if !startSent {
				startSent = true
				startEv := provider.StreamEvent{Type: provider.StreamTypeStart, Timing: timingAnchor, ReceivedAt: frameAt}
				if event.Type == "response.created" && event.Response != nil && event.Response.ID != "" {
					startEv.Delta = provider.NewMetadataDelta(event.Response.ID, "", "")
				}
				select {
				case eventCh <- startEv:
				case <-ctx.Done():
					return
				}
			}

			events := p.handleStreamEvent(ctx, event, &textBlockStarted, &thinkingBlockStarted)
			hasError := false
			for _, e := range events {
				if e.Type == provider.StreamTypeStop {
					stopSent = true
				}
				if e.Type == provider.StreamTypeError {
					hasError = true
				}
				e.ReceivedAt = frameAt
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}

			// WebSocket 是持久连接，收到本轮的终止事件（completed / incomplete / failed / error）后即结束本轮读取
			if stopSent || hasError || event.Type == "response.completed" || event.Type == "response.incomplete" || event.Type == "response.failed" || event.Type == "error" {
				break
			}
		}

		if !stopSent && startSent {
			finishReason := provider.ResolveDegradedReason(
				p.degradedReason,
				config != nil && len(config.Tools) > 0,
			)
			select {
			case eventCh <- provider.StreamEvent{
				Type:         provider.StreamTypeStop,
				FinishReason: finishReason,
				ReceivedAt:   time.Now(),
			}:
			case <-ctx.Done():
				return
			}
		}

		if !startSent {
			select {
			case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart, Timing: timingAnchor, ReceivedAt: time.Now()}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone, ReceivedAt: time.Now()}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

// completeViaWebSocket 在 WebSocket 模式下通过聚合流事件完成同步 Complete 调用。
//
// 启动后台 WebSocket 流式读取任务，并在当前 goroutine 中同步遍历事件 channel，
// 将文本增量、思维过程、加密签名、工具调用参数以及 Token 用量聚合为完整的 [*provider.CompletionResult]。
// 遇到上游错误事件时立即返回结构化的 [*pkgErrors.BambooError]。
func (p *ResponsesProvider) completeViaWebSocket(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	ch := p.chatViaWebSocket(ctx, systemPrompt, messages, config)

	res := &provider.CompletionResult{
		FinishReason: provider.FinishReasonStop,
	}
	var currentTool *provider.ToolCall

	for ev := range ch {
		switch ev.Type {
		case provider.StreamTypeStart:
			if ev.Delta.Type == provider.StreamDeltaTypeMetadata {
				if md, ok := ev.Delta.Data.(provider.MetadataData); ok && md.ResponseID != "" {
					res.ResponseID = md.ResponseID
				}
			}
		case provider.StreamTypeDelta:
			switch ev.Delta.Type {
			case provider.StreamDeltaTypeTextOutput:
				if td, ok := ev.Delta.Data.(provider.TextData); ok {
					res.Content += string(td)
				}
			case provider.StreamDeltaTypeThinking:
				if th, ok := ev.Delta.Data.(provider.ThinkingData); ok {
					res.Thinking += string(th)
				}
			case provider.StreamDeltaTypeSignature:
				if sig, ok := ev.Delta.Data.(provider.SignatureData); ok && sig != "" {
					res.ThinkingSignature = string(sig)
					res.ThinkingSignatureProvider = provider.SignatureProviderOpenAIResponses
				}
			case provider.StreamDeltaTypeMetadata:
				if md, ok := ev.Delta.Data.(provider.MetadataData); ok {
					if md.ResponseID != "" {
						res.ResponseID = md.ResponseID
					}
					if md.ReasoningID != "" {
						res.ReasoningID = md.ReasoningID
					}
					if md.EncryptedContent != "" {
						res.ThinkingSignature = md.EncryptedContent
						res.ThinkingSignatureProvider = provider.SignatureProviderOpenAIResponses
					}
				}
			case provider.StreamDeltaTypeToolCall:
				if tc, ok := ev.Delta.Data.(provider.ToolCallData); ok {
					res.ToolCalls = append(res.ToolCalls, provider.ToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: provider.FunctionCall{
							Name: tc.Name,
						},
					})
					currentTool = &res.ToolCalls[len(res.ToolCalls)-1]
				}
			case provider.StreamDeltaTypeToolCallDelta:
				if currentTool != nil {
					if args, ok := ev.Delta.Data.(provider.ToolCallDeltaData); ok {
						currentTool.Function.Arguments += string(args)
					}
				}
			case provider.StreamDeltaTypeUsage:
				if u, ok := ev.Delta.Data.(provider.UsageData); ok {
					res.Usage = u
				}
			}
		case provider.StreamTypeStop:
			if ev.FinishReason != "" {
				res.FinishReason = ev.FinishReason
			}
		case provider.StreamTypeError:
			if ev.Err != nil {
				return nil, ev.Err
			}
			return nil, pkgErrors.NewBambooError("上游", "OpenAI Responses WebSocket 未知错误", ev.StatusCode)
		}
	}

	if len(res.ToolCalls) > 0 && res.FinishReason == provider.FinishReasonStop {
		res.FinishReason = provider.FinishReasonToolCalls
	}
	return res, nil
}

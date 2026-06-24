package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3/responses"
)

// ==============================
// 流式事件处理
// ==============================

// handleStreamEvent 根据事件类型分发到对应的处理方法。
//
// 接收 OpenAI Responses SSE 事件，根据事件类型调用对应的处理函数，
// 返回统一格式的 StreamEvent 列表。
func (p *ResponsesProvider) handleStreamEvent(ctx context.Context, event responses.ResponseStreamEventUnion, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	switch event.Type {
	case "response.created":
		return p.contentResponseCreated(event)
	case "response.output_item.added":
		return p.contentOutputItemAdded(event)
	case "response.output_text.delta":
		return p.contentOutputTextDelta(event, textBlockStarted)
	case "response.reasoning_text.delta":
		return p.contentReasoningTextDelta(event, thinkingBlockStarted)
	case "response.function_call_arguments.delta":
		return p.contentFunctionCallDelta(event)
	case "response.function_call_arguments.done":
		return p.contentFunctionCallDone(event)
	case "response.completed":
		return p.contentResponseCompleted(event)
	case "response.failed":
		return p.contentResponseFailed(ctx, event)
	case "response.incomplete":
		return p.contentResponseIncomplete(event)
	default:
		return nil
	}
}

// contentResponseCreated 处理响应创建事件。
//
// OpenAI Responses 响应创建时触发，已在 ChatWithSystem 中发送 StreamTypeStart，
// 此处无需额外处理。
func (p *ResponsesProvider) contentResponseCreated(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 响应创建，已在 ChatWithSystem 中发送 StreamTypeStart
	return nil
}

// contentOutputItemAdded 处理输出项添加事件。
//
// 当新输出项（如 function_call）被添加到响应时触发，
// 返回对应工具调用开始的 StreamEvent。
func (p *ResponsesProvider) contentOutputItemAdded(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseOutputItemAdded()
	switch e.Item.Type {
	case "function_call":
		callID := e.Item.CallID
		if callID == "" {
			callID = e.Item.ID
		}
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(callID, e.Item.Name),
		}}
	default:
		return nil
	}
}

// contentOutputTextDelta 处理文本输出增量事件。
//
// 处理普通文本增量，首次文本增量时合成 BlockStart 事件，
// 确保与 Anthropic 协议的一致性。
func (p *ResponsesProvider) contentOutputTextDelta(event responses.ResponseStreamEventUnion, textBlockStarted *bool) []provider.StreamEvent {
	e := event.AsResponseOutputTextDelta()

	// OpenAI Responses 没有明确的 content_block_start 事件，在第一次文本增量前合成 BlockStart
	if !*textBlockStarted {
		*textBlockStarted = true
		return []provider.StreamEvent{
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("text"),
			},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewTextDelta(e.Delta),
			},
		}
	}

	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta(e.Delta),
	}}
}

// contentReasoningTextDelta 处理推理文本增量事件。
//
// 处理 Reasoning/Thinking 过程的文本增量，首次推理增量时合成 BlockStart 事件，
// 确保流式事件的一致性。
func (p *ResponsesProvider) contentReasoningTextDelta(event responses.ResponseStreamEventUnion, thinkingBlockStarted *bool) []provider.StreamEvent {
	e := event.AsResponseReasoningTextDelta()

	// 推理文本也需要在第一次增量前合成 BlockStart
	if !*thinkingBlockStarted {
		*thinkingBlockStarted = true
		return []provider.StreamEvent{
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("thinking"),
			},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewThinkingDelta(e.Delta),
			},
		}
	}

	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewThinkingDelta(e.Delta),
	}}
}

// contentFunctionCallDelta 处理函数调用参数增量事件。
//
// 处理工具调用参数的流式增量，返回包含参数片段的 StreamEvent。
func (p *ResponsesProvider) contentFunctionCallDelta(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseFunctionCallArgumentsDelta()
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaData(e.Delta),
	}}
}

// contentFunctionCallDone 处理函数调用完成事件。
//
// 当工具调用参数传输完成时触发，OpenAI Responses 协议中此事件无需特殊处理。
func (p *ResponsesProvider) contentFunctionCallDone(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 函数调用完成，无需特殊处理
	return nil
}

// contentResponseCompleted 处理响应完成事件（包含 usage 和 stop 事件）。
//
// 当 OpenAI 响应完成时触发，提取 Token 用量信息并返回 UsageDelta 和 StreamTypeStop 事件。
func (p *ResponsesProvider) contentResponseCompleted(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseCompleted()
	usage := e.Response.Usage
	var events []provider.StreamEvent

	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				usage.InputTokens,
				usage.OutputTokens,
				0,
				usage.InputTokensDetails.CachedTokens,
			),
		})
	}

	// 发送 StreamTypeStop 并携带完成原因
	events = append(events, provider.StreamEvent{
		Type:         provider.StreamTypeStop,
		FinishReason: mapResponseFinishReason(e.Response),
	})

	return events
}

// contentResponseFailed 处理响应失败事件。
//
// 当 OpenAI 请求失败时触发，包装错误信息并返回 ErrorEvent。
func (p *ResponsesProvider) contentResponseFailed(ctx context.Context, event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseFailed()
	usage := e.Response.Usage
	var events []provider.StreamEvent

	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				usage.InputTokens,
				usage.OutputTokens,
				0,
				usage.InputTokensDetails.CachedTokens,
			),
		})
	}

	errMsg := "OpenAI 响应失败"
	if e.Response.Error.Message != "" {
		errMsg += ": " + e.Response.Error.Message
	}
	events = append(events, provider.StreamEvent{
		Type: provider.StreamTypeError,
		Err:  xError.NewError(ctx, nil, errMsg, false, nil),
	})

	return events
}

// contentResponseIncomplete 处理响应未完成事件。
//
// 当响应因长度限制等原因未完成时触发，发送停止事件结束流。
func (p *ResponsesProvider) contentResponseIncomplete(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseIncomplete()
	usage := e.Response.Usage
	var events []provider.StreamEvent

	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				usage.InputTokens,
				usage.OutputTokens,
				0,
				usage.InputTokensDetails.CachedTokens,
			),
		})
	}

	// 响应未完成，发送停止事件并携带完成原因
	events = append(events, provider.StreamEvent{
		Type:         provider.StreamTypeStop,
		FinishReason: mapResponseFinishReason(e.Response),
	})

	return events
}

// mapResponseFinishReason 根据 OpenAI Responses 状态和输出推断完成原因。
//
// incomplete 状态：有 function_call 输出时为 ToolCalls，否则为 Length；
// 其他状态：有 function_call 输出时为 ToolCalls，否则为 Stop。
func mapResponseFinishReason(response responses.Response) provider.FinishReason {
	hasToolCalls := false
	for _, item := range response.Output {
		if item.Type == "function_call" {
			hasToolCalls = true
			break
		}
	}

	if response.Status == responses.ResponseStatusIncomplete {
		if hasToolCalls {
			return provider.FinishReasonToolCalls
		}
		return provider.FinishReasonLength
	}

	if hasToolCalls {
		return provider.FinishReasonToolCalls
	}
	return provider.FinishReasonStop
}

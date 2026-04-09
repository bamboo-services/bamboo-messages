package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 流式事件处理
// ==============================

// handleStreamEvent 根据事件类型分发到对应的处理方法
func (p *ResponsesProvider) handleStreamEvent(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	switch event.Type {
	case "response.created":
		return p.contentResponseCreated(event)
	case "response.output_item.added":
		return p.contentOutputItemAdded(event)
	case "response.output_text.delta":
		return p.contentOutputTextDelta(event)
	case "response.reasoning_text.delta":
		return p.contentReasoningTextDelta(event)
	case "response.function_call_arguments.delta":
		return p.contentFunctionCallDelta(event)
	case "response.function_call_arguments.done":
		return p.contentFunctionCallDone(event)
	case "response.completed":
		return p.contentResponseCompleted(event)
	case "response.failed":
		return p.contentResponseFailed(event)
	case "response.incomplete":
		return p.contentResponseIncomplete(event)
	default:
		return nil
	}
}

// contentResponseCreated 处理响应创建事件
func (p *ResponsesProvider) contentResponseCreated(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 响应创建，已在 ChatWithSystem 中发送 StreamTypeStart
	return nil
}

// contentOutputItemAdded 处理输出项添加事件
func (p *ResponsesProvider) contentOutputItemAdded(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseOutputItemAdded()
	switch e.Item.Type {
	case "function_call":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(e.Item.ID, e.Item.Name),
		}}
	default:
		return nil
	}
}

// contentOutputTextDelta 处理文本输出增量事件
func (p *ResponsesProvider) contentOutputTextDelta(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseOutputTextDelta()
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta(e.Delta),
	}}
}

// contentReasoningTextDelta 处理推理文本增量事件
func (p *ResponsesProvider) contentReasoningTextDelta(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseReasoningTextDelta()
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewThinkingDelta(e.Delta),
	}}
}

// contentFunctionCallDelta 处理函数调用参数增量事件
func (p *ResponsesProvider) contentFunctionCallDelta(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseFunctionCallArgumentsDelta()
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaData(e.Delta),
	}}
}

// contentFunctionCallDone 处理函数调用完成事件
func (p *ResponsesProvider) contentFunctionCallDone(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 函数调用完成，无需特殊处理
	return nil
}

// contentResponseCompleted 处理响应完成事件（包含 usage）
func (p *ResponsesProvider) contentResponseCompleted(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseCompleted()
	usage := e.Response.Usage
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewUsageDelta(usage.InputTokens, usage.OutputTokens),
		}}
	}
	return nil
}

// contentResponseFailed 处理响应失败事件
func (p *ResponsesProvider) contentResponseFailed(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseFailed()
	errMsg := "OpenAI 响应失败"
	if e.Response.Error.Message != "" {
		errMsg += ": " + e.Response.Error.Message
	}
	return []provider.StreamEvent{{
		Type: provider.StreamTypeError,
		Err:  xError.NewError(context.TODO(), xError.OperationFailed, xError.ErrMessage(errMsg), false, nil),
	}}
}

// contentResponseIncomplete 处理响应未完成事件
func (p *ResponsesProvider) contentResponseIncomplete(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 响应未完成，发送停止事件
	return []provider.StreamEvent{{
		Type: provider.StreamTypeStop,
	}}
}

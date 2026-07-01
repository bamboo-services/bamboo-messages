package anthropic

import (
	"context"
	"encoding/json"

	"github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// handleStreamEvent 根据事件类型分发到对应的处理方法。
//
// 将 Anthropic SSE 事件类型映射到内部处理函数：
// message_start → contentMessageStart, content_block_start → contentBlockStart 等。
// finishReason 用于跨事件追踪完成原因（message_delta 提取，message_stop 使用）。
func (p *Provider) handleStreamEvent(event messageStreamEvent, finishReason *provider.FinishReason) []provider.StreamEvent {
	switch event.Type {
	case "message_start":
		return p.contentMessageStart(event)
	case "content_block_start":
		return p.contentBlockStart(event)
	case "content_block_delta":
		return p.contentBlockDelta(event)
	case "content_block_stop":
		return p.contentBlockStop(event)
	case "message_delta":
		return p.contentMessageDelta(event, finishReason)
	case "message_stop":
		return p.contentMessageStop(finishReason)
	case "ping":
		// 心跳事件，跳过
		return nil
	case "error":
		return p.contentError(event)
	default:
		return nil
	}
}

// contentMessageStart 处理消息开始事件。
//
// Anthropic message_start 事件携带 message.usage.input_tokens（此时 output_tokens=0），
// 发送 NewUsageDelta 将 input_tokens 传递给上层。
func (p *Provider) contentMessageStart(event messageStreamEvent) []provider.StreamEvent {
	// message_start 事件已在 ChatWithSystem 中发送 StreamTypeStart，此处仅提取 usage
	if event.Message != nil && event.Message.Usage != nil {
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewUsageDelta(int64(event.Message.Usage.InputTokens), 0),
		}}
	}
	return nil	// 消息开始，无需特殊处理，已在 ChatWithSystem 中发送 StreamTypeStart

}

// contentBlockStart 处理内容块开始事件。
//
// 根据内容块类型发出对应的 BlockStart delta：
// text → NewBlockStartDelta("text"), thinking → NewBlockStartDelta("thinking") + NewThinkingDelta, tool_use → NewToolCallDelta。
func (p *Provider) contentBlockStart(event messageStreamEvent) []provider.StreamEvent {
	if event.ContentBlock == nil {
		return nil
	}
	block := event.ContentBlock
	switch block.Type {
	case "text":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStartDelta("text"),
		}}
	case "thinking":
		events := []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStartDelta("thinking"),
		}}
		if block.Thinking != "" {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewThinkingDelta(block.Thinking),
			})
		}
		return events
	case "tool_use":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStartDeltaWithID("tool_use", block.ID, block.Name),
		}}
	default:
		return nil
	}
}

// contentBlockDelta 处理内容块增量事件。
//
// 根据增量类型发出对应的 StreamDelta：
// text_delta → NewTextDelta, thinking_delta → NewThinkingDelta, input_json_delta → NewToolCallDeltaData, signature_delta → NewSignatureDelta。
func (p *Provider) contentBlockDelta(event messageStreamEvent) []provider.StreamEvent {
	if len(event.Delta) == 0 {
		return nil
	}
	var delta contentBlockDelta
	if err := json.Unmarshal(event.Delta, &delta); err != nil {
		return nil
	}
	switch delta.Type {
	case "text_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta(delta.Text),
		}}
	case "thinking_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(delta.Thinking),
		}}
	case "input_json_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDeltaData(delta.PartialJSON),
		}}
	case "signature_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewSignatureDelta(delta.Signature),
		}}
	default:
		return nil
	}
}

// contentBlockStop 处理内容块结束事件。
//
// 将 Anthropic content_block_stop 事件透传为 BlockStop delta，
// 携带内容块索引，供下游组件识别内容块边界。
func (p *Provider) contentBlockStop(event messageStreamEvent) []provider.StreamEvent {
	if event.Index == nil {
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStopDeltaNoIndex(),
		}}
	}
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewBlockStopDelta(*event.Index),
	}}
}

// contentMessageDelta 处理消息增量事件（包含 usage 和 stop_reason）。
//
// Anthropic message_delta 事件携带最终的 Token 用量统计（output_tokens 及 cache 字段）
// 和停止原因。发送 NewUsageDeltaWithCache 并提取 stop_reason 供后续 contentMessageStop 使用。
func (p *Provider) contentMessageDelta(event messageStreamEvent, finishReason *provider.FinishReason) []provider.StreamEvent {
	if len(event.Delta) == 0 {
		return nil
	}
	var msgDelta messageDeltaData
	if err := json.Unmarshal(event.Delta, &msgDelta); err != nil {
		return nil
	}

	// 提取 stop_reason 供 contentMessageStop 使用
	if msgDelta.StopReason != nil && *msgDelta.StopReason != "" {
		*finishReason = mapFinishReason(*msgDelta.StopReason)
	}

	// 提取 message_delta 中的最终 usage（output_tokens 及 cache 字段）
	if msgDelta.Usage != nil {
		return []provider.StreamEvent{{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				int64(msgDelta.Usage.InputTokens),
				int64(msgDelta.Usage.OutputTokens),
				int64(msgDelta.Usage.CacheCreationInputTokens),
				int64(msgDelta.Usage.CacheReadInputTokens),
			),
		}}
	}
	return nil
}

// contentMessageStop 处理消息结束事件。
//
// Anthropic message_stop 事件，发送 StreamTypeStop 并携带从 message_delta 提取的完成原因。
func (p *Provider) contentMessageStop(finishReason *provider.FinishReason) []provider.StreamEvent {
	return []provider.StreamEvent{{
		Type:         provider.StreamTypeStop,
		FinishReason: *finishReason,
	}}
}

// contentError 处理 error 事件。
//
// Anthropic error 事件，发送 StreamTypeError 并携带错误信息。
func (p *Provider) contentError(event messageStreamEvent) []provider.StreamEvent {
	if event.Error == nil {
		return []provider.StreamEvent{{Type: provider.StreamTypeError}}
	}
	return []provider.StreamEvent{{
		Type: provider.StreamTypeError,
		Err:  xError.NewError(context.Background(), nil, xError.ErrMessage("Anthropic 流式事件错误: "+event.Error.Message), false),
	}}
}

// mapFinishReason 将 Anthropic 停止原因映射为统一的 FinishReason。
//
// Anthropic: end_turn → Stop, max_tokens → Length, tool_use → ToolCalls, stop_sequence → Stop，其他默认为 Stop。
func mapFinishReason(reason string) provider.FinishReason {
	switch reason {
	case "end_turn":
		return provider.FinishReasonStop
	case "max_tokens":
		return provider.FinishReasonLength
	case "tool_use":
		return provider.FinishReasonToolCalls
	case "stop_sequence":
		return provider.FinishReasonStop
	default:
		return provider.FinishReasonStop
	}
}

package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// handleStreamEvent 根据事件类型分发到对应的处理方法。
//
// 将 Anthropic SSE 事件类型映射到内部处理函数：
// message_start → contentMessageStart, content_block_start → contentBlockStart 等。
func (p *Provider) handleStreamEvent(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
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
		return p.contentMessageDelta(event)
	case "message_stop":
		return p.contentMessageStop(event)
	default:
		return nil
	}
}

// contentMessageStart 处理消息开始事件。
//
// Anthropic message_start 事件，无需特殊处理，
// StreamTypeStart 已在 ChatWithSystem 中发送。
func (p *Provider) contentMessageStart(_ anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	// 消息开始，无需特殊处理，已在 ChatWithSystem 中发送 StreamTypeStart
	return nil
}

// contentBlockStart 处理内容块开始事件。
//
// 根据内容块类型发出对应的 BlockStart delta：
// text → NewBlockStartDelta("text"), thinking → NewThinkingDelta, tool_use → NewToolCallDelta。
func (p *Provider) contentBlockStart(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	block := event.AsContentBlockStart()
	switch block.ContentBlock.Type {
	case "text":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStartDelta("text"),
		}}
	case "thinking":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(block.ContentBlock.Thinking),
		}}
	case "tool_use":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(block.ContentBlock.ID, block.ContentBlock.Name),
		}}
	default:
		return nil
	}
}

// contentBlockDelta 处理内容块增量事件。
//
// 根据增量类型发出对应的 StreamDelta：
// text_delta → NewTextDelta, thinking_delta → NewThinkingDelta, input_json_delta → NewToolCallDeltaData。
func (p *Provider) contentBlockDelta(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	delta := event.AsContentBlockDelta()
	switch delta.Delta.Type {
	case "text_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta(delta.Delta.Text),
		}}
	case "thinking_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(delta.Delta.Thinking),
		}}
	case "input_json_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDeltaData(delta.Delta.PartialJSON),
		}}
	default:
		return nil
	}
}

// contentBlockStop 处理内容块结束事件。
//
// Anthropic content_block_stop 事件，无需特殊处理。
func (p *Provider) contentBlockStop(_ anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	// 内容块结束，无需特殊处理
	return nil
}

// contentMessageDelta 处理消息增量事件（包含 usage）。
//
// Anthropic message_delta 事件携带 Token 用量统计，
// 发送 NewUsageDelta。
func (p *Provider) contentMessageDelta(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	msgDelta := event.AsMessageDelta()
	if msgDelta.Usage.InputTokens > 0 || msgDelta.Usage.OutputTokens > 0 {
		return []provider.StreamEvent{{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				msgDelta.Usage.InputTokens,
				msgDelta.Usage.OutputTokens,
				msgDelta.Usage.CacheCreationInputTokens,
				msgDelta.Usage.CacheReadInputTokens,
			),
		}}
	}
	return nil
}

// contentMessageStop 处理消息结束事件。
//
// Anthropic message_stop 事件，发送 StreamTypeStop。
func (p *Provider) contentMessageStop(_ anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	return []provider.StreamEvent{{
		Type: provider.StreamTypeStop,
	}}
}

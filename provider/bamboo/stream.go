package bamboo

import (
	"encoding/json"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 流式事件内部 DTO
// ==============================

// streamMessageStart message_start 事件中的 message 对象（仅提取 usage）。
type streamMessageStart struct {
	Usage *wireUsage `json:"usage,omitempty"`
}

// streamContentBlockDelta content_block_delta 事件中的 delta 对象。
type streamContentBlockDelta struct {
	Type        string `json:"type"`                   // text_delta / thinking_delta / input_json_delta / signature_delta
	Text        string `json:"text,omitempty"`         // text_delta 的文本增量
	Thinking    string `json:"thinking,omitempty"`     // thinking_delta 的思考增量
	Signature   string `json:"signature,omitempty"`    // signature_delta 的签名增量
	PartialJSON string `json:"partial_json,omitempty"` // input_json_delta 的 JSON 片段
}

// streamMessageDelta message_delta 事件中的 delta 对象。
type streamMessageDelta struct {
	StopReason string     `json:"stop_reason,omitempty"` // 停止原因
	Usage      *wireUsage `json:"usage,omitempty"`       // 最终 Token 用量（output_tokens 等）
}

// ==============================
// 事件分发
// ==============================

// handleStreamEvent 根据事件类型分发到对应的处理逻辑。
//
// 将 bamboo 原生协议 SSE 事件映射为 provider.StreamEvent 列表。
// 一个 wire 事件可能产生多个 provider 事件（如 redacted_thinking 的 BlockStart + RedactedThinkingDelta）。
// finishReason 用于跨事件追踪完成原因（message_delta 提取，message_stop 使用）。
//
// 注意：
//   - message_start 不发送 StreamTypeStart（由 chat.go 通过 startSent 标志控制）
//   - ping 事件跳过（provider 无对应类型）
//   - content_block_stop 必须发出 BlockStop delta（不可返回 nil）
func (p *Provider) handleStreamEvent(eventType string, data []byte, finishReason *provider.FinishReason) []provider.StreamEvent {
	switch eventType {
	case "message_start":
		return p.handleMessageStart(data)
	case "content_block_start":
		return p.handleContentBlockStart(data)
	case "content_block_delta":
		return p.handleContentBlockDelta(data)
	case "content_block_stop":
		return p.handleContentBlockStop(data)
	case "message_delta":
		return p.handleMessageDelta(data, finishReason)
	case "message_stop":
		return p.handleMessageStop(finishReason)
	case "ping":
		return nil
	case "error":
		return p.handleError(data)
	default:
		return nil
	}
}

// handleMessageStart 处理 message_start 事件。
//
// 提取 message.usage 中的 input_tokens 和缓存统计，发送 NewUsageDeltaWithCache。
// 不发送 StreamTypeStart（由 chat.go 控制）。
func (p *Provider) handleMessageStart(data []byte) []provider.StreamEvent {
	var raw struct {
		Message *streamMessageStart `json:"message,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Message == nil || raw.Message.Usage == nil {
		return nil
	}
	u := raw.Message.Usage
	return []provider.StreamEvent{{
		Type: provider.StreamTypeDelta,
		Delta: provider.NewUsageDeltaWithCache(
			u.InputTokens,
			u.OutputTokens,
			u.CacheCreationInputTokens,
			u.CacheReadInputTokens,
		),
	}}
}

// handleContentBlockStart 处理 content_block_start 事件。
//
// 根据内容块类型发出对应的 BlockStart delta：
//   - text → NewBlockStartDelta("text")
//   - thinking → NewBlockStartDelta("thinking") + 若有初始内容则 NewThinkingDelta
//   - tool_use → NewBlockStartDeltaWithID("tool_use", id, name)
//   - redacted_thinking → NewBlockStartDelta("redacted_thinking") + 若有 data 则 NewRedactedThinkingDelta
func (p *Provider) handleContentBlockStart(data []byte) []provider.StreamEvent {
	var raw struct {
		Index        *int              `json:"index,omitempty"`
		ContentBlock *wireContentBlock `json:"content_block,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.ContentBlock == nil {
		return nil
	}
	block := raw.ContentBlock
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
	case "redacted_thinking":
		events := []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStartDelta("redacted_thinking"),
		}}
		if block.Data != "" {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewRedactedThinkingDelta(block.Data),
			})
		}
		return events
	default:
		return nil
	}
}

// handleContentBlockDelta 处理 content_block_delta 事件。
//
// 根据增量类型发出对应的 StreamDelta：
//   - text_delta → NewTextDelta
//   - thinking_delta → NewThinkingDelta
//   - input_json_delta → NewToolCallDeltaData
//   - signature_delta → NewSignatureDelta
func (p *Provider) handleContentBlockDelta(data []byte) []provider.StreamEvent {
	var raw struct {
		Delta streamContentBlockDelta `json:"delta"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	delta := raw.Delta
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

// handleContentBlockStop 处理 content_block_stop 事件。
//
// 将 bamboo content_block_stop 事件透传为 BlockStop delta，
// 携带内容块索引（如有），供下游组件识别内容块边界。
func (p *Provider) handleContentBlockStop(data []byte) []provider.StreamEvent {
	var raw struct {
		Index *int `json:"index,omitempty"`
	}
	_ = json.Unmarshal(data, &raw)
	if raw.Index == nil {
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStopDeltaNoIndex(),
		}}
	}
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewBlockStopDelta(*raw.Index),
	}}
}

// handleMessageDelta 处理 message_delta 事件。
//
// 提取 stop_reason 映射为 FinishReason（供后续 message_stop 使用），
// 若携带 usage 则发送 NewUsageDeltaWithCache。
func (p *Provider) handleMessageDelta(data []byte, finishReason *provider.FinishReason) []provider.StreamEvent {
	var raw struct {
		Delta streamMessageDelta `json:"delta"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	delta := raw.Delta
	if delta.StopReason != "" {
		*finishReason = mapBambooFinishReason(delta.StopReason)
	}
	if delta.Usage != nil {
		u := delta.Usage
		return []provider.StreamEvent{{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				u.InputTokens,
				u.OutputTokens,
				u.CacheCreationInputTokens,
				u.CacheReadInputTokens,
			),
		}}
	}
	return nil
}

// handleMessageStop 处理 message_stop 事件。
//
// 发送 StreamTypeStop 并携带从 message_delta 提取的完成原因。
func (p *Provider) handleMessageStop(finishReason *provider.FinishReason) []provider.StreamEvent {
	return []provider.StreamEvent{{
		Type:         provider.StreamTypeStop,
		FinishReason: *finishReason,
	}}
}

// handleError 处理 error 事件。
//
// 发送 StreamTypeError 并携带错误信息。
func (p *Provider) handleError(data []byte) []provider.StreamEvent {
	var errPayload wireErrorPayload
	if err := json.Unmarshal(data, &errPayload); err != nil {
		return []provider.StreamEvent{{Type: provider.StreamTypeError}}
	}
	return []provider.StreamEvent{{
		Type: provider.StreamTypeError,
		Err:  pkgErrors.NewBambooError("上游", "bamboo 流式事件错误: "+errPayload.Error.Message, 0),
	}}
}

// mapBambooFinishReason 将 bamboo 停止原因映射为统一的 FinishReason。
//
// end_turn → Stop, max_tokens → Length, tool_use → ToolCalls,
// stop_sequence → Stop, pause_turn → PauseTurn, refusal → Refusal,
// server_tool_use → ServerToolUse，其他默认为 Stop。
func mapBambooFinishReason(reason string) provider.FinishReason {
	switch reason {
	case "end_turn":
		return provider.FinishReasonStop
	case "max_tokens":
		return provider.FinishReasonLength
	case "tool_use":
		return provider.FinishReasonToolCalls
	case "stop_sequence":
		return provider.FinishReasonStop
	case "pause_turn":
		return provider.FinishReasonPauseTurn
	case "refusal":
		return provider.FinishReasonRefusal
	case "server_tool_use":
		return provider.FinishReasonServerToolUse
	default:
		return provider.FinishReasonStop
	}
}

package anthropic

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

// anthropicStreamSerializer Anthropic 流式序列化器，有状态。
//
// Bamboo 的 StreamEvent 设计本身就是 Anthropic 风格，
// 序列化几乎是 1:1 JSON 编码 + SSE event/data 行封装。
type anthropicStreamSerializer struct {
	messageID string
	model     string
	started   bool
	// startUsage 累计已知的 usage，用于填充 message_start 帧。
	//
	// Anthropic 协议把输入与缓存统计放在 message_start，而上游 Provider 通常在
	// 随后的 usage 增量（ping 事件）才给出，因此这里先缓存再回填。
	startUsage *bamboo.Usage
	// pendingStart 表示 message_start 帧尚未输出，等待 usage 就绪后回填。
	pendingStart bool
}

// newStreamSerializer 创建一个新的 Anthropic 流式序列化器实例。
func newStreamSerializer(model string) *anthropicStreamSerializer {
	return &anthropicStreamSerializer{
		model: model,
	}
}

// Serialize 将单个 StreamEvent 序列化为 Anthropic SSE 数据帧。
//
// 每条 SSE 格式: `event: {type}\ndata: {json}\n\n`
func (s *anthropicStreamSerializer) Serialize(event bamboo.StreamEvent) ([]byte, error) {
	var frame []byte
	var err error

	switch event.Type {
	case bamboo.EventMessageStart:
		return s.handleMessageStart(event)

	case bamboo.EventContentBlockStart:
		frame, err = s.handleContentBlockStart(event)

	case bamboo.EventContentBlockDelta:
		frame, err = s.handleContentBlockDelta(event)

	case bamboo.EventContentBlockStop:
		frame, err = s.handleContentBlockStop(event)

	case bamboo.EventMessageDelta:
		s.mergeUsage(event.Usage)
		frame, err = s.handleMessageDelta(event)

	case bamboo.EventMessageStop:
		frame, err = s.handleMessageStop(event)

	case bamboo.EventPing:
		// ping 事件可能携带 usage（Anthropic 上游在 message_start 阶段给出输入与缓存统计），
		// 先回填再输出，保证 message_start 帧带上真实的缓存统计。
		s.mergeUsage(event.Usage)
		frame, err = s.handlePing(event)

	case bamboo.EventError:
		frame, err = s.handleError(event)

	default:
		return nil, nil
	}

	if err != nil {
		return nil, err
	}
	return s.withPendingStart(frame)
}

// withPendingStart 在待输出的 message_start 帧前拼接当前事件帧。
//
// message_start 必须始终是流中的第一帧，因此这里做延迟输出：
// 等到第一个携带 usage 的事件（ping）到达后再回填缓存统计并输出。
func (s *anthropicStreamSerializer) withPendingStart(frame []byte) ([]byte, error) {
	if !s.pendingStart {
		return frame, nil
	}
	s.pendingStart = false
	start, err := s.buildMessageStartFrame()
	if err != nil {
		return nil, err
	}
	return append(start, frame...), nil
}

// mergeUsage 累计已知的 usage（零值不覆盖已有值）。
func (s *anthropicStreamSerializer) mergeUsage(usage *bamboo.Usage) {
	if usage == nil {
		return
	}
	if s.startUsage == nil {
		s.startUsage = &bamboo.Usage{}
	}
	if usage.InputTokens > 0 {
		s.startUsage.InputTokens = usage.InputTokens
	}
	if usage.OutputTokens > 0 {
		s.startUsage.OutputTokens = usage.OutputTokens
	}
	if usage.CacheCreationInputTokens > 0 {
		s.startUsage.CacheCreationInputTokens = usage.CacheCreationInputTokens
	}
	if usage.CacheReadInputTokens > 0 {
		s.startUsage.CacheReadInputTokens = usage.CacheReadInputTokens
	}
	if usage.ReasoningTokens > 0 {
		s.startUsage.ReasoningTokens = usage.ReasoningTokens
	}
}

// Flush 刷新缓冲区。
//
// Anthropic 流式协议没有 [DONE] 标记，message_stop 事件即为终止信号。
func (s *anthropicStreamSerializer) Flush() ([]byte, error) {
	// 流可能在 message_start 之后立即中断，此时补发待输出的 message_start 帧，
	// 保证下游看到的帧序始终符合 Anthropic 协议。
	if s.pendingStart {
		return s.withPendingStart(nil)
	}
	return nil, nil
}

// handleMessageStart 处理 message_start 事件。
//
// 该事件只登记待输出的状态，不立即产出帧：Anthropic 协议要求 message_start 携带
// 输入与缓存统计，而这些统计通常在随后的 usage 增量（ping 事件）中才到达，
// 因此帧的输出被延迟到 withPendingStart。
//
// 最终输出格式:
//
//	event: message_start
//	data: {"type":"message_start","message":{"id":"...","type":"message","role":"assistant","content":[],"model":"...","stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}
func (s *anthropicStreamSerializer) handleMessageStart(event bamboo.StreamEvent) ([]byte, error) {
	s.started = true

	if s.messageID == "" {
		s.messageID = fmt.Sprintf("msg_bamboo_%d", time.Now().UnixNano())
	}

	// 缓存事件携带的 usage，但暂不输出帧：
	// message_start 必须携带真实的输入与缓存统计，而这些统计通常在随后的
	// usage 增量（ping 事件）中才到达，因此延迟到 withPendingStart 统一输出。
	s.mergeUsage(event.Usage)
	s.pendingStart = true
	return nil, nil
}

// buildMessageStartFrame 构造 message_start 帧。
//
// Anthropic 协议的 input_tokens 仅统计非缓存输入 token，
// bamboo.Usage.InputTokens 是总输入（含 cache_creation + cache_read），需要扣除缓存部分。
func (s *anthropicStreamSerializer) buildMessageStartFrame() ([]byte, error) {
	var inputTokens, outputTokens int64
	var cacheCreation, cacheRead int64
	if s.startUsage != nil {
		inputTokens = s.startUsage.InputTokens
		outputTokens = s.startUsage.OutputTokens
		cacheCreation = s.startUsage.CacheCreationInputTokens
		cacheRead = s.startUsage.CacheReadInputTokens
	}

	nonCachedInput := inputTokens - cacheCreation - cacheRead
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}

	usageMap := map[string]any{
		"input_tokens":  nonCachedInput,
		"output_tokens": outputTokens,
	}
	if cacheCreation > 0 {
		usageMap["cache_creation_input_tokens"] = cacheCreation
	}
	if cacheRead > 0 {
		usageMap["cache_read_input_tokens"] = cacheRead
	}

	payload := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         s.model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         usageMap,
		},
	}

	return s.marshalEvent("message_start", payload)
}

// handleContentBlockStart 处理 content_block_start 事件。
//
// 输出格式:
//
//	event: content_block_start
//	data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}
func (s *anthropicStreamSerializer) handleContentBlockStart(event bamboo.StreamEvent) ([]byte, error) {
	if event.ContentBlock == nil {
		return nil, nil
	}

	contentBlock := serializeStreamContentBlock(event.ContentBlock)
	payload := map[string]any{
		"type":          "content_block_start",
		"index":         event.Index,
		"content_block": contentBlock,
	}

	return s.marshalEvent("content_block_start", payload)
}

// handleContentBlockDelta 处理 content_block_delta 事件。
//
// 输出格式:
//
//	event: content_block_delta
//	data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"..."}}
func (s *anthropicStreamSerializer) handleContentBlockDelta(event bamboo.StreamEvent) ([]byte, error) {
	delta, ok := event.Delta.(*bamboo.StreamDelta)
	if !ok {
		return nil, nil
	}

	deltaObj := map[string]any{
		"type": delta.Type,
	}
	switch delta.Type {
	case bamboo.DeltaTextDelta:
		deltaObj["text"] = delta.Text
	case bamboo.DeltaThinkingDelta:
		deltaObj["thinking"] = delta.Thinking
	case bamboo.DeltaInputJSON:
		deltaObj["partial_json"] = delta.PartialJSON
	case bamboo.DeltaSignature:
		deltaObj["signature"] = delta.Signature
	}

	payload := map[string]any{
		"type":  "content_block_delta",
		"index": event.Index,
		"delta": deltaObj,
	}

	return s.marshalEvent("content_block_delta", payload)
}

// handleContentBlockStop 处理 content_block_stop 事件。
//
// 输出格式:
//
//	event: content_block_stop
//	data: {"type":"content_block_stop","index":0}
func (s *anthropicStreamSerializer) handleContentBlockStop(event bamboo.StreamEvent) ([]byte, error) {
	payload := map[string]any{
		"type":  "content_block_stop",
		"index": event.Index,
	}
	return s.marshalEvent("content_block_stop", payload)
}

// handleMessageDelta 处理 message_delta 事件。
//
// 输出格式:
//
//	event: message_delta
//	data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":N,"output_tokens":N}}
func (s *anthropicStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok {
		return nil, nil
	}
	if msgDelta.StopReason == "" && event.Usage == nil {
		return nil, nil
	}

	stopSeq := msgDelta.StopSequence
	var stopSeqField any = stopSeq
	if stopSeq == "" {
		stopSeqField = nil
	}

	deltaObj := map[string]any{
		"stop_reason":   string(msgDelta.StopReason),
		"stop_sequence": stopSeqField,
	}

	payload := map[string]any{
		"type":  "message_delta",
		"delta": deltaObj,
	}

	if event.Usage != nil {
		// Anthropic 协议的 input_tokens 仅统计非缓存输入 token，
		// bamboo.Usage.InputTokens 是总输入（含 cache_creation + cache_read）。
		nonCachedInput := event.Usage.InputTokens -
			event.Usage.CacheCreationInputTokens -
			event.Usage.CacheReadInputTokens
		if nonCachedInput < 0 {
			nonCachedInput = 0
		}
		usageMap := map[string]any{
			"input_tokens":  nonCachedInput,
			"output_tokens": event.Usage.OutputTokens,
		}
		if event.Usage.CacheCreationInputTokens > 0 {
			usageMap["cache_creation_input_tokens"] = event.Usage.CacheCreationInputTokens
		}
		if event.Usage.CacheReadInputTokens > 0 {
			usageMap["cache_read_input_tokens"] = event.Usage.CacheReadInputTokens
		}
		payload["usage"] = usageMap
	}

	return s.marshalEvent("message_delta", payload)
}

// handleMessageStop 处理 message_stop 事件。
//
// 输出格式:
//
//	event: message_stop
//	data: {"type":"message_stop"}
func (s *anthropicStreamSerializer) handleMessageStop(event bamboo.StreamEvent) ([]byte, error) {
	payload := map[string]any{
		"type": "message_stop",
	}
	return s.marshalEvent("message_stop", payload)
}

// handlePing 处理 ping 事件。
//
// 输出格式:
//
//	event: ping
//	data: {"type":"ping"}
func (s *anthropicStreamSerializer) handlePing(event bamboo.StreamEvent) ([]byte, error) {
	payload := map[string]any{
		"type": "ping",
	}
	return s.marshalEvent("ping", payload)
}

// handleError 处理 error 事件。
//
// 输出格式:
//
//	event: error
//	data: {"type":"error","error":{"type":"...","message":"..."}}
func (s *anthropicStreamSerializer) handleError(event bamboo.StreamEvent) ([]byte, error) {
	errType := "api_error"
	errMsg := "unknown error"
	if event.Error != nil {
		errType = mapStatusCodeToAnthropicType(event.Error.StatusCode)
		if event.Error.Message != "" {
			errMsg = event.Error.Message
		}
	}

	payload := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": errMsg,
		},
	}

	return s.marshalEvent("error", payload)
}

// marshalEvent 将事件序列化为 SSE 格式（event + data 双行）。
func (s *anthropicStreamSerializer) marshalEvent(eventType string, payload map[string]any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal stream event: %v", err), 0)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}

// serializeStreamContentBlock 将 bamboo.ContentBlock 序列化为 stream content_block 的初始状态 JSON。
//
// 在 content_block_start 事件中，content_block 字段需要提供块的初始状态:
//   - text:    {"type":"text","text":""}
//   - thinking: {"type":"thinking","thinking":""}
//   - tool_use: {"type":"tool_use","id":"...","name":"...","input":{}}
func serializeStreamContentBlock(block bamboo.ContentBlock) map[string]any {
	switch b := block.(type) {
	case *bamboo.TextBlock:
		return map[string]any{
			"type": "text",
			"text": "",
		}

	case *bamboo.ThinkingBlock:
		return map[string]any{
			"type":     "thinking",
			"thinking": "",
		}

	case *bamboo.ToolUseBlock:
		return map[string]any{
			"type":  "tool_use",
			"id":    b.ID,
			"name":  b.Name,
			"input": map[string]any{},
		}

	case *bamboo.ImageBlock:
		source := map[string]any{}
		if b.Source != nil {
			source = map[string]any{
				"type":       b.Source.Type,
				"media_type": b.Source.MediaType,
				"data":       b.Source.Data,
				"url":        b.Source.URL,
			}
		}
		return map[string]any{
			"type":   "image",
			"source": source,
		}
	case *bamboo.RedactedThinkingBlock:
		return map[string]any{
			"type": "redacted_thinking",
			"data": b.Data,
		}
	}

	return map[string]any{
		"type": string(block.BlockType()),
	}
}

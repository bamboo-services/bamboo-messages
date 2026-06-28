package anthropic

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// anthropicStreamSerializer Anthropic 流式序列化器，有状态。
//
// Bamboo 的 StreamEvent 设计本身就是 Anthropic 风格，
// 序列化几乎是 1:1 JSON 编码 + SSE event/data 行封装。
type anthropicStreamSerializer struct {
	messageID string
	model     string
	started   bool
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
	switch event.Type {
	case bamboo.EventMessageStart:
		return s.handleMessageStart(event)

	case bamboo.EventContentBlockStart:
		return s.handleContentBlockStart(event)

	case bamboo.EventContentBlockDelta:
		return s.handleContentBlockDelta(event)

	case bamboo.EventContentBlockStop:
		return s.handleContentBlockStop(event)

	case bamboo.EventMessageDelta:
		return s.handleMessageDelta(event)

	case bamboo.EventMessageStop:
		return s.handleMessageStop(event)

	case bamboo.EventPing:
		return s.handlePing(event)

	case bamboo.EventError:
		return s.handleError(event)

	default:
		return nil, nil
	}
}

// Flush 刷新缓冲区。
//
// Anthropic 流式协议没有 [DONE] 标记，message_stop 事件即为终止信号。
func (s *anthropicStreamSerializer) Flush() ([]byte, error) {
	return nil, nil
}

// handleMessageStart 处理 message_start 事件。
//
// 输出格式:
//
//	event: message_start
//	data: {"type":"message_start","message":{"id":"...","type":"message","role":"assistant","content":[],"model":"...","stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}
func (s *anthropicStreamSerializer) handleMessageStart(event bamboo.StreamEvent) ([]byte, error) {
	s.started = true

	if s.messageID == "" {
		s.messageID = fmt.Sprintf("msg_bamboo_%d", time.Now().UnixNano())
	}

	if event.Message != nil {
		// 从 message 中提取 id 和 model（若有）
		// BambooMessage 没有 ID/Model 字段，这些通常由 stream 初始化时注入
	}

	var inputTokens, outputTokens int64
	var cacheCreation, cacheRead int64
	if event.Usage != nil {
		inputTokens = event.Usage.InputTokens
		outputTokens = event.Usage.OutputTokens
		cacheCreation = event.Usage.CacheCreationInputTokens
		cacheRead = event.Usage.CacheReadInputTokens
	}

	usageMap := map[string]any{
		"input_tokens":  inputTokens,
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
		usageMap := map[string]any{
			"input_tokens":  event.Usage.InputTokens,
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
		if event.Error.Type != "" {
			errType = event.Error.Type
		}
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
		return nil, fmt.Errorf("failed to marshal stream event: %w", err)
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
	}

	return map[string]any{
		"type": string(block.BlockType()),
	}
}

package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// unmarshalEvent 将 JSON 字符串反序列化为内部 messageStreamEvent。
func unmarshalEvent(t *testing.T, rawJSON string) messageStreamEvent {
	t.Helper()
	var event messageStreamEvent
	if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
		t.Fatalf("failed to unmarshal event JSON: %v", err)
	}
	return event
}

// TestContentBlockStopPassedThrough 验证 content_block_stop 事件被透传为 BlockStop delta。
//
// Anthropic 在内容块结束时发送 content_block_stop 事件，携带该内容块的索引，
// 需要将其透传给上游，以便下游组件正确识别内容块边界。
func TestContentBlockStopPassedThrough(t *testing.T) {
	p := NewProvider("test-key")

	// 构造 content_block_stop 事件，index 为 1
	eventJSON := `{
		"type": "content_block_stop",
		"index": 1
	}`
	event := unmarshalEvent(t, eventJSON)

	var finishReason provider.FinishReason
	events := p.handleStreamEvent(event, &finishReason)

	if len(events) == 0 {
		t.Fatal("expected 1 event for content_block_stop, got 0 — block stop is silently dropped")
	}

	if events[0].Type != provider.StreamTypeDelta {
		t.Errorf("event type = %q, want %q", events[0].Type, provider.StreamTypeDelta)
	}

	if events[0].Delta.Type != provider.StreamDeltaTypeBlockStop {
		t.Errorf("delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeBlockStop)
	}

	stopData, ok := events[0].Delta.Data.(provider.BlockStopData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.BlockStopData", events[0].Delta.Data)
	}

	if stopData.Index != 1 {
		t.Errorf("block stop index = %d, want 1", stopData.Index)
	}

	if !stopData.HasIndex {
		t.Error("block stop HasIndex = false, want true")
	}
}

// TestContentBlockDelta_SignatureDelta 验证 signature_delta 事件被正确转换为 NewSignatureDelta。
//
// Anthropic extended thinking 在 thinking 块结束前发送 signature_delta 携带验证签名，
// 用于多轮对话中保留推理上下文。
func TestContentMessageStart_CacheFieldsPropagated(t *testing.T) {
	p := NewProvider("test-key")

	// 构造 message_start 事件，携带 cache 字段
	eventJSON := `{
		"type": "message_start",
		"message": {
			"id": "msg_01",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-opus-20240229",
			"usage": {
				"input_tokens": 100,
				"output_tokens": 0,
				"cache_creation_input_tokens": 50,
				"cache_read_input_tokens": 25
			}
		}
	}`
	event := unmarshalEvent(t, eventJSON)

	var finishReason provider.FinishReason
	events := p.handleStreamEvent(event, &finishReason)

	if len(events) != 1 {
		t.Fatalf("expected 1 event for message_start, got %d", len(events))
	}

	if events[0].Type != provider.StreamTypeDelta {
		t.Errorf("event type = %q, want %q", events[0].Type, provider.StreamTypeDelta)
	}

	if events[0].Delta.Type != provider.StreamDeltaTypeUsage {
		t.Errorf("delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeUsage)
	}

	usageData, ok := events[0].Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.UsageData", events[0].Delta.Data)
	}

	if usageData.InputTokens != 100 {
		t.Errorf("input_tokens = %d, want 100", usageData.InputTokens)
	}
	if usageData.OutputTokens != 0 {
		t.Errorf("output_tokens = %d, want 0", usageData.OutputTokens)
	}
	if usageData.CacheCreationInputTokens != 50 {
		t.Errorf("cache_creation_input_tokens = %d, want 50", usageData.CacheCreationInputTokens)
	}
	if usageData.CacheReadInputTokens != 25 {
		t.Errorf("cache_read_input_tokens = %d, want 25", usageData.CacheReadInputTokens)
	}
}

func TestContentMessageStart_NoCacheFields(t *testing.T) {
	p := NewProvider("test-key")

	// 构造 message_start 事件，不携带 cache 字段
	eventJSON := `{
		"type": "message_start",
		"message": {
			"id": "msg_01",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-opus-20240229",
			"usage": {
				"input_tokens": 100,
				"output_tokens": 0
			}
		}
	}`
	event := unmarshalEvent(t, eventJSON)

	var finishReason provider.FinishReason
	events := p.handleStreamEvent(event, &finishReason)

	if len(events) != 1 {
		t.Fatalf("expected 1 event for message_start, got %d", len(events))
	}

	if events[0].Type != provider.StreamTypeDelta {
		t.Errorf("event type = %q, want %q", events[0].Type, provider.StreamTypeDelta)
	}

	if events[0].Delta.Type != provider.StreamDeltaTypeUsage {
		t.Errorf("delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeUsage)
	}

	usageData, ok := events[0].Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.UsageData", events[0].Delta.Data)
	}

	if usageData.InputTokens != 100 {
		t.Errorf("input_tokens = %d, want 100", usageData.InputTokens)
	}
	if usageData.OutputTokens != 0 {
		t.Errorf("output_tokens = %d, want 0", usageData.OutputTokens)
	}
	if usageData.CacheCreationInputTokens != 0 {
		t.Errorf("cache_creation_input_tokens = %d, want 0", usageData.CacheCreationInputTokens)
	}
	if usageData.CacheReadInputTokens != 0 {
		t.Errorf("cache_read_input_tokens = %d, want 0", usageData.CacheReadInputTokens)
	}
}

func TestContentBlockDelta_SignatureDelta(t *testing.T) {
	p := NewProvider("test-key")

	// 构造 signature_delta 事件
	// messageStreamEvent.Type 为 "content_block_delta"，
	// Delta 字段为 delta 子对象的原始 JSON
	eventJSON := `{
		"type": "content_block_delta",
		"index": 0,
		"delta": {
			"type": "signature_delta",
			"signature": "EvEFCu4F..."
		}
	}`

	// 手动构建 event，Delta 需要是 delta 子对象
	var raw struct {
		Type  string          `json:"type"`
		Index int             `json:"index"`
		Delta json.RawMessage `json:"delta"`
	}
	if err := json.Unmarshal([]byte(eventJSON), &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	idx := raw.Index
	event := messageStreamEvent{
		Type:  raw.Type,
		Delta: raw.Delta,
		Index: &idx,
	}

	var finishReason provider.FinishReason
	events := p.handleStreamEvent(event, &finishReason)

	if len(events) == 0 {
		t.Fatal("expected 1 event for signature_delta, got 0 — signature is silently dropped")
	}

	if events[0].Delta.Type != provider.StreamDeltaTypeSignature {
		t.Errorf("delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeSignature)
	}

	sigData, ok := events[0].Delta.Data.(provider.SignatureData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.SignatureData", events[0].Delta.Data)
	}

	if string(sigData) != "EvEFCu4F..." {
		t.Errorf("signature = %q, want %q", string(sigData), "EvEFCu4F...")
	}
}

// TestContentBlockStart_RedactedThinking 验证 redacted_thinking 类型的 content_block_start 事件
// 产生 BlockStart + RedactedThinkingDelta 两个事件。
func TestContentBlockStart_RedactedThinking(t *testing.T) {
	p := NewProvider("test-key")

	eventJSON := `{
		"type": "content_block_start",
		"index": 0,
		"content_block": {
			"type": "redacted_thinking",
			"data": "encrypted-redacted-data-123"
		}
	}`
	event := unmarshalEvent(t, eventJSON)

	var finishReason provider.FinishReason
	events := p.handleStreamEvent(event, &finishReason)

	if len(events) != 2 {
		t.Fatalf("expected 2 events (BlockStart + RedactedThinkingDelta), got %d", len(events))
	}

	// 第一个事件：BlockStart
	if events[0].Delta.Type != provider.StreamDeltaTypeBlockStart {
		t.Errorf("first delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeBlockStart)
	}
	blockStart, ok := events[0].Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatalf("first delta data = %T, want provider.BlockStartData", events[0].Delta.Data)
	}
	if blockStart.BlockType != "redacted_thinking" {
		t.Errorf("BlockType = %q, want 'redacted_thinking'", blockStart.BlockType)
	}

	// 第二个事件：RedactedThinkingDelta
	if events[1].Delta.Type != provider.StreamDeltaTypeRedactedThinking {
		t.Errorf("second delta type = %q, want %q", events[1].Delta.Type, provider.StreamDeltaTypeRedactedThinking)
	}
	rtData, ok := events[1].Delta.Data.(provider.RedactedThinkingData)
	if !ok {
		t.Fatalf("second delta data = %T, want provider.RedactedThinkingData", events[1].Delta.Data)
	}
	if string(rtData) != "encrypted-redacted-data-123" {
		t.Errorf("redacted thinking data = %q, want 'encrypted-redacted-data-123'", string(rtData))
	}
}

// TestContentBlockStart_RedactedThinking_NoData 验证 redacted_thinking 无 data 时只产生 BlockStart。
func TestContentBlockStart_RedactedThinking_NoData(t *testing.T) {
	p := NewProvider("test-key")

	eventJSON := `{
		"type": "content_block_start",
		"index": 0,
		"content_block": {
			"type": "redacted_thinking"
		}
	}`
	event := unmarshalEvent(t, eventJSON)

	var finishReason provider.FinishReason
	events := p.handleStreamEvent(event, &finishReason)

	if len(events) != 1 {
		t.Fatalf("expected 1 event (BlockStart only), got %d", len(events))
	}

	if events[0].Delta.Type != provider.StreamDeltaTypeBlockStart {
		t.Errorf("delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeBlockStart)
	}
}

package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// helper: 解析 SSE event + data 行为 map
func parseSSEEvent(t *testing.T, raw []byte) (eventType string, payload map[string]any) {
	t.Helper()
	str := string(raw)
	lines := strings.Split(strings.TrimRight(str, "\n"), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(dataStr), &payload); err != nil {
				t.Fatalf("failed to unmarshal SSE data: %v\nraw: %s", err, dataStr)
			}
		}
	}
	if eventType == "" {
		t.Fatalf("missing event: line in SSE: %q", str)
	}
	if payload == nil {
		t.Fatalf("missing data: line in SSE: %q", str)
	}
	return eventType, payload
}

func TestStreamSerializer_TextStream(t *testing.T) {
	s := newStreamSerializer("")

	// 1. message_start
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
		Usage:   &bamboo.Usage{InputTokens: 10, OutputTokens: 0},
	})
	if err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}
	if data == nil {
		t.Fatal("message_start should produce output")
	}
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "message_start" {
		t.Errorf("eventType = %q, want %q", eventType, "message_start")
	}
	if payload["type"] != "message_start" {
		t.Errorf("payload type = %v", payload["type"])
	}
	msg, ok := payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message field should be object")
	}
	if msg["role"] != "assistant" {
		t.Errorf("role = %v", msg["role"])
	}
	if msg["type"] != "message" {
		t.Errorf("message.type = %v", msg["type"])
	}

	// 2. content_block_start (text)
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "content_block_start" {
		t.Errorf("eventType = %q", eventType)
	}
	if payload["index"].(float64) != 0 {
		t.Errorf("index = %v", payload["index"])
	}
	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block should be object")
	}
	if cb["type"] != "text" {
		t.Errorf("content_block.type = %v", cb["type"])
	}

	// 3. content_block_delta (text_delta)
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "Hello"},
	})
	if err != nil {
		t.Fatalf("Serialize(text_delta) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "content_block_delta" {
		t.Errorf("eventType = %q", eventType)
	}
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["type"] != "text_delta" {
		t.Errorf("delta.type = %v", delta["type"])
	}
	if delta["text"] != "Hello" {
		t.Errorf("delta.text = %v", delta["text"])
	}

	// 4. content_block_stop
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "content_block_stop" {
		t.Errorf("eventType = %q", eventType)
	}

	// 5. message_delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
		Usage: &bamboo.Usage{InputTokens: 10, OutputTokens: 5},
	})
	if err != nil {
		t.Fatalf("Serialize(message_delta) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "message_delta" {
		t.Errorf("eventType = %q", eventType)
	}
	deltaObj, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if deltaObj["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v", deltaObj["stop_reason"])
	}
	if deltaObj["stop_sequence"] != nil {
		t.Errorf("stop_sequence should be null, got %v", deltaObj["stop_sequence"])
	}

	// 6. message_stop
	data, err = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageStop,
	})
	if err != nil {
		t.Fatalf("Serialize(message_stop) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "message_stop" {
		t.Errorf("eventType = %q", eventType)
	}
	if payload["type"] != "message_stop" {
		t.Errorf("payload type = %v", payload["type"])
	}
}

func TestStreamSerializer_FlushReturnsNil(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if data != nil {
		t.Errorf("Flush() should return nil for Anthropic (no [DONE]), got %q", string(data))
	}
}

func TestStreamSerializer_ErrorEvent(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventError,
		Error: &bamboo.BambooError{
			Category:   "上游",
			Message:    "rate exceeded",
			StatusCode: 429,
		},
	})
	if err != nil {
		t.Fatalf("Serialize(error) error = %v", err)
	}
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "error" {
		t.Errorf("eventType = %q, want %q", eventType, "error")
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field should be object")
	}
	if errObj["type"] != "rate_limit_error" {
		t.Errorf("error.type = %v", errObj["type"])
	}
	if errObj["message"] != "rate exceeded" {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestStreamSerializer_ToolUseStream(t *testing.T) {
	s := newStreamSerializer("")

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// content_block_start (tool_use)
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewToolUseBlock("call_abc", "get_weather", nil),
	})
	if err != nil {
		t.Fatalf("Serialize(tool_use start) error = %v", err)
	}
	_, payload := parseSSEEvent(t, data)
	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block should be object")
	}
	if cb["type"] != "tool_use" {
		t.Errorf("type = %v", cb["type"])
	}
	if cb["id"] != "call_abc" {
		t.Errorf("id = %v", cb["id"])
	}
	if cb["name"] != "get_weather" {
		t.Errorf("name = %v", cb["name"])
	}

	// input_json_delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"city":"SF"}`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta) error = %v", err)
	}
	_, payload = parseSSEEvent(t, data)
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["type"] != "input_json_delta" {
		t.Errorf("delta.type = %v", delta["type"])
	}
	if delta["partial_json"] != `{"city":"SF"}` {
		t.Errorf("partial_json = %v", delta["partial_json"])
	}
}

func TestStreamSerializer_ThinkingStream(t *testing.T) {
	s := newStreamSerializer("")

	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// thinking content_block_start
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewThinkingBlock("", ""),
	})
	if err != nil {
		t.Fatalf("Serialize(thinking start) error = %v", err)
	}
	_, payload := parseSSEEvent(t, data)
	cb, _ := payload["content_block"].(map[string]any)
	if cb["type"] != "thinking" {
		t.Errorf("type = %v, want %q", cb["type"], "thinking")
	}

	// thinking_delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "hmm..."},
	})
	if err != nil {
		t.Fatalf("Serialize(thinking_delta) error = %v", err)
	}
	_, payload = parseSSEEvent(t, data)
	delta, _ := payload["delta"].(map[string]any)
	if delta["thinking"] != "hmm..." {
		t.Errorf("thinking = %v", delta["thinking"])
	}
}

func TestStreamSerializer_PingEvent(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventPing,
	})
	if err != nil {
		t.Fatalf("Serialize(ping) error = %v", err)
	}
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "ping" {
		t.Errorf("eventType = %q, want %q", eventType, "ping")
	}
	if payload["type"] != "ping" {
		t.Errorf("payload type = %v", payload["type"])
	}
}

func TestStreamSerializer_MessageDeltaUsage(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonMaxTokens,
		},
		Usage: &bamboo.Usage{InputTokens: 10, OutputTokens: 100},
	})
	if err != nil {
		t.Fatalf("Serialize error = %v", err)
	}
	_, payload := parseSSEEvent(t, data)
	delta, _ := payload["delta"].(map[string]any)
	if delta["stop_reason"] != "max_tokens" {
		t.Errorf("stop_reason = %v", delta["stop_reason"])
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage should be object")
	}
	if usage["input_tokens"].(float64) != 10 {
		t.Errorf("input_tokens = %v", usage["input_tokens"])
	}
	if usage["output_tokens"].(float64) != 100 {
		t.Errorf("output_tokens = %v", usage["output_tokens"])
	}
}

func TestMessageStartModelID(t *testing.T) {
	model := "claude-sonnet-4-20250514"
	s := newStreamSerializer(model)

	data, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})
	if err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}
	if data == nil {
		t.Fatal("message_start should produce output")
	}

	_, payload := parseSSEEvent(t, data)
	msg, ok := payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message field should be object")
	}
	if msg["model"] != model {
		t.Errorf("message.model = %q, want %q", msg["model"], model)
	}

	id, ok := msg["id"].(string)
	if !ok {
		t.Fatalf("message.id should be string, got %T", msg["id"])
	}
	if id == "" {
		t.Errorf("message.id should not be empty")
	}
}

func TestSerializeStreamContentBlock_RedactedThinking(t *testing.T) {
	block := bamboo.NewRedactedThinkingBlock("encrypted_data")
	got := serializeStreamContentBlock(block)
	want := map[string]any{
		"type": "redacted_thinking",
		"data": "encrypted_data",
	}
	if got["type"] != want["type"] {
		t.Errorf("type = %v, want %v", got["type"], want["type"])
	}
	if got["data"] != want["data"] {
		t.Errorf("data = %v, want %v", got["data"], want["data"])
	}
}

func TestHandleContentBlockStart_RedactedThinking(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewRedactedThinkingBlock("encrypted_data"),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}

	eventType, payload := parseSSEEvent(t, data)
	if eventType != "content_block_start" {
		t.Errorf("eventType = %q, want %q", eventType, "content_block_start")
	}

	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block should be object")
	}
	if cb["type"] != "redacted_thinking" {
		t.Errorf("content_block.type = %v, want %q", cb["type"], "redacted_thinking")
	}
	if cb["data"] != "encrypted_data" {
		t.Errorf("content_block.data = %v, want %q", cb["data"], "encrypted_data")
	}
}

// TestSerialize_SignatureOnly_ThinkingBlock_SSE 验证 omitted thinking 模式下，
// StreamConverter + Anthropic codec 能输出完整的 content_block_start/thinking 与
// content_block_delta/signature_delta SSE 帧。
func TestSerialize_SignatureOnly_ThinkingBlock_SSE(t *testing.T) {
	// 复现 omitted thinking 模式：Provider 只发送 signature_delta
	sc := bamboo.NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewSignatureDelta("test_sig"),
	})
	if len(events) != 2 {
		t.Fatalf("期望 2 个 bamboo 事件，实际 %d 个", len(events))
	}

	s := newStreamSerializer("claude-sonnet-4-20250514")

	// 1. content_block_start: type=thinking
	data, err := s.Serialize(events[0])
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "content_block_start" {
		t.Errorf("eventType = %q, want content_block_start", eventType)
	}
	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block 应为对象")
	}
	if cb["type"] != "thinking" {
		t.Errorf("content_block.type = %v, want thinking", cb["type"])
	}

	// 2. content_block_delta: type=signature_delta, signature=test_sig
	data, err = s.Serialize(events[1])
	if err != nil {
		t.Fatalf("Serialize(content_block_delta) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "content_block_delta" {
		t.Errorf("eventType = %q, want content_block_delta", eventType)
	}
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta 应为对象")
	}
	if delta["type"] != "signature_delta" {
		t.Errorf("delta.type = %v, want signature_delta", delta["type"])
	}
	if delta["signature"] != "test_sig" {
		t.Errorf("delta.signature = %v, want test_sig", delta["signature"])
	}
}

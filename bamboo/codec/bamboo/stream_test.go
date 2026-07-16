package bamboo

import (
	"encoding/json"
	"strings"
	"testing"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
)

// parseSSEEvent 解析 SSE event + data 行，返回事件类型与 data 行反序列化后的 map。
//
// 与 anthropic/stream_test.go 中的同名 helper 行为一致，仅用于测试断言。
// 不依赖 relay.SplitSSEFrames 以避免 codec/bamboo → relay 的潜在循环依赖。
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

// assertSSEFrameFormat 断言 SSE 帧的整体格式：以 `event: ` 开头，包含 `data: ` 行，以 `\n\n` 结尾。
func assertSSEFrameFormat(t *testing.T, raw []byte, wantType string) {
	t.Helper()
	str := string(raw)
	if !strings.HasPrefix(str, "event: "+wantType+"\n") {
		t.Fatalf("SSE frame should start with %q, got: %q", "event: "+wantType+"\n", str)
	}
	if !strings.HasSuffix(str, "\n\n") {
		t.Fatalf("SSE frame should end with \\n\\n, got: %q", str)
	}
	if !strings.Contains(str, "data: ") {
		t.Fatalf("SSE frame missing data: line, got: %q", str)
	}
}

// TestStreamSerializer_MessageStart 验证 message_start 事件的 SSE 帧格式。
func TestStreamSerializer_MessageStart(t *testing.T) {
	s := newStreamSerializer("bamboo-1.0")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type:    bmbamboo.EventMessageStart,
		Message: &bmbamboo.BambooMessage{Role: bmbamboo.RoleAssistant},
		Usage:   &bmbamboo.Usage{InputTokens: 10, OutputTokens: 0},
	})
	if err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}
	if data == nil {
		t.Fatal("message_start should produce output")
	}

	assertSSEFrameFormat(t, data, "message_start")
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "message_start" {
		t.Errorf("eventType = %q, want %q", eventType, "message_start")
	}
	if payload["type"] != "message_start" {
		t.Errorf("payload type = %v", payload["type"])
	}
	msg, ok := payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message field should be object, got %T", payload["message"])
	}
	if msg["role"] != "assistant" {
		t.Errorf("message.role = %v, want assistant", msg["role"])
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage field should be object, got %T", payload["usage"])
	}
	if usage["input_tokens"].(float64) != 10 {
		t.Errorf("usage.input_tokens = %v, want 10", usage["input_tokens"])
	}
}

// TestStreamSerializer_ContentBlockStart 验证 content_block_start 事件的 SSE 帧格式。
func TestStreamSerializer_ContentBlockStart(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type:         bmbamboo.EventContentBlockStart,
		Index:        1,
		ContentBlock: bmbamboo.NewTextBlock(""),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "content_block_start")
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "content_block_start" {
		t.Errorf("eventType = %q, want content_block_start", eventType)
	}
	// Index=1 > 0，omitempty 不会省略
	if payload["index"].(float64) != 1 {
		t.Errorf("index = %v, want 1", payload["index"])
	}
	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block should be object, got %T", payload["content_block"])
	}
	if cb["type"] != "text" {
		t.Errorf("content_block.type = %v, want text", cb["type"])
	}
}

// TestStreamSerializer_ContentBlockDelta_TextDelta 验证 text_delta 增量。
func TestStreamSerializer_ContentBlockDelta_TextDelta(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type:  bmbamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bmbamboo.StreamDelta{Type: bmbamboo.DeltaTextDelta, Text: "Hello"},
	})
	if err != nil {
		t.Fatalf("Serialize(text_delta) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "content_block_delta")
	_, payload := parseSSEEvent(t, data)
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object, got %T", payload["delta"])
	}
	if delta["type"] != "text_delta" {
		t.Errorf("delta.type = %v, want text_delta", delta["type"])
	}
	if delta["text"] != "Hello" {
		t.Errorf("delta.text = %v, want Hello", delta["text"])
	}
}

// TestStreamSerializer_ContentBlockDelta_ThinkingDelta 验证 thinking_delta 增量。
func TestStreamSerializer_ContentBlockDelta_ThinkingDelta(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type:  bmbamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bmbamboo.StreamDelta{Type: bmbamboo.DeltaThinkingDelta, Thinking: "hmm..."},
	})
	if err != nil {
		t.Fatalf("Serialize(thinking_delta) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "content_block_delta")
	_, payload := parseSSEEvent(t, data)
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["type"] != "thinking_delta" {
		t.Errorf("delta.type = %v, want thinking_delta", delta["type"])
	}
	if delta["thinking"] != "hmm..." {
		t.Errorf("delta.thinking = %v, want hmm...", delta["thinking"])
	}
}

// TestStreamSerializer_ContentBlockDelta_InputJSON 验证 input_json_delta 工具调用参数增量。
func TestStreamSerializer_ContentBlockDelta_InputJSON(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type:  bmbamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bmbamboo.StreamDelta{Type: bmbamboo.DeltaInputJSON, PartialJSON: `{"city":"SF"}`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "content_block_delta")
	_, payload := parseSSEEvent(t, data)
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["type"] != "input_json_delta" {
		t.Errorf("delta.type = %v, want input_json_delta", delta["type"])
	}
	if delta["partial_json"] != `{"city":"SF"}` {
		t.Errorf("delta.partial_json = %v, want {\"city\":\"SF\"}", delta["partial_json"])
	}
}

// TestStreamSerializer_ContentBlockDelta_Signature 验证 signature_delta 思考签名增量。
func TestStreamSerializer_ContentBlockDelta_Signature(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type:  bmbamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bmbamboo.StreamDelta{Type: bmbamboo.DeltaSignature, Signature: "sig_abc"},
	})
	if err != nil {
		t.Fatalf("Serialize(signature_delta) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "content_block_delta")
	_, payload := parseSSEEvent(t, data)
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["type"] != "signature_delta" {
		t.Errorf("delta.type = %v, want signature_delta", delta["type"])
	}
	if delta["signature"] != "sig_abc" {
		t.Errorf("delta.signature = %v, want sig_abc", delta["signature"])
	}
}

// TestStreamSerializer_ContentBlockStop 验证 content_block_stop 事件。
func TestStreamSerializer_ContentBlockStop(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type:  bmbamboo.EventContentBlockStop,
		Index: 1,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "content_block_stop")
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "content_block_stop" {
		t.Errorf("eventType = %q, want content_block_stop", eventType)
	}
	if payload["type"] != "content_block_stop" {
		t.Errorf("payload type = %v", payload["type"])
	}
	if payload["index"].(float64) != 1 {
		t.Errorf("index = %v, want 1", payload["index"])
	}
}

// TestStreamSerializer_MessageDelta 验证 message_delta 事件携带停止原因与用量。
func TestStreamSerializer_MessageDelta(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type: bmbamboo.EventMessageDelta,
		Delta: &bmbamboo.MessageDelta{
			StopReason: bmbamboo.FinishReasonEndTurn,
		},
		Usage: &bmbamboo.Usage{InputTokens: 10, OutputTokens: 5},
	})
	if err != nil {
		t.Fatalf("Serialize(message_delta) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "message_delta")
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "message_delta" {
		t.Errorf("eventType = %q, want message_delta", eventType)
	}
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["stop_reason"] != "end_turn" {
		t.Errorf("delta.stop_reason = %v, want end_turn", delta["stop_reason"])
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage should be object")
	}
	if usage["input_tokens"].(float64) != 10 {
		t.Errorf("usage.input_tokens = %v, want 10", usage["input_tokens"])
	}
	if usage["output_tokens"].(float64) != 5 {
		t.Errorf("usage.output_tokens = %v, want 5", usage["output_tokens"])
	}
}

// TestStreamSerializer_MessageStop 验证 message_stop 事件。
func TestStreamSerializer_MessageStop(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type: bmbamboo.EventMessageStop,
	})
	if err != nil {
		t.Fatalf("Serialize(message_stop) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "message_stop")
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "message_stop" {
		t.Errorf("eventType = %q, want message_stop", eventType)
	}
	if payload["type"] != "message_stop" {
		t.Errorf("payload type = %v", payload["type"])
	}
}

// TestStreamSerializer_Ping 验证 ping 心跳事件。
func TestStreamSerializer_Ping(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type: bmbamboo.EventPing,
	})
	if err != nil {
		t.Fatalf("Serialize(ping) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "ping")
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "ping" {
		t.Errorf("eventType = %q, want ping", eventType)
	}
	if payload["type"] != "ping" {
		t.Errorf("payload type = %v", payload["type"])
	}
}

// TestStreamSerializer_Error 验证 error 事件携带 BambooError。
func TestStreamSerializer_Error(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bmbamboo.StreamEvent{
		Type: bmbamboo.EventError,
		Error: &bmbamboo.BambooError{
			Category:   "上游",
			Message:    "rate exceeded",
			StatusCode: 429,
		},
	})
	if err != nil {
		t.Fatalf("Serialize(error) error = %v", err)
	}
	assertSSEFrameFormat(t, data, "error")
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "error" {
		t.Errorf("eventType = %q, want error", eventType)
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field should be object, got %T", payload["error"])
	}
	if errObj["category"] != "上游" {
		t.Errorf("error.category = %v, want 上游", errObj["category"])
	}
	if errObj["message"] != "rate exceeded" {
		t.Errorf("error.message = %v, want rate exceeded", errObj["message"])
	}
	if errObj["status_code"].(float64) != 429 {
		t.Errorf("error.status_code = %v, want 429", errObj["status_code"])
	}
}

// TestStreamSerializer_FlushReturnsNil 验证 Flush 返回 nil（无 [DONE]，message_stop 即终止）。
func TestStreamSerializer_FlushReturnsNil(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if data != nil {
		t.Errorf("Flush() should return nil for bamboo (no [DONE]), got %q", string(data))
	}
}

// TestStreamSerializer_FullLifecycle 验证完整的文本流事件生命周期：
// message_start → content_block_start → content_block_delta → content_block_stop → message_delta → message_stop
// 全部经过同一个序列化器实例，输出均可被正确解析。
func TestStreamSerializer_FullLifecycle(t *testing.T) {
	s := newStreamSerializer("bamboo-1.0")

	steps := []bmbamboo.StreamEvent{
		{Type: bmbamboo.EventMessageStart, Message: &bmbamboo.BambooMessage{Role: bmbamboo.RoleAssistant}, Usage: &bmbamboo.Usage{}},
		{Type: bmbamboo.EventContentBlockStart, Index: 1, ContentBlock: bmbamboo.NewTextBlock("")},
		{Type: bmbamboo.EventContentBlockDelta, Index: 1, Delta: &bmbamboo.StreamDelta{Type: bmbamboo.DeltaTextDelta, Text: "Hi"}},
		{Type: bmbamboo.EventContentBlockStop, Index: 1},
		{Type: bmbamboo.EventMessageDelta, Delta: &bmbamboo.MessageDelta{StopReason: bmbamboo.FinishReasonEndTurn}, Usage: &bmbamboo.Usage{InputTokens: 5, OutputTokens: 1}},
		{Type: bmbamboo.EventMessageStop},
	}

	wantTypes := []bmbamboo.StreamEventType{
		bmbamboo.EventMessageStart,
		bmbamboo.EventContentBlockStart,
		bmbamboo.EventContentBlockDelta,
		bmbamboo.EventContentBlockStop,
		bmbamboo.EventMessageDelta,
		bmbamboo.EventMessageStop,
	}

	for i, ev := range steps {
		data, err := s.Serialize(ev)
		if err != nil {
			t.Fatalf("step %d Serialize error = %v", i, err)
		}
		if data == nil {
			t.Fatalf("step %d produced nil output", i)
		}
		eventType, _ := parseSSEEvent(t, data)
		if eventType != string(wantTypes[i]) {
			t.Errorf("step %d eventType = %q, want %q", i, eventType, wantTypes[i])
		}
	}

	// Flush after lifecycle should still be nil
	flushData, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if flushData != nil {
		t.Errorf("Flush() should return nil, got %q", string(flushData))
	}
}

// TestNewSerializer_ReturnsNonNil 验证 NewSerializer 不再返回 nil（stub 已替换）。
func TestNewSerializer_ReturnsNonNil(t *testing.T) {
	s := newStreamSerializer("any-model")
	if s == nil {
		t.Fatal("newStreamSerializer should return non-nil serializer (stub replaced)")
	}
}

// TestCodec_NewSerializer_ReturnsNonNil 验证通过 Codec 接口调用 NewSerializer 返回非 nil。
func TestCodec_NewSerializer_ReturnsNonNil(t *testing.T) {
	s := Codec.NewSerializer("bamboo-1.0")
	if s == nil {
		t.Fatal("Codec.NewSerializer should return non-nil serializer")
	}
	// 验证返回的 serializer 可正常工作
	data, err := s.Serialize(bmbamboo.StreamEvent{Type: bmbamboo.EventPing})
	if err != nil {
		t.Fatalf("Serialize(ping) error = %v", err)
	}
	if data == nil {
		t.Fatal("Serialize(ping) should produce output")
	}
}

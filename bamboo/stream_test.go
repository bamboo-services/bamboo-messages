package bamboo

import (
	"encoding/json"
	"testing"
)

// ---- StreamEvent JSON 序列化测试 ----

func TestStreamEvent_MessageStart(t *testing.T) {
	event := StreamEvent{
		Type: EventMessageStart,
		Message: &BambooMessage{
			Role:    RoleAssistant,
			Content: []ContentBlock{},
		},
		Usage: &Usage{InputTokens: 10, OutputTokens: 0},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Type != EventMessageStart {
		t.Errorf("Type 不匹配")
	}
	if parsed.Message == nil || parsed.Message.Role != RoleAssistant {
		t.Error("Message 不匹配")
	}
	if parsed.Usage == nil || parsed.Usage.InputTokens != 10 {
		t.Error("Usage 不匹配")
	}
}

func TestStreamEvent_ContentBlockDelta(t *testing.T) {
	delta := &StreamDelta{Type: DeltaTextDelta, Text: "你好"}
	event := StreamEvent{
		Type:  EventContentBlockDelta,
		Index: 0,
		Delta: delta,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	deltaMap, ok := parsed.Delta.(map[string]any)
	if !ok {
		t.Fatalf("Delta 类型断言失败，实际类型: %T", parsed.Delta)
	}
	if deltaMap["type"] != "text_delta" {
		t.Errorf("Delta.type 不匹配: %v", deltaMap["type"])
	}
	if deltaMap["text"] != "你好" {
		t.Errorf("Delta.text 不匹配: %v", deltaMap["text"])
	}
}

func TestStreamEvent_MessageDelta(t *testing.T) {
	msgDelta := &MessageDelta{StopReason: FinishReasonEndTurn}
	event := StreamEvent{
		Type:  EventMessageDelta,
		Delta: msgDelta,
		Usage: &Usage{InputTokens: 10, OutputTokens: 42},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Usage == nil || parsed.Usage.OutputTokens != 42 {
		t.Errorf("Usage.OutputTokens 不匹配")
	}

	deltaMap, ok := parsed.Delta.(map[string]any)
	if !ok {
		t.Fatalf("Delta 类型断言失败")
	}
	if deltaMap["stop_reason"] != "end_turn" {
		t.Errorf("Delta.stop_reason 不匹配: %v", deltaMap["stop_reason"])
	}
}

func TestStreamEvent_ContentBlockStop(t *testing.T) {
	event := StreamEvent{Type: EventContentBlockStop, Index: 1}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if parsed.Index != 1 {
		t.Errorf("Index 不匹配")
	}
}

func TestStreamEvent_MessageStop(t *testing.T) {
	event := StreamEvent{Type: EventMessageStop}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if parsed.Type != EventMessageStop {
		t.Errorf("Type 不匹配")
	}
}

func TestStreamEvent_Ping(t *testing.T) {
	event := StreamEvent{Type: EventPing}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if parsed.Type != EventPing {
		t.Errorf("Type 不匹配")
	}
}

func TestStreamEvent_Error(t *testing.T) {
	event := StreamEvent{
		Type: EventError,
		Error: &BambooError{
			Category:   "rate_limit",
			Message:    "请求频率超限，请稍后重试",
			StatusCode: 429,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Error == nil {
		t.Fatal("Error 不应为 nil")
	}
	if parsed.Error.Category != "rate_limit" {
		t.Errorf("Error.Category 不匹配")
	}
	if parsed.Error.StatusCode != 429 {
		t.Errorf("Error.StatusCode 不匹配")
	}
	if parsed.Error.Message != "请求频率超限，请稍后重试" {
		t.Errorf("Error.Message 不匹配")
	}
}

// ---- StreamDelta 测试 ----

func TestStreamDelta_TextDelta(t *testing.T) {
	delta := StreamDelta{Type: DeltaTextDelta, Text: "增量文本"}
	data, err := json.Marshal(delta)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamDelta
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if parsed.Text != "增量文本" {
		t.Errorf("Text 不匹配")
	}
}

func TestStreamDelta_ThinkingDelta(t *testing.T) {
	delta := StreamDelta{Type: DeltaThinkingDelta, Thinking: "思考过程增量"}
	data, err := json.Marshal(delta)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamDelta
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if parsed.Thinking != "思考过程增量" {
		t.Errorf("Thinking 不匹配")
	}
}

func TestStreamDelta_InputJSON(t *testing.T) {
	delta := StreamDelta{Type: DeltaInputJSON, PartialJSON: `{"city":`}
	data, err := json.Marshal(delta)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed StreamDelta
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if parsed.PartialJSON != `{"city":` {
		t.Errorf("PartialJSON 不匹配")
	}
}

// ---- MessageDelta 测试 ----

func TestMessageDelta_JSONRoundtrip(t *testing.T) {
	original := MessageDelta{StopReason: FinishReasonToolUse}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed MessageDelta
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if parsed.StopReason != FinishReasonToolUse {
		t.Errorf("StopReason 不匹配")
	}
}

// ---- 常量值测试 ----

func TestStreamEventType_Values(t *testing.T) {
	tests := []struct {
		constant StreamEventType
		expected string
	}{
		{EventMessageStart, "message_start"},
		{EventContentBlockStart, "content_block_start"},
		{EventContentBlockDelta, "content_block_delta"},
		{EventContentBlockStop, "content_block_stop"},
		{EventMessageDelta, "message_delta"},
		{EventMessageStop, "message_stop"},
		{EventPing, "ping"},
		{EventError, "error"},
	}
	for _, tt := range tests {
		if string(tt.constant) != tt.expected {
			t.Errorf("期望 %s，实际 %s", tt.expected, tt.constant)
		}
	}
}

func TestStreamDeltaType_Values(t *testing.T) {
	tests := []struct {
		constant StreamDeltaType
		expected string
	}{
		{DeltaTextDelta, "text_delta"},
		{DeltaThinkingDelta, "thinking_delta"},
		{DeltaInputJSON, "input_json_delta"},
		{DeltaSignature, "signature_delta"},
	}
	for _, tt := range tests {
		if string(tt.constant) != tt.expected {
			t.Errorf("期望 %s，实际 %s", tt.expected, tt.constant)
		}
	}
}

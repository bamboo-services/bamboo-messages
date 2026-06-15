package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// helper: 解析 SSE data 行为 openaiChunk
func parseSSEChunk(t *testing.T, raw []byte) openaiChunk {
	t.Helper()
	str := string(raw)
	if !strings.HasPrefix(str, "data: ") {
		t.Fatalf("expected SSE data prefix, got: %q", str)
	}
	str = strings.TrimPrefix(str, "data: ")
	str = strings.TrimRight(str, "\n\n")

	var chunk openaiChunk
	if err := json.Unmarshal([]byte(str), &chunk); err != nil {
		t.Fatalf("failed to unmarshal chunk: %v\nraw: %s", err, str)
	}
	return chunk
}

func TestStreamSerializer_TextStream(t *testing.T) {
	s := newStreamSerializer()

	// 1. message_start
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
	chunk := parseSSEChunk(t, data)
	if len(chunk.Choices) != 1 {
		t.Fatalf("Choices len = %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Role != "assistant" {
		t.Errorf("Delta.Role = %q, want %q", chunk.Choices[0].Delta.Role, "assistant")
	}

	// 2. content_block_start (text) — should be nil
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	if data != nil {
		t.Error("text content_block_start should produce nil output")
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
	chunk = parseSSEChunk(t, data)
	if chunk.Choices[0].Delta.Content != "Hello" {
		t.Errorf("Delta.Content = %q, want %q", chunk.Choices[0].Delta.Content, "Hello")
	}

	// 4. content_block_stop — should be nil
	data, _ = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if data != nil {
		t.Error("content_block_stop should produce nil output")
	}

	// 5. message_delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
	})
	if err != nil {
		t.Fatalf("Serialize(message_delta) error = %v", err)
	}
	chunk = parseSSEChunk(t, data)
	if chunk.Choices[0].FinishReason == nil {
		t.Fatal("FinishReason is nil")
	}
	if *chunk.Choices[0].FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", *chunk.Choices[0].FinishReason, "stop")
	}

	// 6. message_stop — should be nil
	data, _ = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageStop,
	})
	if data != nil {
		t.Error("message_stop should produce nil output")
	}

	// 7. Flush
	data, err = s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if string(data) != "data: [DONE]\n\n" {
		t.Errorf("Flush() = %q, want %q", string(data), "data: [DONE]\n\n")
	}
}

func TestStreamSerializer_ThinkingStream(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// thinking delta
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "hmm..."},
	})
	if err != nil {
		t.Fatalf("Serialize(thinking_delta) error = %v", err)
	}
	chunk := parseSSEChunk(t, data)
	if chunk.Choices[0].Delta.ReasoningContent != "hmm..." {
		t.Errorf("ReasoningContent = %q, want %q", chunk.Choices[0].Delta.ReasoningContent, "hmm...")
	}
}

func TestStreamSerializer_ToolCallStream(t *testing.T) {
	s := newStreamSerializer()

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
		t.Fatalf("Serialize(tool_use block_start) error = %v", err)
	}
	chunk := parseSSEChunk(t, data)
	if len(chunk.Choices[0].Delta.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(chunk.Choices[0].Delta.ToolCalls))
	}
	tc := chunk.Choices[0].Delta.ToolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("ToolCall ID = %q", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("ToolCall Name = %q", tc.Function.Name)
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
	chunk = parseSSEChunk(t, data)
	if len(chunk.Choices[0].Delta.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(chunk.Choices[0].Delta.ToolCalls))
	}
	if chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments != `{"city":"SF"}` {
		t.Errorf("Arguments = %q", chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)
	}

	// message_delta with tool_use finish reason
	data, _ = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonToolUse,
		},
	})
	chunk = parseSSEChunk(t, data)
	if *chunk.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", *chunk.Choices[0].FinishReason, "tool_calls")
	}
}

func TestStreamSerializer_FlushDONE(t *testing.T) {
	s := newStreamSerializer()
	data, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if !strings.Contains(string(data), "[DONE]") {
		t.Errorf("Flush output should contain [DONE], got: %q", string(data))
	}
}

func TestStreamSerializer_ErrorEvent(t *testing.T) {
	s := newStreamSerializer()
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventError,
		Error: &bamboo.BambooError{
			Type:    "api_error",
			Message: "rate exceeded",
		},
	})
	if err != nil {
		t.Fatalf("Serialize(error) error = %v", err)
	}
	str := string(data)
	if !strings.Contains(str, "rate exceeded") {
		t.Errorf("error event should contain error message, got: %q", str)
	}
	if !strings.HasPrefix(str, "data: ") {
		t.Errorf("error should be SSE format")
	}
}

func TestStreamSerializer_MultipleToolCalls(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// 第一个 tool_use
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewToolUseBlock("call_1", "tool_a", nil),
	})

	// 第二个 tool_use — index should increment
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        1,
		ContentBlock: bamboo.NewToolUseBlock("call_2", "tool_b", nil),
	})
	if err != nil {
		t.Fatalf("second tool_use error = %v", err)
	}
	chunk := parseSSEChunk(t, data)
	if chunk.Choices[0].Delta.ToolCalls[0].Index != 1 {
		t.Errorf("second tool index = %d, want 1", chunk.Choices[0].Delta.ToolCalls[0].Index)
	}
}

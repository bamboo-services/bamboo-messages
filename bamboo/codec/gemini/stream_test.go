package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// helper: 解析 SSE data 行为 geminiStreamChunk
func parseGeminiSSE(t *testing.T, raw []byte) geminiStreamChunk {
	t.Helper()
	str := string(raw)
	if !strings.HasPrefix(str, "data: ") {
		t.Fatalf("expected SSE data prefix, got: %q", str)
	}
	str = strings.TrimPrefix(str, "data: ")
	str = strings.TrimRight(str, "\n\n")

	var chunk geminiStreamChunk
	if err := json.Unmarshal([]byte(str), &chunk); err != nil {
		t.Fatalf("failed to unmarshal chunk: %v\nraw: %s", err, str)
	}
	return chunk
}

func TestStreamSerializer_TextStream(t *testing.T) {
	s := newStreamSerializer("")

	// 1. message_start — should be nil (Gemini has no block_start)
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})
	if err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}
	if data != nil {
		t.Error("message_start should produce nil output")
	}

	// 2. content_block_start (text) — should be nil
	data, _ = s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
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
	if data == nil {
		t.Fatal("text_delta should produce output")
	}
	chunk := parseGeminiSSE(t, data)
	if len(chunk.Candidates) != 1 {
		t.Fatalf("Candidates len = %d", len(chunk.Candidates))
	}
	cand := chunk.Candidates[0]
	if cand.Content == nil {
		t.Fatal("Content is nil")
	}
	if cand.Content.Role != "model" {
		t.Errorf("Role = %q, want model", cand.Content.Role)
	}
	if len(cand.Content.Parts) != 1 {
		t.Fatalf("Parts len = %d", len(cand.Content.Parts))
	}
	if cand.Content.Parts[0].Text != "Hello" {
		t.Errorf("text = %q, want Hello", cand.Content.Parts[0].Text)
	}

	// 4. content_block_stop — should be nil (not accumulating)
	data, _ = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if data != nil {
		t.Error("content_block_stop (text) should produce nil output")
	}

	// 5. message_delta — finishReason + usage
	data, err = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
		Usage: &bamboo.Usage{
			InputTokens:  10,
			OutputTokens: 20,
		},
	})
	if err != nil {
		t.Fatalf("Serialize(message_delta) error = %v", err)
	}
	chunk = parseGeminiSSE(t, data)
	if chunk.Candidates[0].FinishReason != "STOP" {
		t.Errorf("FinishReason = %q, want STOP", chunk.Candidates[0].FinishReason)
	}
	if chunk.UsageMetadata == nil {
		t.Fatal("UsageMetadata is nil")
	}
	if chunk.UsageMetadata.TotalTokenCount != 30 {
		t.Errorf("TotalTokenCount = %d, want 30", chunk.UsageMetadata.TotalTokenCount)
	}

	// 6. message_stop — should be nil
	data, _ = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageStop,
	})
	if data != nil {
		t.Error("message_stop should produce nil output")
	}

	// 7. Flush — should be nil (Gemini has no [DONE])
	data, err = s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if data != nil {
		t.Errorf("Flush() should return nil, got %q", string(data))
	}
}

func TestStreamSerializer_FunctionCallAccumulation(t *testing.T) {
	s := newStreamSerializer("")

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// content_block_start (tool_use) — should be nil, but records name
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewToolUseBlock("call-1", "get_weather", nil),
	})
	if err != nil {
		t.Fatalf("Serialize(tool_use block_start) error = %v", err)
	}
	if data != nil {
		t.Error("tool_use content_block_start should produce nil output (accumulation mode)")
	}

	// input_json_delta — should accumulate, no output
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"city"`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta 1) error = %v", err)
	}
	if data != nil {
		t.Error("input_json_delta should produce nil output (accumulating)")
	}

	// second input_json_delta — still accumulating
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `:"SF"}`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta 2) error = %v", err)
	}
	if data != nil {
		t.Error("second input_json_delta should produce nil output (still accumulating)")
	}

	// content_block_stop — should output complete functionCall
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	if data == nil {
		t.Fatal("content_block_stop should produce output (flushing accumulated functionCall)")
	}
	chunk := parseGeminiSSE(t, data)
	if len(chunk.Candidates) != 1 {
		t.Fatalf("Candidates len = %d", len(chunk.Candidates))
	}
	parts := chunk.Candidates[0].Content.Parts
	if len(parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(parts))
	}
	if parts[0].FunctionCall == nil {
		t.Fatal("functionCall is nil")
	}
	if parts[0].FunctionCall.Name != "get_weather" {
		t.Errorf("functionCall.name = %q, want get_weather", parts[0].FunctionCall.Name)
	}

	// 验证累积后的完整 args
	var args map[string]any
	if err := json.Unmarshal(parts[0].FunctionCall.Args, &args); err != nil {
		t.Fatalf("failed to unmarshal accumulated args: %v", err)
	}
	if args["city"] != "SF" {
		t.Errorf("args.city = %v, want SF", args["city"])
	}

	// 验证累积器已重置
	if s.accumulating {
		t.Error("accumulating should be false after content_block_stop")
	}
}

func TestStreamSerializer_ThinkingStream(t *testing.T) {
	s := newStreamSerializer("")

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// thinking content_block_start — nil
	data, _ := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewThinkingBlock("", ""),
	})
	if data != nil {
		t.Error("thinking content_block_start should produce nil output")
	}

	// thinking_delta — should output with thought:true
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "hmm..."},
	})
	if err != nil {
		t.Fatalf("Serialize(thinking_delta) error = %v", err)
	}
	chunk := parseGeminiSSE(t, data)
	parts := chunk.Candidates[0].Content.Parts
	if len(parts) != 1 {
		t.Fatalf("Parts len = %d", len(parts))
	}
	if parts[0].Text != "hmm..." {
		t.Errorf("text = %q, want hmm...", parts[0].Text)
	}
	if !parts[0].Thought {
		t.Error("Thought should be true for thinking_delta")
	}
}

func TestStreamSerializer_MultipleFunctionCalls(t *testing.T) {
	s := newStreamSerializer("")

	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// 第一个 functionCall
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewToolUseBlock("call-1", "tool_a", nil),
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"x":1}`},
	})
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("first content_block_stop error = %v", err)
	}
	chunk := parseGeminiSSE(t, data)
	if chunk.Candidates[0].Content.Parts[0].FunctionCall.Name != "tool_a" {
		t.Errorf("first call name = %q", chunk.Candidates[0].Content.Parts[0].FunctionCall.Name)
	}

	// 第二个 functionCall
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        1,
		ContentBlock: bamboo.NewToolUseBlock("call-2", "tool_b", nil),
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"y":2}`},
	})
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 1,
	})
	if err != nil {
		t.Fatalf("second content_block_stop error = %v", err)
	}
	chunk = parseGeminiSSE(t, data)
	if chunk.Candidates[0].Content.Parts[0].FunctionCall.Name != "tool_b" {
		t.Errorf("second call name = %q", chunk.Candidates[0].Content.Parts[0].FunctionCall.Name)
	}
}

func TestStreamSerializer_FlushReturnsNil(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if data != nil {
		t.Errorf("Flush() should return nil for Gemini, got %q", string(data))
	}
}

func TestStreamSerializer_ErrorEvent(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventError,
		Error: &bamboo.BambooError{
			Type:    bamboo.ErrorTypeRateLimit,
			Message: "quota exceeded",
		},
	})
	if err != nil {
		t.Fatalf("Serialize(error) error = %v", err)
	}
	str := string(data)
	if !strings.HasPrefix(str, "data: ") {
		t.Errorf("error should be SSE format")
	}
	if !strings.Contains(str, "quota exceeded") {
		t.Errorf("error event should contain error message, got: %q", str)
	}

	// 验证错误码映射
	var payload geminiStreamError
	jsonStr := strings.TrimPrefix(str, "data: ")
	jsonStr = strings.TrimRight(jsonStr, "\n\n")
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}
	if payload.Error.Code != 429 {
		t.Errorf("error code = %d, want 429", payload.Error.Code)
	}
	if payload.Error.Status != "RESOURCE_EXHAUSTED" {
		t.Errorf("error status = %q, want RESOURCE_EXHAUSTED", payload.Error.Status)
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
	if data != nil {
		t.Error("ping should produce nil output")
	}
}

func TestStreamSerializer_FinishReasonMapping(t *testing.T) {
	tests := []struct {
		reason bamboo.FinishReason
		want   string
	}{
		{bamboo.FinishReasonEndTurn, "STOP"},
		{bamboo.FinishReasonMaxTokens, "MAX_TOKENS"},
		{bamboo.FinishReasonToolUse, "STOP"},
		{bamboo.FinishReasonStopSequence, "STOP"},
	}

	for _, tt := range tests {
		s := newStreamSerializer("")
		data, err := s.Serialize(bamboo.StreamEvent{
			Type: bamboo.EventMessageDelta,
			Delta: &bamboo.MessageDelta{
				StopReason: tt.reason,
			},
		})
		if err != nil {
			t.Fatalf("Serialize(message_delta %v) error = %v", tt.reason, err)
		}
		chunk := parseGeminiSSE(t, data)
		if chunk.Candidates[0].FinishReason != tt.want {
			t.Errorf("reason %v → %q, want %q", tt.reason, chunk.Candidates[0].FinishReason, tt.want)
		}
	}
}

func TestStreamSerializer_TextThenFunctionCall(t *testing.T) {
	s := newStreamSerializer("")

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// text block
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	data, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "Let me search."},
	})
	chunk := parseGeminiSSE(t, data)
	if chunk.Candidates[0].Content.Parts[0].Text != "Let me search." {
		t.Errorf("text = %q", chunk.Candidates[0].Content.Parts[0].Text)
	}
	s.Serialize(bamboo.StreamEvent{Type: bamboo.EventContentBlockStop, Index: 0})

	// functionCall block
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        1,
		ContentBlock: bamboo.NewToolUseBlock("call-1", "search", nil),
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"q":"test"}`},
	})
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 1,
	})
	if err != nil {
		t.Fatalf("content_block_stop error = %v", err)
	}
	chunk = parseGeminiSSE(t, data)
	fc := chunk.Candidates[0].Content.Parts[0].FunctionCall
	if fc == nil || fc.Name != "search" {
		t.Errorf("functionCall = %+v", fc)
	}
}

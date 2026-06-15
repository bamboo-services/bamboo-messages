package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// parseResponsesSSE 解析 Responses 格式的 SSE 数据帧。
//
// 格式：`event: {type}\ndata: {json}\n\n`
func parseResponsesSSE(t *testing.T, raw []byte) (eventType string, payload map[string]any) {
	t.Helper()
	str := string(raw)

	// 提取 event 行
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
		t.Fatalf("missing event type in SSE: %q", str)
	}
	return eventType, payload
}

func TestStreamSerializer_TextStream(t *testing.T) {
	s := newStreamSerializer()

	// 1. message_start → response.created
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
	evType, payload := parseResponsesSSE(t, data)
	if evType != "response.created" {
		t.Errorf("event type = %q, want %q", evType, "response.created")
	}
	resp, ok := payload["object"].(string)
	if !ok || resp != "response" {
		t.Errorf("object = %v, want %q", payload["object"], "response")
	}

	// 2. content_block_start (text) → response.output_item.added
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	if data != nil {
		evType, payload = parseResponsesSSE(t, data)
		if evType != "response.output_item.added" {
			t.Errorf("event type = %q, want %q", evType, "response.output_item.added")
		}
		item, ok := payload["item"].(map[string]any)
		if !ok {
			t.Fatal("missing item in output_item.added")
		}
		if item["type"] != "message" {
			t.Errorf("item.type = %v, want %q", item["type"], "message")
		}
		if item["role"] != "assistant" {
			t.Errorf("item.role = %v", item["role"])
		}
	}

	// 3. content_block_delta (text_delta) → response.output_text.delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "Hello"},
	})
	if err != nil {
		t.Fatalf("Serialize(text_delta) error = %v", err)
	}
	evType, payload = parseResponsesSSE(t, data)
	if evType != "response.output_text.delta" {
		t.Errorf("event type = %q, want %q", evType, "response.output_text.delta")
	}
	if payload["delta"] != "Hello" {
		t.Errorf("delta = %v, want %q", payload["delta"], "Hello")
	}

	// 4. content_block_stop (text) → response.output_text.done
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	if data != nil {
		evType, payload = parseResponsesSSE(t, data)
		if evType != "response.output_text.done" {
			t.Errorf("event type = %q, want %q", evType, "response.output_text.done")
		}
		if payload["text"] != "Hello" {
			t.Errorf("text = %v, want %q", payload["text"], "Hello")
		}
	}

	// 5. message_delta → response.completed
	data, err = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
		Usage: &bamboo.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	})
	if err != nil {
		t.Fatalf("Serialize(message_delta) error = %v", err)
	}
	evType, payload = parseResponsesSSE(t, data)
	if evType != "response.completed" {
		t.Errorf("event type = %q, want %q", evType, "response.completed")
	}
	if payload["status"] != "completed" {
		t.Errorf("status = %v, want %q", payload["status"], "completed")
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatal("missing usage in response.completed")
	}
	if int64(usage["input_tokens"].(float64)) != 10 {
		t.Errorf("usage.input_tokens = %v", usage["input_tokens"])
	}
}

func TestStreamSerializer_FunctionCallStream(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// content_block_start (tool_use) → response.output_item.added (function_call)
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewToolUseBlock("call_abc", "get_weather", nil),
	})
	if err != nil {
		t.Fatalf("Serialize(tool_use block_start) error = %v", err)
	}
	evType, payload := parseResponsesSSE(t, data)
	if evType != "response.output_item.added" {
		t.Errorf("event type = %q, want %q", evType, "response.output_item.added")
	}
	item, ok := payload["item"].(map[string]any)
	if !ok {
		t.Fatal("missing item")
	}
	if item["type"] != "function_call" {
		t.Errorf("item.type = %v, want %q", item["type"], "function_call")
	}
	if item["call_id"] != "call_abc" {
		t.Errorf("item.call_id = %v", item["call_id"])
	}
	if item["name"] != "get_weather" {
		t.Errorf("item.name = %v", item["name"])
	}

	// input_json_delta → response.function_call_arguments.delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"city":"SF"`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta) error = %v", err)
	}
	evType, payload = parseResponsesSSE(t, data)
	if evType != "response.function_call_arguments.delta" {
		t.Errorf("event type = %q, want %q", evType, "response.function_call_arguments.delta")
	}
	if payload["delta"] != `{"city":"SF"` {
		t.Errorf("delta = %v", payload["delta"])
	}

	// content_block_stop (tool_use) → response.function_call_arguments.done
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	if data != nil {
		evType, payload = parseResponsesSSE(t, data)
		if evType != "response.function_call_arguments.done" {
			t.Errorf("event type = %q, want %q", evType, "response.function_call_arguments.done")
		}
		if payload["arguments"] != `{"city":"SF"` {
			t.Errorf("arguments = %v", payload["arguments"])
		}
	}
}

func TestStreamSerializer_ThinkingStream(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// content_block_start (thinking) → response.output_item.added (reasoning)
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewThinkingBlock("", ""),
	})
	if err != nil {
		t.Fatalf("Serialize(thinking block_start) error = %v", err)
	}
	evType, payload := parseResponsesSSE(t, data)
	if evType != "response.output_item.added" {
		t.Errorf("event type = %q, want %q", evType, "response.output_item.added")
	}
	item, ok := payload["item"].(map[string]any)
	if !ok {
		t.Fatal("missing item")
	}
	if item["type"] != "reasoning" {
		t.Errorf("item.type = %v, want %q", item["type"], "reasoning")
	}

	// thinking_delta → response.reasoning_text.delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "hmm..."},
	})
	if err != nil {
		t.Fatalf("Serialize(thinking_delta) error = %v", err)
	}
	evType, payload = parseResponsesSSE(t, data)
	if evType != "response.reasoning_text.delta" {
		t.Errorf("event type = %q, want %q", evType, "response.reasoning_text.delta")
	}
	if payload["delta"] != "hmm..." {
		t.Errorf("delta = %v", payload["delta"])
	}
}

func TestStreamSerializer_ResponseCreated(t *testing.T) {
	s := newStreamSerializer()

	data, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})
	if err != nil {
		t.Fatalf("Serialize error = %v", err)
	}

	evType, payload := parseResponsesSSE(t, data)
	if evType != "response.created" {
		t.Errorf("event type = %q, want %q", evType, "response.created")
	}
	if payload["id"] == nil || payload["id"] == "" {
		t.Error("response.id should not be empty")
	}
	if payload["object"] != "response" {
		t.Errorf("object = %v, want %q", payload["object"], "response")
	}
	if payload["status"] != "in_progress" {
		t.Errorf("status = %v, want %q", payload["status"], "in_progress")
	}
}

func TestStreamSerializer_ResponseCompletedWithUsage(t *testing.T) {
	s := newStreamSerializer()

	// 先发送 message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// message_delta with usage
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
		Usage: &bamboo.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	})
	if err != nil {
		t.Fatalf("Serialize error = %v", err)
	}

	evType, payload := parseResponsesSSE(t, data)
	if evType != "response.completed" {
		t.Errorf("event type = %q, want %q", evType, "response.completed")
	}
	if payload["status"] != "completed" {
		t.Errorf("status = %v", payload["status"])
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatal("missing usage")
	}
	if int64(usage["input_tokens"].(float64)) != 100 {
		t.Errorf("input_tokens = %v", usage["input_tokens"])
	}
	if int64(usage["output_tokens"].(float64)) != 50 {
		t.Errorf("output_tokens = %v", usage["output_tokens"])
	}
}

func TestStreamSerializer_MaxTokensStatus(t *testing.T) {
	s := newStreamSerializer()

	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonMaxTokens,
		},
	})
	if err != nil {
		t.Fatalf("Serialize error = %v", err)
	}

	evType, payload := parseResponsesSSE(t, data)
	if evType != "response.completed" {
		t.Errorf("event type = %q", evType)
	}
	if payload["status"] != "incomplete" {
		t.Errorf("status = %v, want %q", payload["status"], "incomplete")
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
		t.Fatalf("Serialize error = %v", err)
	}

	evType, payload := parseResponsesSSE(t, data)
	if evType != "response.failed" {
		t.Errorf("event type = %q, want %q", evType, "response.failed")
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatal("missing error object")
	}
	if errObj["message"] != "rate exceeded" {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestStreamSerializer_Flush(t *testing.T) {
	s := newStreamSerializer()
	data, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	// Responses 的 Flush 返回 nil（由 response.completed 终结）
	if data != nil {
		t.Errorf("Flush() should return nil for Responses format, got %q", string(data))
	}
}

func TestStreamSerializer_CombinedStream(t *testing.T) {
	// 完整流程：thinking + text + tool_use
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// thinking block
	data1, _ := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewThinkingBlock("", ""),
	})
	if data1 == nil {
		t.Fatal("thinking block_start should produce output")
	}

	data2, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "Let me think..."},
	})
	if data2 == nil {
		t.Fatal("thinking delta should produce output")
	}

	// text block
	data3, _ := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        1,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	if data3 == nil {
		t.Fatal("text block_start should produce output")
	}

	data4, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "Answer: "},
	})
	if data4 == nil {
		t.Fatal("text delta should produce output")
	}

	data5, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 1,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "42"},
	})
	if data5 == nil {
		t.Fatal("second text delta should produce output")
	}

	// 验证累积文本正确
	data6, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 1,
	})
	if data6 != nil {
		evType, payload := parseResponsesSSE(t, data6)
		if evType != "response.output_text.done" {
			t.Errorf("event type = %q", evType)
		}
		if payload["text"] != "Answer: 42" {
			t.Errorf("accumulated text = %v, want %q", payload["text"], "Answer: 42")
		}
	}
}

func TestStreamSerializer_TextAccumulation(t *testing.T) {
	s := newStreamSerializer()

	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// 多次 text delta
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})

	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "A"},
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "B"},
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "C"},
	})

	// output_text.done 应包含累积的 "ABC"
	data, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if data != nil {
		_, payload := parseResponsesSSE(t, data)
		if payload["text"] != "ABC" {
			t.Errorf("text = %v, want %q", payload["text"], "ABC")
		}
	}
}

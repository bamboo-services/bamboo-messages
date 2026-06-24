package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ════════════════════════════════════════════════════════════════════════════
// SSE 解析辅助
// ════════════════════════════════════════════════════════════════════════════

// responsesSSEFrame 单个 SSE 数据帧。
type responsesSSEFrame struct {
	eventType string
	payload   map[string]any
}

// parseAllResponsesSSE 解析所有 SSE 数据帧（支持多帧拼接）。
//
// marshalSSE 输出格式为 `event: {type}\ndata: {json}\n\n`，
// handleContentBlockStop 会拼接两个 SSE 帧到同一 []byte。
func parseAllResponsesSSE(t *testing.T, raw []byte) []responsesSSEFrame {
	t.Helper()
	str := string(raw)

	// 按 \n\n 分割为独立 SSE 帧
	trimmed := strings.TrimRight(str, "\n")
	chunks := strings.Split(trimmed, "\n\n")

	var frames []responsesSSEFrame
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		frame := responsesSSEFrame{}
		for _, line := range strings.Split(chunk, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "event: "):
				frame.eventType = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				dataStr := strings.TrimPrefix(line, "data: ")
				if err := json.Unmarshal([]byte(dataStr), &frame.payload); err != nil {
					t.Fatalf("failed to unmarshal SSE data: %v\nraw: %s", err, dataStr)
				}
			}
		}
		if frame.eventType == "" {
			t.Fatalf("missing event type in SSE chunk: %q", chunk)
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		t.Fatalf("no SSE frames found in raw data: %q", str)
	}
	return frames
}

// parseResponsesSSE 解析单个 SSE 数据帧（取第一帧）。
//
// 对于单帧事件直接返回；对于多帧拼接（如 handleContentBlockStop），
// 使用 parseAllResponsesSSE 获取所有帧。
func parseResponsesSSE(t *testing.T, raw []byte) (eventType string, payload map[string]any) {
	t.Helper()
	frames := parseAllResponsesSSE(t, raw)
	return frames[0].eventType, frames[0].payload
}

// extractResponse 从 payload 中提取嵌套的 response 对象。
//
// 生命周期事件（response.created / response.completed / response.failed）
// 使用 marshalSSEWithResponse 包装：`{"type":"...","response":{...}}`。
func extractResponse(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	resp, ok := payload["response"].(map[string]any)
	if !ok {
		t.Fatalf("missing or invalid 'response' field in payload: %v", payload)
	}
	return resp
}

func requireSequenceNumber(t *testing.T, payload map[string]any) {
	t.Helper()
	if _, ok := payload["sequence_number"].(float64); !ok {
		t.Fatalf("payload for %v missing numeric sequence_number: %v", payload["type"], payload)
	}
}

func requireStringField(t *testing.T, payload map[string]any, key string) string {
	t.Helper()
	value, ok := payload[key].(string)
	if !ok || value == "" {
		t.Fatalf("payload for %v missing string %q: %v", payload["type"], key, payload)
	}
	return value
}

// ════════════════════════════════════════════════════════════════════════════
// 流式序列化测试
// ════════════════════════════════════════════════════════════════════════════

func TestStreamSerializer_TextStream(t *testing.T) {
	s := newStreamSerializer()

	// 1. message_start → response.created（嵌套在 response 字段中）
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
	// 验证 type 字段注入
	if payload["type"] != "response.created" {
		t.Errorf("payload.type = %v, want %q", payload["type"], "response.created")
	}
	requireSequenceNumber(t, payload)
	resp := extractResponse(t, payload)
	if resp["object"] != "response" {
		t.Errorf("response.object = %v, want %q", resp["object"], "response")
	}
	if resp["status"] != "in_progress" {
		t.Errorf("response.status = %v, want %q", resp["status"], "in_progress")
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
		// type 字段注入
		if payload["type"] != "response.output_item.added" {
			t.Errorf("payload.type = %v, want %q", payload["type"], "response.output_item.added")
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
	requireSequenceNumber(t, payload)
	requireStringField(t, payload, "response_id")
	requireStringField(t, payload, "item_id")
	if _, ok := payload["logprobs"].([]any); !ok {
		t.Fatalf("output_text.delta should include logprobs array, got %T", payload["logprobs"])
	}
	if payload["delta"] != "Hello" {
		t.Errorf("delta = %v, want %q", payload["delta"], "Hello")
	}
	if payload["type"] != "response.output_text.delta" {
		t.Errorf("payload.type = %v, want %q", payload["type"], "response.output_text.delta")
	}

	// 4. content_block_stop (text) → 2 帧: response.output_text.done + response.output_item.done
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	frames := parseAllResponsesSSE(t, data)
	if len(frames) != 2 {
		t.Fatalf("content_block_stop should produce 2 SSE frames, got %d", len(frames))
	}
	// 第一帧：output_text.done
	if frames[0].eventType != "response.output_text.done" {
		t.Errorf("frame[0] event type = %q, want %q", frames[0].eventType, "response.output_text.done")
	}
	requireSequenceNumber(t, frames[0].payload)
	requireStringField(t, frames[0].payload, "item_id")
	if _, ok := frames[0].payload["logprobs"].([]any); !ok {
		t.Fatalf("output_text.done should include logprobs array, got %T", frames[0].payload["logprobs"])
	}
	if frames[0].payload["text"] != "Hello" {
		t.Errorf("frame[0] text = %v, want %q", frames[0].payload["text"], "Hello")
	}
	// 第二帧：output_item.done（携带完整 message item，status=completed）
	if frames[1].eventType != "response.output_item.done" {
		t.Errorf("frame[1] event type = %q, want %q", frames[1].eventType, "response.output_item.done")
	}
	requireSequenceNumber(t, frames[1].payload)
	item, ok := frames[1].payload["item"].(map[string]any)
	if !ok {
		t.Fatal("frame[1] missing item in output_item.done")
	}
	if item["status"] != "completed" {
		t.Errorf("frame[1] item.status = %v, want %q", item["status"], "completed")
	}

	// 5. message_delta → response.completed（嵌套在 response 字段中）
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
	requireSequenceNumber(t, payload)
	resp = extractResponse(t, payload)
	if resp["status"] != "completed" {
		t.Errorf("response.status = %v, want %q", resp["status"], "completed")
	}
	usage, ok := resp["usage"].(map[string]any)
	if !ok {
		t.Fatal("missing usage in response.completed")
	}
	if int64(usage["input_tokens"].(float64)) != 10 {
		t.Errorf("usage.input_tokens = %v", usage["input_tokens"])
	}
	if int64(usage["output_tokens"].(float64)) != 5 {
		t.Errorf("usage.output_tokens = %v", usage["output_tokens"])
	}
	if _, ok := usage["input_tokens_details"].(map[string]any); !ok {
		t.Fatalf("usage.input_tokens_details missing: %v", usage)
	}
	if _, ok := usage["output_tokens_details"].(map[string]any); !ok {
		t.Fatalf("usage.output_tokens_details missing: %v", usage)
	}
	if output, ok := resp["output"].([]any); !ok || len(output) == 0 {
		t.Fatalf("response.completed should include completed output items, got %v", resp["output"])
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
	requireSequenceNumber(t, payload)
	requireStringField(t, payload, "response_id")
	requireStringField(t, payload, "item_id")
	if payload["call_id"] != "call_abc" {
		t.Fatalf("payload.call_id = %v, want call_abc", payload["call_id"])
	}
	if payload["delta"] != `{"city":"SF"` {
		t.Errorf("delta = %v", payload["delta"])
	}

	// content_block_stop (tool_use) → 2 帧: function_call_arguments.done + output_item.done
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	frames := parseAllResponsesSSE(t, data)
	if len(frames) != 2 {
		t.Fatalf("content_block_stop should produce 2 SSE frames, got %d", len(frames))
	}
	// 第一帧：function_call_arguments.done
	if frames[0].eventType != "response.function_call_arguments.done" {
		t.Errorf("frame[0] event type = %q, want %q", frames[0].eventType, "response.function_call_arguments.done")
	}
	requireSequenceNumber(t, frames[0].payload)
	requireStringField(t, frames[0].payload, "item_id")
	if frames[0].payload["call_id"] != "call_abc" {
		t.Fatalf("frame[0] call_id = %v, want call_abc", frames[0].payload["call_id"])
	}
	if frames[0].payload["name"] != "get_weather" {
		t.Fatalf("frame[0] name = %v, want get_weather", frames[0].payload["name"])
	}
	if frames[0].payload["arguments"] != `{"city":"SF"` {
		t.Errorf("frame[0] arguments = %v", frames[0].payload["arguments"])
	}
	// 第二帧：output_item.done
	if frames[1].eventType != "response.output_item.done" {
		t.Errorf("frame[1] event type = %q, want %q", frames[1].eventType, "response.output_item.done")
	}
	doneItem, ok := frames[1].payload["item"].(map[string]any)
	if !ok {
		t.Fatal("frame[1] missing item in output_item.done")
	}
	if doneItem["status"] != "completed" {
		t.Errorf("frame[1] item.status = %v, want %q", doneItem["status"], "completed")
	}
	if doneItem["name"] != "get_weather" {
		t.Errorf("frame[1] item.name = %v", doneItem["name"])
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
	// type 字段注入
	if payload["type"] != "response.created" {
		t.Errorf("payload.type = %v, want %q", payload["type"], "response.created")
	}
	// response 嵌套包装
	resp := extractResponse(t, payload)
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("response.id should not be empty")
	}
	if resp["object"] != "response" {
		t.Errorf("response.object = %v, want %q", resp["object"], "response")
	}
	if resp["status"] != "in_progress" {
		t.Errorf("response.status = %v, want %q", resp["status"], "in_progress")
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
	resp := extractResponse(t, payload)
	if resp["status"] != "completed" {
		t.Errorf("response.status = %v", resp["status"])
	}
	usage, ok := resp["usage"].(map[string]any)
	if !ok {
		t.Fatal("missing usage in response")
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
	resp := extractResponse(t, payload)
	if resp["status"] != "incomplete" {
		t.Errorf("response.status = %v, want %q", resp["status"], "incomplete")
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
	resp := extractResponse(t, payload)
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("missing error object in response")
	}
	if errObj["message"] != "rate exceeded" {
		t.Errorf("error.message = %v", errObj["message"])
	}
	if errObj["type"] != "api_error" {
		t.Errorf("error.type = %v", errObj["type"])
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

	// content_block_stop → 2 帧: output_text.done + output_item.done
	data6, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 1,
	})
	frames := parseAllResponsesSSE(t, data6)
	if len(frames) != 2 {
		t.Fatalf("content_block_stop should produce 2 SSE frames, got %d", len(frames))
	}
	// 第一帧验证累积文本
	if frames[0].eventType != "response.output_text.done" {
		t.Errorf("frame[0] event type = %q, want %q", frames[0].eventType, "response.output_text.done")
	}
	if frames[0].payload["text"] != "Answer: 42" {
		t.Errorf("frame[0] accumulated text = %v, want %q", frames[0].payload["text"], "Answer: 42")
	}
	// 第二帧验证 output_item.done
	if frames[1].eventType != "response.output_item.done" {
		t.Errorf("frame[1] event type = %q, want %q", frames[1].eventType, "response.output_item.done")
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

	// content_block_stop → 第一帧 output_text.done 应包含累积的 "ABC"
	data, _ := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	frames := parseAllResponsesSSE(t, data)
	if len(frames) < 1 {
		t.Fatalf("expected at least 1 frame, got %d", len(frames))
	}
	if frames[0].payload["text"] != "ABC" {
		t.Errorf("frame[0] text = %v, want %q", frames[0].payload["text"], "ABC")
	}
}

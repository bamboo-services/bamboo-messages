package responses

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ════════════════════════════════════════════════════════════════════════════
// Usage total_tokens 回归测试（流式 + 非流式）
// ════════════════════════════════════════════════════════════════════════════
//
// 背景：response.completed 事件的 usage 需要包含 total_tokens 字段，
// 值为 input_tokens + output_tokens。即使 Usage 为 nil 也应输出零值 usage。
// 非流式 serializeResponse 同理。

// TestResponsesStream_TotalTokens 验证 response.completed SSE 中 total_tokens 正确计算。
//
// 构造 MessageDelta 事件携带 Usage{InputTokens:10, OutputTokens:20}，
// 断言 response.completed 的 JSON 中包含 "total_tokens":30。
func TestResponsesStream_TotalTokens(t *testing.T) {
	s := newStreamSerializer()

	// 先发 message_start 建立 response.created
	if _, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	}); err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}

	// 发 message_delta 携带 usage
	data, err := s.Serialize(bamboo.StreamEvent{
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
	if data == nil {
		t.Fatal("message_delta should produce response.completed output")
	}

	_, payload := parseResponsesSSE(t, data)
	resp := extractResponse(t, payload)

	usageRaw, ok := resp["usage"]
	if !ok {
		t.Fatal("missing 'usage' field in response.completed")
	}
	usage, ok := usageRaw.(map[string]any)
	if !ok {
		t.Fatalf("usage is not a map: %T", usageRaw)
	}

	// 验证 total_tokens = 30
	totalTokens, _ := usage["total_tokens"].(float64)
	if int64(totalTokens) != 30 {
		t.Errorf("usage.total_tokens = %v, want 30", totalTokens)
	}
	inputTokens, _ := usage["input_tokens"].(float64)
	if int64(inputTokens) != 10 {
		t.Errorf("usage.input_tokens = %v, want 10", inputTokens)
	}
	outputTokens, _ := usage["output_tokens"].(float64)
	if int64(outputTokens) != 20 {
		t.Errorf("usage.output_tokens = %v, want 20", outputTokens)
	}
}

// TestResponsesStream_ZeroUsage 验证 Usage 为 nil 时仍输出零值 usage 结构。
//
// feed MessageDelta 不带 Usage（nil），断言 response.completed JSON
// 仍包含 usage:{input_tokens:0, output_tokens:0, total_tokens:0}。
func TestResponsesStream_ZeroUsage(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// message_delta 不带 Usage
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
		// Usage 为 nil
	})
	if err != nil {
		t.Fatalf("Serialize(message_delta) error = %v", err)
	}
	if data == nil {
		t.Fatal("message_delta should produce response.completed output")
	}

	_, payload := parseResponsesSSE(t, data)
	resp := extractResponse(t, payload)

	usageRaw, ok := resp["usage"]
	if !ok {
		t.Fatal("missing 'usage' field in response.completed even when Usage is nil")
	}
	usage, ok := usageRaw.(map[string]any)
	if !ok {
		t.Fatalf("usage is not a map: %T", usageRaw)
	}

	// 所有字段应为 0
	for _, key := range []string{"input_tokens", "output_tokens", "total_tokens"} {
		v, exists := usage[key]
		if !exists {
			t.Errorf("usage.%s missing when Usage is nil", key)
			continue
		}
		fv, ok := v.(float64)
		if !ok {
			t.Errorf("usage.%s is not a number: %T", key, v)
			continue
		}
		if int64(fv) != 0 {
			t.Errorf("usage.%s = %v, want 0 (Usage is nil)", key, fv)
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
// Reasoning 累积回归测试
// ════════════════════════════════════════════════════════════════════════════
//
// 背景：reasoning_text.delta 事件的内容应在 serializer 中累积，
// content_block_stop 时发出 reasoning_text.done（携带完整文本）和
// output_item.done（携带 reasoning item 的 content 数组）。

// setupReasoningStream 构建一个发送了 thinking 增量的 serializer，
// 返回 content_block_stop 产生的 SSE 字节（包含两个帧）。
func setupReasoningStream(t *testing.T) []byte {
	t.Helper()
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// content_block_start (thinking)
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewThinkingBlock("", ""),
	})

	// 两次 thinking delta — 内容应累积为 "Hello World"
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "Hello "},
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "World"},
	})

	// content_block_stop → 2 帧: reasoning_text.done + output_item.done
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	if data == nil {
		t.Fatal("content_block_stop for thinking should produce output")
	}
	return data
}

// TestResponsesStream_ReasoningAccumulation 验证 reasoning_text.done 携带累积后的完整文本。
//
// feed BlockStart(thinking) + DeltaThinkingDelta("Hello ") + DeltaThinkingDelta("World") + BlockStop，
// 解析 SSE 找到 response.reasoning_text.done，断言 text 为 "Hello World"。
func TestResponsesStream_ReasoningAccumulation(t *testing.T) {
	data := setupReasoningStream(t)
	frames := parseAllResponsesSSE(t, data)

	// 找到 reasoning_text.done 帧
	var found bool
	for _, f := range frames {
		if f.eventType != "response.reasoning_text.done" {
			continue
		}
		found = true
		text, ok := f.payload["text"].(string)
		if !ok {
			t.Fatalf("reasoning_text.done payload.text is not string: %T", f.payload["text"])
		}
		if text != "Hello World" {
			t.Errorf("reasoning_text.done text = %q, want %q", text, "Hello World")
		}
	}
	if !found {
		t.Fatalf("response.reasoning_text.done frame not found in %d frames", len(frames))
	}
}

// TestResponsesStream_ReasoningOutputItemContent 验证 output_item.done 的 reasoning item content。
//
// 同上 setup，解析 response.output_item.done，断言 reasoning item 的 content 数组
// 包含 {"type":"reasoning_text","text":"Hello World"}。
func TestResponsesStream_ReasoningOutputItemContent(t *testing.T) {
	data := setupReasoningStream(t)
	frames := parseAllResponsesSSE(t, data)

	// 找到 output_item.done 帧
	var found bool
	for _, f := range frames {
		if f.eventType != "response.output_item.done" {
			continue
		}
		found = true
		itemRaw, ok := f.payload["item"]
		if !ok {
			t.Fatal("output_item.done missing 'item' field")
		}
		// item 经过 map[string]any 反序列化，需要重新 marshal/unmarshal 到 outputItem
		itemBytes, _ := json.Marshal(itemRaw)
		var item outputItem
		if err := json.Unmarshal(itemBytes, &item); err != nil {
			t.Fatalf("failed to unmarshal item: %v", err)
		}
		if item.Type != "reasoning" {
			t.Errorf("item.type = %q, want %q", item.Type, "reasoning")
		}
		// thinking 内容在 summary 中（明文），content 为空
		if len(item.Summary) != 1 {
			t.Fatalf("item.summary len = %d, want 1", len(item.Summary))
		}
		if item.Summary[0].Type != "summary_text" {
			t.Errorf("summary[0].type = %q, want %q", item.Summary[0].Type, "summary_text")
		}
		if item.Summary[0].Text != "Hello World" {
			t.Errorf("summary[0].text = %q, want %q", item.Summary[0].Text, "Hello World")
		}
	}
	if !found {
		t.Fatalf("response.output_item.done frame not found in %d frames", len(frames))
	}
}

// TestResponsesNonStream_TotalTokens 验证非流式 serializeResponse 的 total_tokens。
//
// 调用 serializeResponse 传入 Response{Usage: Usage{InputTokens:5, OutputTokens:15}}，
// 解析 JSON 断言 total_tokens = 20。
func TestResponsesNonStream_TotalTokens(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_test_001",
		Model:      "gpt-4",
		StopReason: bamboo.FinishReasonEndTurn,
		Usage: bamboo.Usage{
			InputTokens:  5,
			OutputTokens: 15,
		},
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("hello"),
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out responsesOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("failed to unmarshal response JSON: %v", err)
	}

	if out.Usage.TotalTokens != 20 {
		t.Errorf("usage.total_tokens = %d, want 20", out.Usage.TotalTokens)
	}
	if out.Usage.InputTokens != 5 {
		t.Errorf("usage.input_tokens = %d, want 5", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 15 {
		t.Errorf("usage.output_tokens = %d, want 15", out.Usage.OutputTokens)
	}
}

package responses

import (
	"context"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// contentReasoningTextDelta 测试
// ==============================

// findBlockStartType 查找事件列表中的 BlockStart 类型
func findBlockStartType(t *testing.T, events []provider.StreamEvent) string {
	t.Helper()
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeBlockStart {
			data := e.Delta.Data.(provider.BlockStartData)
			return data.BlockType
		}
	}
	return ""
}

// TestContentReasoningTextDelta_BlockStartType 验证首次调用时 BlockStart 类型为 "thinking"
func TestContentReasoningTextDelta_BlockStartType(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	// 构造 reasoning_text.delta 事件
	rawJSON := `{"type":"response.reasoning_text.delta","delta":"Let me think about this..."}`
	event := unmarshalResponseEvent(t, rawJSON)

	// thinkingBlockStarted 为 false，首次调用
	thinkingBlockStarted := false
	events := p.contentReasoningTextDelta(event, &thinkingBlockStarted)

	// 验证：应该返回 2 个事件（BlockStart + ThinkingDelta）
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// 验证：第一个事件是 BlockStart
	blockStartType := findBlockStartType(t, events)
	if blockStartType != "thinking" {
		t.Errorf("expected BlockStart type 'thinking', got '%s'", blockStartType)
	}

	// 验证：thinkingBlockStarted 被设置为 true
	if !thinkingBlockStarted {
		t.Error("thinkingBlockStarted should be set to true after first call")
	}

	// 验证：第二个事件是 ThinkingDelta
	if events[1].Delta.Type != provider.StreamDeltaTypeThinking {
		t.Errorf("expected ThinkingDelta, got %v", events[1].Delta.Type)
	}
}

// TestContentReasoningTextDelta_Continuation 验证后续调用时不发出 BlockStart
func TestContentReasoningTextDelta_Continuation(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	// 构造 reasoning_text.delta 事件
	rawJSON := `{"type":"response.reasoning_text.delta","delta":"more thinking..."}`
	event := unmarshalResponseEvent(t, rawJSON)

	// thinkingBlockStarted 为 true，后续调用
	thinkingBlockStarted := true
	events := p.contentReasoningTextDelta(event, &thinkingBlockStarted)

	// 验证：只返回 1 个事件（ThinkingDelta）
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// 验证：没有 BlockStart 事件
	blockStartType := findBlockStartType(t, events)
	if blockStartType != "" {
		t.Errorf("expected no BlockStart event, got '%s'", blockStartType)
	}

	// 验证：事件是 ThinkingDelta
	if events[0].Delta.Type != provider.StreamDeltaTypeThinking {
		t.Errorf("expected ThinkingDelta, got %v", events[0].Delta.Type)
	}

	// 验证：thinkingBlockStarted 仍为 true
	if !thinkingBlockStarted {
		t.Error("thinkingBlockStarted should remain true")
	}
}

// TestReasoningAndTextIndependentTracking 验证 reasoning 和 text block 独立追踪
func TestReasoningAndTextIndependentTracking(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	// 第一步：调用 contentReasoningTextDelta（首次调用）
	thinkingBlockStarted := false
	textBlockStarted := false

	reasoningJSON := `{"type":"response.reasoning_text.delta","delta":"Let me think..."}`
	reasoningEvent := unmarshalResponseEvent(t, reasoningJSON)
	reasoningEvents := p.contentReasoningTextDelta(reasoningEvent, &thinkingBlockStarted)

	// 验证：reasoning 事件应该包含 BlockStart("thinking")
	if len(reasoningEvents) != 2 {
		t.Fatalf("expected 2 reasoning events, got %d", len(reasoningEvents))
	}
	if findBlockStartType(t, reasoningEvents) != "thinking" {
		t.Error("expected BlockStart type 'thinking' for reasoning")
	}
	if !thinkingBlockStarted {
		t.Error("thinkingBlockStarted should be true after reasoning call")
	}

	// 第二步：调用 contentOutputTextDelta（首次调用）
	outputJSON := `{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello world"}`
	outputEvent := unmarshalResponseEvent(t, outputJSON)
	outputEvents := p.contentOutputTextDelta(outputEvent, &textBlockStarted)

	// 验证：output 事件应该包含 BlockStart("text")
	if len(outputEvents) != 2 {
		t.Fatalf("expected 2 output events, got %d", len(outputEvents))
	}
	if findBlockStartType(t, outputEvents) != "text" {
		t.Error("expected BlockStart type 'text' for output")
	}
	if !textBlockStarted {
		t.Error("textBlockStarted should be true after output call")
	}

	// 验证：两个 block 状态独立，互不影响
	if !thinkingBlockStarted || !textBlockStarted {
		t.Error("both block started flags should be true")
	}
}

// TestContentResponseFailed_UsageCarried 验证 response.failed 事件仍携带 Usage。
//
// 背景：contentResponseFailed 在流式响应失败时触发，应像 contentResponseCompleted
// 一样提取 usage 信息并发送 UsageDelta，确保失败场景下 usage 也能被上层捕获。
//
// 构造 ResponseFailed 事件带 Usage{InputTokens:50, OutputTokens:30}，
// 断言 events 包含 UsageDelta。
func TestContentResponseFailed_UsageCarried(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{
		"type": "response.failed",
		"response": {
			"id": "resp_fail_001",
			"status": "failed",
			"usage": {
				"input_tokens": 50,
				"output_tokens": 30
			},
			"error": {
				"message": "internal error"
			}
		}
	}`
	event := unmarshalResponseEvent(t, rawJSON)

	events := p.contentResponseFailed(context.Background(), event)

	found := false
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeUsage {
			found = true
			data, ok := e.Delta.Data.(provider.UsageData)
			if !ok {
				t.Fatal("UsageDelta Data is not UsageData")
			}
			if data.InputTokens != 50 {
				t.Errorf("UsageData.InputTokens = %d, want 50", data.InputTokens)
			}
			if data.OutputTokens != 30 {
				t.Errorf("UsageData.OutputTokens = %d, want 30", data.OutputTokens)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected UsageDelta event in response.failed with usage — " +
			"usage should be carried even on failure path")
	}
}

func TestContentOutputItemAdded_UsesCallIDForToolCallID(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{
		"type": "response.output_item.added",
		"output_index": 0,
		"item": {
			"type": "function_call",
			"id": "fc_item_123",
			"call_id": "call_real_456",
			"name": "run_shell",
			"arguments": ""
		}
	}`
	event := unmarshalResponseEvent(t, rawJSON)

	events := p.contentOutputItemAdded(event, new(bool))
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].Delta.Type != provider.StreamDeltaTypeToolCall {
		t.Fatalf("delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeToolCall)
	}
	data, ok := events[0].Delta.Data.(provider.ToolCallData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.ToolCallData", events[0].Delta.Data)
	}
	if data.ID != "call_real_456" {
		t.Fatalf("ToolCallData.ID = %q, want call_id %q", data.ID, "call_real_456")
	}
	if data.Name != "run_shell" {
		t.Fatalf("ToolCallData.Name = %q, want run_shell", data.Name)
	}
}

// ==============================
// Task 9: 推理事件 + output_item.done 测试
// ==============================

// TestContentReasoningSummaryTextDelta_BlockStart 验证 reasoning_summary_text.delta 首次调用时合成 thinking BlockStart
func TestContentReasoningSummaryTextDelta_BlockStart(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{"type":"response.reasoning_summary_text.delta","output_index":0,"content_index":0,"delta":"Analyzing the request..."}`
	event := unmarshalResponseEvent(t, rawJSON)

	thinkingBlockStarted := false
	events := p.contentReasoningSummaryTextDelta(event, &thinkingBlockStarted)

	if len(events) != 2 {
		t.Fatalf("expected 2 events (BlockStart + ThinkingDelta), got %d", len(events))
	}
	if findBlockStartType(t, events) != "thinking" {
		t.Error("expected BlockStart type 'thinking'")
	}
	if !thinkingBlockStarted {
		t.Error("thinkingBlockStarted should be true")
	}
	if events[1].Delta.Type != provider.StreamDeltaTypeThinking {
		t.Errorf("expected ThinkingDelta, got %v", events[1].Delta.Type)
	}
}

// TestContentOutputItemDone_Reasoning 验证 output_item.done 提取 reasoning ID 和 EncryptedContent
func TestContentOutputItemDone_Reasoning(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{
		"type": "response.output_item.done",
		"output_index": 0,
		"item": {
			"type": "reasoning",
			"id": "rs_abc123",
			"encrypted_content": "gAAAAABp_test_encrypted_content",
			"summary": []
		}
	}`
	event := unmarshalResponseEvent(t, rawJSON)

	events := p.contentOutputItemDone(event)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Delta.Type != provider.StreamDeltaTypeMetadata {
		t.Fatalf("expected MetadataDelta, got %v", events[0].Delta.Type)
	}
	data, ok := events[0].Delta.Data.(provider.MetadataData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.MetadataData", events[0].Delta.Data)
	}
	if data.ReasoningID != "rs_abc123" {
		t.Errorf("ReasoningID = %q, want rs_abc123", data.ReasoningID)
	}
	if data.EncryptedContent != "gAAAAABp_test_encrypted_content" {
		t.Errorf("EncryptedContent = %q, want gAAAAABp_test_encrypted_content", data.EncryptedContent)
	}
}

// TestContentOutputItemDone_NonReasoning 验证 output_item.done 对非 reasoning 类型返回 nil
func TestContentOutputItemDone_NonReasoning(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{
		"type": "response.output_item.done",
		"output_index": 0,
		"item": {
			"type": "function_call",
			"id": "fc_123",
			"call_id": "call_456",
			"name": "search",
			"arguments": "{}"
		}
	}`
	event := unmarshalResponseEvent(t, rawJSON)

	events := p.contentOutputItemDone(event)
	if events != nil {
		t.Fatalf("expected nil for non-reasoning item, got %d events", len(events))
	}
}

// TestContentResponseCreated_ExtractsResponseID 验证 response.created 提取 Response ID
func TestContentResponseCreated_ExtractsResponseID(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{
		"type": "response.created",
		"response": {
			"id": "resp_test789",
			"status": "in_progress",
			"model": "o3",
			"output": []
		}
	}`
	event := unmarshalResponseEvent(t, rawJSON)

	events := p.contentResponseCreated(event)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Delta.Type != provider.StreamDeltaTypeMetadata {
		t.Fatalf("expected MetadataDelta, got %v", events[0].Delta.Type)
	}
	data, ok := events[0].Delta.Data.(provider.MetadataData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.MetadataData", events[0].Delta.Data)
	}
	if data.ResponseID != "resp_test789" {
		t.Errorf("ResponseID = %q, want resp_test789", data.ResponseID)
	}
}

// TestContentResponseCreated_EmptyID 验证 response.created 空 ID 时返回 nil
func TestContentResponseCreated_EmptyID(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{
		"type": "response.created",
		"response": {
			"id": "",
			"status": "in_progress",
			"model": "o3",
			"output": []
		}
	}`
	event := unmarshalResponseEvent(t, rawJSON)

	events := p.contentResponseCreated(event)
	if events != nil {
		t.Fatalf("expected nil for empty response ID, got %d events", len(events))
	}
}

// TestContentReasoningTextDelta_LegacyCompat 验证旧事件名 reasoning_text.delta 仍被处理
func TestContentReasoningTextDelta_LegacyCompat(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	rawJSON := `{"type":"response.reasoning_text.delta","delta":"legacy thinking"}`
	event := unmarshalResponseEvent(t, rawJSON)

	thinkingBlockStarted := true // 已开始，不应再发 BlockStart
	events := p.contentReasoningTextDelta(event, &thinkingBlockStarted)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Delta.Type != provider.StreamDeltaTypeThinking {
		t.Errorf("expected ThinkingDelta, got %v", events[0].Delta.Type)
	}
}

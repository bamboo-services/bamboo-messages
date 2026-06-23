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

// TestStreamSerializer_UsageDeltaNoFinishReason 验证 UsageDelta 产生的 message_delta
// （StopReason 为空）不会输出 finish_reason 字段。
//
// 背景：上游模型在 finish chunk 中同时返回 usage 和 finish_reason，
// bamboo 适配器会拆分为 UsageDelta（先）和 StreamTypeStop（后）两个事件。
// UsageDelta 在 StreamConverter 中生成 EventMessageDelta(StopReason=""),
// 如果 codec 层把空 StopReason 映射为 "stop"，会产生一个额外的 finish_reason:"stop" chunk，
// 导致客户端收到两个 finish_reason（先 stop 后 tool_calls）。
func TestStreamSerializer_UsageDeltaNoFinishReason(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// 模拟 UsageDelta 产生的 EventMessageDelta（StopReason 为空，仅携带 usage）
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: "",
		},
		Usage: &bamboo.Usage{
			InputTokens:  10,
			OutputTokens: 20,
		},
	})
	if err != nil {
		t.Fatalf("Serialize(usage delta) error = %v", err)
	}
	chunk := parseSSEChunk(t, data)

	if chunk.Choices[0].FinishReason != nil {
		t.Errorf("FinishReason should be nil for empty StopReason, got %q", *chunk.Choices[0].FinishReason)
	}
	if chunk.Usage == nil {
		t.Fatal("Usage should be present")
	}
	if chunk.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", chunk.Usage.PromptTokens)
	}
	if chunk.Usage.CompletionTokens != 20 {
		t.Errorf("CompletionTokens = %d, want 20", chunk.Usage.CompletionTokens)
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

// TestStreamSerializer_ToolCallIndexConsistency 验证 tool_call 的 index 在
// content_block_start 和 content_block_delta(input_json_delta) 之间保持一致。
//
// 该测试模拟 bamboo.StreamConverter 的真实行为：当模型发起工具调用时，
// StreamConverter 的 sc.blockIndex 会在 ToolCall 事件中先递增，导致
// content_block_start 和 content_block_delta 的 Index 字段携带的是
// StreamConverter 内部的 blockIndex（而非从 0 起算的 tool 序号）。
//
// 之前 codec 的 DeltaInputJSON 分支直接使用 event.Index，而 ContentBlockStart
// 分支使用内部 toolIndex（从 0 起算），两者不一致。这导致下游客户端（如
// Vercel AI SDK）把参数增量误认为新的 tool_call，因缺少 id 而抛出
// "Expected 'id' to be a string."。
//
// 场景一：模型直接调用工具（无前置 text block）
//
//	StreamConverter 行为: stopIdx=0, blockIndex++→1, block_start Index=1, delta Index=1
//
// 场景二：模型先输出 text 再调用工具
//
//	StreamConverter 行为: text block Index=0; tool: stopIdx=0, blockIndex++→1,
//	block_start Index=1, delta Index=1
func TestStreamSerializer_ToolCallIndexConsistency_NoPrecedingText(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// 模拟 StreamConverter 在"无前置 text"时的输出：
	// content_block_start (tool_use) Index=1
	blockStartData, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        1, // StreamConverter: sc.blockIndex 已递增到 1
		ContentBlock: bamboo.NewToolUseBlock("call_abc", "get_weather", nil),
	})
	if err != nil {
		t.Fatalf("Serialize(block_start tool_use) error = %v", err)
	}
	blockStartChunk := parseSSEChunk(t, blockStartData)
	if len(blockStartChunk.Choices[0].Delta.ToolCalls) != 1 {
		t.Fatalf("block_start ToolCalls len = %d", len(blockStartChunk.Choices[0].Delta.ToolCalls))
	}
	startIndex := blockStartChunk.Choices[0].Delta.ToolCalls[0].Index

	// content_block_delta (input_json_delta) Index=1（StreamConverter: sc.blockIndex 仍为 1）
	deltaData, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 1, // StreamConverter: sc.blockIndex 仍为 1
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"city":"SF"}`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta) error = %v", err)
	}
	deltaChunk := parseSSEChunk(t, deltaData)
	if len(deltaChunk.Choices[0].Delta.ToolCalls) != 1 {
		t.Fatalf("delta ToolCalls len = %d", len(deltaChunk.Choices[0].Delta.ToolCalls))
	}
	deltaIndex := deltaChunk.Choices[0].Delta.ToolCalls[0].Index

	if startIndex != deltaIndex {
		t.Errorf("tool_call index mismatch: block_start=%d, delta=%d — "+
			"downstream client will treat delta as a new tool_call and fail "+
			"with 'Expected id to be a string'", startIndex, deltaIndex)
	}
}

func TestStreamSerializer_ToolCallIndexConsistency_WithPrecedingText(t *testing.T) {
	s := newStreamSerializer()

	// message_start
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// text block (Index=0)
	s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "Let me check."},
	})
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})

	// 模拟 StreamConverter 在 ToolCall 事件中的行为：
	// stopIdx = sc.blockIndex = 0, sc.blockIndex++ → 1
	// content_block_stop Index=0
	s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	// content_block_start (tool_use) Index=1
	blockStartData, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        1, // StreamConverter: sc.blockIndex 已递增到 1
		ContentBlock: bamboo.NewToolUseBlock("call_def", "search", nil),
	})
	if err != nil {
		t.Fatalf("Serialize(block_start tool_use) error = %v", err)
	}
	blockStartChunk := parseSSEChunk(t, blockStartData)
	startIndex := blockStartChunk.Choices[0].Delta.ToolCalls[0].Index

	// content_block_delta (input_json_delta) Index=1
	deltaData, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 1, // StreamConverter: sc.blockIndex 仍为 1
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"q":"weather"}`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta) error = %v", err)
	}
	deltaChunk := parseSSEChunk(t, deltaData)
	deltaIndex := deltaChunk.Choices[0].Delta.ToolCalls[0].Index

	if startIndex != deltaIndex {
		t.Errorf("tool_call index mismatch: block_start=%d, delta=%d — "+
			"downstream client will treat delta as a new tool_call and fail "+
			"with 'Expected id to be a string'", startIndex, deltaIndex)
	}
}

package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestAudit_ToolUse_NoDoubleBlockStart 验证 Gemini 工具调用事件序列。
//
// Fix: Gemini handlePart 为 FunctionCall 仅发出 ToolCallDelta + ToolCallDeltaData，
// 不再发出 BlockStartDeltaWithID("tool_use")，与 Anthropic/OpenAI 适配器保持一致。
func TestAudit_ToolUse_NoDoubleBlockStart(t *testing.T) {
	p := NewProvider("test-key")

	argsJSON, _ := json.Marshal(map[string]any{"city": "Tokyo"})
	part := &geminiPart{
		FunctionCall: &functionCall{
			ID:   "call_123",
			Name: "get_weather",
			Args: argsJSON,
		},
	}

	textStarted := false
	thinkingStarted := false
	events := p.handlePart(part, &textStarted, &thinkingStarted)

	if len(events) == 0 {
		t.Fatal("expected events for FunctionCall, got 0")
	}

	blockStartCount := 0
	toolCallCount := 0
	for _, e := range events {
		switch e.Delta.Type {
		case provider.StreamDeltaTypeBlockStart:
			blockStartCount++
		case provider.StreamDeltaTypeToolCall:
			toolCallCount++
		}
	}

	// Fix 验证：不应有 BlockStart，仅有 ToolCallDelta
	if blockStartCount > 0 {
		t.Errorf("BlockStart emitted %d times for tool_use, should be 0 (StreamConverter handles block lifecycle)", blockStartCount)
	}
	if toolCallCount != 1 {
		t.Errorf("ToolCallDelta count = %d, want 1", toolCallCount)
	}
}

// TestAudit_ToolUse_StreamConverterSimulation 验证 Gemini 工具调用通过 StreamConverter 后的事件序列。
//
// Fix: 模拟 StreamConverter 处理 Gemini 工具调用事件，验证不会产生空块。
func TestAudit_ToolUse_StreamConverterSimulation(t *testing.T) {
	p := NewProvider("test-key")

	// 先模拟文本
	textPart := &geminiPart{Text: "Let me check."}
	textStarted := false
	thinkingStarted := false
	textEvents := p.handlePart(textPart, &textStarted, &thinkingStarted)

	// 再模拟工具调用
	argsJSON, _ := json.Marshal(map[string]any{"city": "Tokyo"})
	toolPart := &geminiPart{
		FunctionCall: &functionCall{
			ID:   "call_456",
			Name: "get_weather",
			Args: argsJSON,
		},
	}
	toolEvents := p.handlePart(toolPart, &textStarted, &thinkingStarted)

	allEvents := append(textEvents, toolEvents...)

	// 模拟 StreamConverter 处理
	blockIndex := 0
	var eventLog []string

	for _, pe := range allEvents {
		switch pe.Delta.Type {
		case provider.StreamDeltaTypeBlockStart:
			data := pe.Delta.Data.(provider.BlockStartData)
			eventLog = append(eventLog, fmt.Sprintf("block_start(%s,idx=%d)", data.BlockType, blockIndex))
		case provider.StreamDeltaTypeTextOutput:
			eventLog = append(eventLog, fmt.Sprintf("text(idx=%d)", blockIndex))
		case provider.StreamDeltaTypeToolCall:
			eventLog = append(eventLog, fmt.Sprintf("block_stop(idx=%d)", blockIndex))
			blockIndex++
			eventLog = append(eventLog, fmt.Sprintf("block_start(tool_use,idx=%d)", blockIndex))
		case provider.StreamDeltaTypeToolCallDelta:
			eventLog = append(eventLog, fmt.Sprintf("tool_delta(idx=%d)", blockIndex))
		}
	}

	logStr := strings.Join(eventLog, " → ")

	// 验证：不应该有 block_start(tool_use,idx=0) 后紧跟 block_stop(idx=0) 的模式（空块）
	for i := 1; i < len(eventLog); i++ {
		if strings.HasPrefix(eventLog[i], "block_stop") && strings.HasPrefix(eventLog[i-1], "block_start(tool_use") {
			t.Errorf("Empty tool_use block detected (opened then immediately closed).\n"+
				"Event sequence: %s", logStr)
			return
		}
	}
}

// TestAudit_ThinkingBlockStart_DeltaType 验证 Gemini thinking 块的 BlockStart 事件类型正确。
func TestAudit_ThinkingBlockStart_DeltaType(t *testing.T) {
	p := NewProvider("test-key")

	part := &geminiPart{
		Thought: true,
		Text:    "Let me reason about this...",
	}

	textStarted := false
	thinkingStarted := false
	events := p.handlePart(part, &textStarted, &thinkingStarted)

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (BlockStart + ThinkingDelta), got %d", len(events))
	}

	// 第一个事件应该是 BlockStart("thinking")
	if events[0].Delta.Type != provider.StreamDeltaTypeBlockStart {
		t.Errorf("first event should be BlockStart, got %v", events[0].Delta.Type)
	}
	data, ok := events[0].Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatal("BlockStart data type assertion failed")
	}
	if data.BlockType != "thinking" {
		t.Errorf("BlockStart type = %q, want thinking", data.BlockType)
	}

	// 第二个事件应该是 ThinkingDelta
	if events[1].Delta.Type != provider.StreamDeltaTypeThinking {
		t.Errorf("second event should be ThinkingDelta, got %v", events[1].Delta.Type)
	}
}

package gemini

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestHandlePart_ThinkingExtraction 验证 Gemini thinking part 的 Thought 内容被正确提取为 ThinkingDelta。
//
// Gemini 2.5 thinking 功能通过 Thought=true 标记推理内容，
// 用于多轮对话中保留推理上下文。
func TestHandlePart_ThinkingExtraction(t *testing.T) {
	p := NewProvider("test-key")

	// 构造一个带 Thought 的 thinking part
	part := &geminiPart{
		Text:    "Let me analyze this...",
		Thought: true,
	}

	textStarted := false
	thinkingStarted := false
	events := p.handlePart(part, &textStarted, &thinkingStarted)

	// 应该有 2 个事件: BlockStart("thinking") + ThinkingDelta
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (BlockStart + ThinkingDelta), got %d", len(events))
	}

	// 查找 ThinkingDelta 事件
	var foundThinking bool
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeThinking {
			foundThinking = true
			thinkingData, ok := e.Delta.Data.(provider.ThinkingData)
			if !ok {
				t.Fatalf("thinking delta data type = %T, want provider.ThinkingData", e.Delta.Data)
			}
			if string(thinkingData) != "Let me analyze this..." {
				t.Errorf("thinking = %q, want %q", string(thinkingData), "Let me analyze this...")
			}
		}
	}

	if !foundThinking {
		t.Fatal("no ThinkingDelta event found — Thought content is silently dropped")
	}

	// thinkingStarted 应该被设置为 true
	if !thinkingStarted {
		t.Error("thinkingBlockStarted should be true after thinking part")
	}
}

// TestHandlePart_ThinkingBlockStartType 验证 thinking part 首次触发 BlockStart("thinking")。
func TestHandlePart_ThinkingBlockStartType(t *testing.T) {
	p := NewProvider("test-key")

	part := &geminiPart{
		Text:    "Reasoning...",
		Thought: true,
	}

	textStarted := false
	thinkingStarted := false
	events := p.handlePart(part, &textStarted, &thinkingStarted)

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	if events[0].Delta.Type != provider.StreamDeltaTypeBlockStart {
		t.Fatalf("first event should be BlockStart, got %v", events[0].Delta.Type)
	}
	data, ok := events[0].Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatal("BlockStart data type assertion failed")
	}
	if data.BlockType != "thinking" {
		t.Errorf("BlockStart type = %q, want thinking", data.BlockType)
	}
}

package completions

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// checkBlockStartType 验证事件列表中包含指定类型的 BlockStart 事件
func checkBlockStartType(t *testing.T, events []provider.StreamEvent, expectedType string) {
	t.Helper()
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeBlockStart {
			data := e.Delta.Data.(provider.BlockStartData)
			if data.BlockType != expectedType {
				t.Errorf("BlockStart type = %q, want %q", data.BlockType, expectedType)
			}
			return
		}
	}
	t.Errorf("No BlockStart event found in %d events", len(events))
}

// hasDeltaType 检查事件列表中是否存在指定类型的 Delta 事件
func hasDeltaType(events []provider.StreamEvent, deltaType provider.StreamDeltaType) bool {
	for _, e := range events {
		if e.Delta.Type == deltaType {
			return true
		}
	}
	return false
}

func TestHandleChoice_ReasoningContent(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	choice := openai.ChatCompletionChunkChoice{
		Index: 0,
		Delta: openai.ChatCompletionChunkChoiceDelta{},
	}
	choice.Delta.JSON.ExtraFields = map[string]respjson.Field{
		"reasoning_content": respjson.NewField(`"thinking text"`),
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted)

	// 应产生 2 个事件: BlockStart("thinking") + ThinkingDelta
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	checkBlockStartType(t, events, "thinking")
	if !hasDeltaType(events, provider.StreamDeltaTypeThinking) {
		t.Error("missing ThinkingDelta event")
	}
	if textBlockStarted {
		t.Error("textBlockStarted should remain false")
	}
	if !thinkingBlockStarted {
		t.Error("thinkingBlockStarted should be true")
	}
}

func TestHandleChoice_ReasoningContentNull(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	choice := openai.ChatCompletionChunkChoice{
		Index: 0,
		Delta: openai.ChatCompletionChunkChoiceDelta{},
	}
	choice.Delta.JSON.ExtraFields = map[string]respjson.Field{
		"reasoning_content": respjson.NewField("null"),
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted)

	// null 值: Valid() 返回 false，应跳过
	if len(events) != 0 {
		t.Fatalf("expected 0 events for null reasoning_content, got %d", len(events))
	}
	if hasDeltaType(events, provider.StreamDeltaTypeThinking) {
		t.Error("should not emit ThinkingDelta for null value")
	}
}

func TestHandleChoice_ReasoningContentEmpty(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	choice := openai.ChatCompletionChunkChoice{
		Index: 0,
		Delta: openai.ChatCompletionChunkChoiceDelta{},
	}
	choice.Delta.JSON.ExtraFields = map[string]respjson.Field{
		"reasoning_content": respjson.NewField(`""`),
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted)

	// 空字符串: 反序列化成功但 reasoning == ""，应跳过
	if len(events) != 0 {
		t.Fatalf("expected 0 events for empty reasoning_content, got %d", len(events))
	}
	if hasDeltaType(events, provider.StreamDeltaTypeThinking) {
		t.Error("should not emit ThinkingDelta for empty string")
	}
}

func TestHandleChoice_ReasoningBeforeText(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	// 第一次调用: 仅 reasoning_content，无 Content
	reasoningChoice := openai.ChatCompletionChunkChoice{
		Index: 0,
		Delta: openai.ChatCompletionChunkChoiceDelta{},
	}
	reasoningChoice.Delta.JSON.ExtraFields = map[string]respjson.Field{
		"reasoning_content": respjson.NewField(`"step 1: analyze"`),
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	events1 := p.handleChoice(reasoningChoice, &textBlockStarted, &thinkingBlockStarted)

	if len(events1) != 2 {
		t.Fatalf("first call: expected 2 events, got %d", len(events1))
	}
	checkBlockStartType(t, events1, "thinking")
	if !hasDeltaType(events1, provider.StreamDeltaTypeThinking) {
		t.Error("first call: missing ThinkingDelta")
	}
	if !thinkingBlockStarted {
		t.Error("first call: thinkingBlockStarted should be true")
	}
	if textBlockStarted {
		t.Error("first call: textBlockStarted should remain false")
	}

	// 第二次调用: 仅 Content，无 reasoning_content
	textChoice := openai.ChatCompletionChunkChoice{
		Index: 0,
		Delta: openai.ChatCompletionChunkChoiceDelta{
			Content: "final answer",
		},
	}

	events2 := p.handleChoice(textChoice, &textBlockStarted, &thinkingBlockStarted)

	if len(events2) != 2 {
		t.Fatalf("second call: expected 2 events, got %d", len(events2))
	}
	checkBlockStartType(t, events2, "text")
	if !hasDeltaType(events2, provider.StreamDeltaTypeTextOutput) {
		t.Error("second call: missing TextDelta")
	}
	if !textBlockStarted {
		t.Error("second call: textBlockStarted should be true")
	}
}

func TestHandleChoice_OnlyReasoning(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	choice := openai.ChatCompletionChunkChoice{
		Index: 0,
		Delta: openai.ChatCompletionChunkChoiceDelta{},
	}
	choice.Delta.JSON.ExtraFields = map[string]respjson.Field{
		"reasoning_content": respjson.NewField(`"deep thought"`),
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted)

	// 应产生 BlockStart("thinking") + ThinkingDelta，不崩溃
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	checkBlockStartType(t, events, "thinking")
	if !hasDeltaType(events, provider.StreamDeltaTypeThinking) {
		t.Error("missing ThinkingDelta event")
	}
	// 确保没有文本相关事件
	if hasDeltaType(events, provider.StreamDeltaTypeTextOutput) {
		t.Error("should not emit TextDelta when only reasoning_content present")
	}
}

package completions

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
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

	choice := chatCompletionChunkChoice{
		Index: 0,
		Delta: chatCompletionDelta{
			ReasoningContent: json.RawMessage(`"thinking text"`),
		},
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)

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

	choice := chatCompletionChunkChoice{
		Index: 0,
		Delta: chatCompletionDelta{
			ReasoningContent: json.RawMessage("null"),
		},
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)

	// null 值解析为字符串 "null"，非空但语义无效 — parseReasoningRaw 会返回 "null"
	// 这里验证 null 不会产生有效推理增量
	if len(events) != 0 {
		t.Fatalf("expected 0 events for null reasoning_content, got %d", len(events))
	}
	if hasDeltaType(events, provider.StreamDeltaTypeThinking) {
		t.Error("should not emit ThinkingDelta for null value")
	}
}

func TestHandleChoice_ReasoningContentEmpty(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	choice := chatCompletionChunkChoice{
		Index: 0,
		Delta: chatCompletionDelta{
			ReasoningContent: json.RawMessage(`""`),
		},
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)

	// 空字符串: 解析后 reasoning == ""，应跳过
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
	reasoningChoice := chatCompletionChunkChoice{
		Index: 0,
		Delta: chatCompletionDelta{
			ReasoningContent: json.RawMessage(`"step 1: analyze"`),
		},
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	events1 := p.handleChoice(reasoningChoice, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)

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
	textChoice := chatCompletionChunkChoice{
		Index: 0,
		Delta: chatCompletionDelta{
			Content: "final answer",
		},
	}

	events2 := p.handleChoice(textChoice, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)

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

	choice := chatCompletionChunkChoice{
		Index: 0,
		Delta: chatCompletionDelta{
			ReasoningContent: json.RawMessage(`"deep thought"`),
		},
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	events := p.handleChoice(choice, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)

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

// TestHandleChunk_RelaxedUsageCondition 验证 usage 提取条件已放宽。
//
// 背景：原条件 `chunk.Usage.TotalTokens > 0` 会遗漏 TotalTokens=0 的 chunk
// （如 CompletionTokens=0 但 PromptTokens>0 的场景）。修复后条件为
// TotalTokens>0 || PromptTokens>0 || CompletionTokens>0，确保只要有
// 任一非零字段就提取 usage。
func TestHandleChunk_RelaxedUsageCondition(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	chunk := chatCompletionChunk{
		Usage: &chunkUsage{
			PromptTokens:     100,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	events := p.handleChunk(chunk, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)

	found := false
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeUsage {
			found = true
			data, ok := e.Delta.Data.(provider.UsageData)
			if !ok {
				t.Fatal("UsageDelta Data is not UsageData")
			}
			if data.InputTokens != 100 {
				t.Errorf("UsageData.InputTokens = %d, want 100", data.InputTokens)
			}
			if data.OutputTokens != 0 {
				t.Errorf("UsageData.OutputTokens = %d, want 0", data.OutputTokens)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected UsageDelta event when PromptTokens>0 but TotalTokens=0 — " +
			"relaxed condition should fire")
	}
}

func TestHandleChoice_AllowsFinishReasonUpgradeAfterStop(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false

	stopReason := "stop"
	stopEvents := p.handleChoice(chatCompletionChunkChoice{
		FinishReason: &stopReason,
	}, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)
	if len(stopEvents) != 1 {
		t.Fatalf("stop choice events = %d, want 1", len(stopEvents))
	}
	if stopEvents[0].FinishReason != provider.FinishReasonStop {
		t.Fatalf("first finish reason = %q, want %q", stopEvents[0].FinishReason, provider.FinishReasonStop)
	}

	toolReason := "tool_calls"
	toolEvents := p.handleChoice(chatCompletionChunkChoice{
		FinishReason: &toolReason,
	}, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)
	if len(toolEvents) != 1 {
		t.Fatalf("tool_calls choice events = %d, want 1; stopSent must not swallow finish reason upgrades", len(toolEvents))
	}
	if toolEvents[0].FinishReason != provider.FinishReasonToolCalls {
		t.Fatalf("second finish reason = %q, want %q", toolEvents[0].FinishReason, provider.FinishReasonToolCalls)
	}
}

// TestParseReasoningRaw_ObjectFormat 验证 JSON 对象格式的 reasoning_content 解析。
func TestParseReasoningRaw_ObjectFormat(t *testing.T) {
	// {"text": "..."} 格式
	got := parseReasoningRaw(json.RawMessage(`{"text": "object reasoning"}`))
	if got != "object reasoning" {
		t.Errorf("parseReasoningRaw({text:...}) = %q, want %q", got, "object reasoning")
	}

	// {"content": "..."} 格式
	got = parseReasoningRaw(json.RawMessage(`{"content": "content field"}`))
	if got != "content field" {
		t.Errorf("parseReasoningRaw({content:...}) = %q, want %q", got, "content field")
	}

	// 纯字符串格式
	got = parseReasoningRaw(json.RawMessage(`"plain string"`))
	if got != "plain string" {
		t.Errorf("parseReasoningRaw(string) = %q, want %q", got, "plain string")
	}

	// 空值
	got = parseReasoningRaw(nil)
	if got != "" {
		t.Errorf("parseReasoningRaw(nil) = %q, want empty", got)
	}
}

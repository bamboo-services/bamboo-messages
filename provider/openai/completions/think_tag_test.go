package completions

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// 本文件中所有 think 标签均通过 thinkOpenTag / thinkCloseTag 常量拼接构造
// （常量定义见 think_tag.go）。禁止在源码中书写完整标签字面量——
// 完整字面量会被部分工具链（如 AI 代码助手）误解析为自身思考块的边界，
// 导致生成内容被截断、文件损坏。碎片字面量（如 "<th"、"ink>"）不受影响。

func TestThinkTagStripper_NoThinkTags(t *testing.T) {
	s := newThinkTagStripper()

	events := s.process("Hello world")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Delta.Type != provider.StreamDeltaTypeTextOutput {
		t.Fatalf("expected text_output, got %s", events[0].Delta.Type)
	}
	if string(events[0].Delta.Data.(provider.TextData)) != "Hello world" {
		t.Fatalf("expected 'Hello world', got %q", events[0].Delta.Data)
	}
}

func TestThinkTagStripper_InlineThinkTag(t *testing.T) {
	s := newThinkTagStripper()

	events := s.process("Hello world")
	assertTextEvents(t, events, "Hello world")

	// 完整 think 标签在单个 chunk 中
	events = s.process(thinkOpenTag + "reasoning" + thinkCloseTag + "response")
	// 应该产生: thinking delta + text delta
	var thinkingParts, textParts []string
	for _, e := range events {
		switch e.Delta.Type {
		case provider.StreamDeltaTypeThinking:
			thinkingParts = append(thinkingParts, string(e.Delta.Data.(provider.ThinkingData)))
		case provider.StreamDeltaTypeTextOutput:
			textParts = append(textParts, string(e.Delta.Data.(provider.TextData)))
		}
	}
	if join(thinkingParts) != "reasoning" {
		t.Fatalf("expected thinking='reasoning', got %q", join(thinkingParts))
	}
	if join(textParts) != "response" {
		t.Fatalf("expected text='response', got %q", join(textParts))
	}
}

func TestThinkTagStripper_SplitAcrossChunks(t *testing.T) {
	s := newThinkTagStripper()

	// 模拟流式分片: "<th" "ink>" "reasoning" "</th" "ink>" "text"
	events := s.process("<th")
	// 应该缓冲，不输出
	if len(events) != 0 {
		t.Fatalf("expected 0 events for partial tag, got %d", len(events))
	}

	events = s.process("ink>")
	// 标签完成，进入 thinking 模式
	var hasBlockStart bool
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeBlockStart {
			hasBlockStart = true
		}
	}
	if !hasBlockStart {
		t.Fatal("expected block_start for thinking")
	}

	events = s.process("reasoning")
	assertThinkingEvents(t, events, "reasoning")

	events = s.process("</th")
	if len(events) != 0 {
		t.Fatalf("expected 0 events for partial close tag, got %d", len(events))
	}

	events = s.process("ink>")
	// 关闭标签完成，退出 thinking 模式
	if len(events) != 0 {
		t.Fatalf("expected 0 events for close tag, got %d", len(events))
	}

	events = s.process("text")
	assertTextEvents(t, events, "text")
}

func TestThinkTagStripper_OnlyThinkContent(t *testing.T) {
	s := newThinkTagStripper()

	events := s.process(thinkOpenTag + "deep thought" + thinkCloseTag)
	var thinkingParts []string
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeThinking {
			thinkingParts = append(thinkingParts, string(e.Delta.Data.(provider.ThinkingData)))
		}
	}
	if join(thinkingParts) != "deep thought" {
		t.Fatalf("expected 'deep thought', got %q", join(thinkingParts))
	}
}

func TestThinkTagStripper_PartialTagAtFlush(t *testing.T) {
	s := newThinkTagStripper()

	// 正常文本后跟一个不完整的标签前缀
	events := s.process("hello <")
	// "hello " 应该输出，"<" 被缓冲
	assertTextEvents(t, events, "hello ")

	// flush 时应该输出缓冲的 "<"
	events = s.flush()
	assertTextEvents(t, events, "<")
}

func TestThinkTagStripper_LegitLessThan(t *testing.T) {
	s := newThinkTagStripper()

	// "a < b" 中的 "<" 后面跟空格，不是标签
	events := s.process("a < b")
	// 应该输出 "a " 然后缓冲 "<"，然后 " b" 到来时释放缓冲
	var textParts []string
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeTextOutput {
			textParts = append(textParts, string(e.Delta.Data.(provider.TextData)))
		}
	}
	result := join(textParts)
	if result != "a < b" {
		t.Fatalf("expected 'a < b', got %q", result)
	}
}

func TestThinkTagStripper_MultipleThinkBlocks(t *testing.T) {
	s := newThinkTagStripper()

	events := s.process(thinkOpenTag + "first" + thinkCloseTag + "text1" + thinkOpenTag + "second" + thinkCloseTag + "text2")
	var thinkingParts, textParts []string
	for _, e := range events {
		switch e.Delta.Type {
		case provider.StreamDeltaTypeThinking:
			thinkingParts = append(thinkingParts, string(e.Delta.Data.(provider.ThinkingData)))
		case provider.StreamDeltaTypeTextOutput:
			textParts = append(textParts, string(e.Delta.Data.(provider.TextData)))
		}
	}
	if join(thinkingParts) != "firstsecond" {
		t.Fatalf("expected 'firstsecond', got %q", join(thinkingParts))
	}
	if join(textParts) != "text1text2" {
		t.Fatalf("expected 'text1text2', got %q", join(textParts))
	}
}

func TestThinkTagStripper_WhitespaceAfterOpenTag(t *testing.T) {
	s := newThinkTagStripper()

	// GLM 实际行为: 开标签后紧跟换行
	events := s.process(thinkOpenTag + "\n")
	// 应该进入 thinking 模式，不产生文本输出
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeTextOutput {
			t.Fatalf("unexpected text output: %q", e.Delta.Data)
		}
	}
}

func TestThinkTagStripper_EmptyInput(t *testing.T) {
	s := newThinkTagStripper()
	events := s.process("")
	if len(events) != 0 {
		t.Fatalf("expected 0 events for empty input, got %d", len(events))
	}
}

func TestThinkTagStripper_FlushWhileThinking(t *testing.T) {
	s := newThinkTagStripper()

	s.process(thinkOpenTag + "partial reasoning")
	events := s.flush()
	// flush 不应该产生额外事件（thinking 内容已经输出）
	if len(events) != 0 {
		t.Fatalf("expected 0 events on flush while thinking, got %d", len(events))
	}
}

// === 孤立闭标签（reasoning_content 泄漏场景）===

func TestThinkTagStripper_StrayCloseTag(t *testing.T) {
	s := newThinkTagStripper()

	// 文本模式下的孤立闭标签应被静默吞掉
	events := s.process("回答内容" + thinkCloseTag)
	assertTextEvents(t, events, "回答内容")
}

func TestThinkTagStripper_StrayCloseTagSplit(t *testing.T) {
	s := newThinkTagStripper()

	// 孤立闭标签跨 chunk 切断
	events := s.process("回答内容</t")
	assertTextEvents(t, events, "回答内容")

	events = s.process("hink>")
	if len(events) != 0 {
		t.Fatalf("expected stray close tag to be swallowed, got %d events", len(events))
	}

	events = s.process("后续")
	assertTextEvents(t, events, "后续")
}

func TestThinkTagStripper_FailedCandidateWithEmbeddedOpenTag(t *testing.T) {
	s := newThinkTagStripper()

	// 失败候选内嵌新标签起点: "<t" 后接完整开标签，
	// 验证再缓冲逻辑不丢失内嵌标签
	events := s.process("<t" + thinkOpenTag + "x" + thinkCloseTag + "y")
	assertThinkingEvents(t, events, "x")
	assertTextEvents(t, events, "<ty")
}

// === 非流式剥离 ===

func TestStripThinkTagsFromContent_PairedTags(t *testing.T) {
	var thinking string
	text := stripThinkTagsFromContent(thinkOpenTag+"先思考"+thinkCloseTag+"再回答", &thinking)
	if text != "再回答" {
		t.Fatalf("expected text %q, got %q", "再回答", text)
	}
	if thinking != "先思考" {
		t.Fatalf("expected thinking %q, got %q", "先思考", thinking)
	}
}

func TestStripThinkTagsFromContent_StrayCloseTag(t *testing.T) {
	var thinking string
	text := stripThinkTagsFromContent("回答"+thinkCloseTag+"继续", &thinking)
	if text != "回答继续" {
		t.Fatalf("expected text %q, got %q", "回答继续", text)
	}
	if thinking != "" {
		t.Fatalf("expected empty thinking, got %q", thinking)
	}
}

func TestStripThinkTagsFromContent_UnclosedOpenTag(t *testing.T) {
	var thinking string
	text := stripThinkTagsFromContent("正文"+thinkOpenTag+"未闭合的思考", &thinking)
	if text != "正文" {
		t.Fatalf("expected text %q, got %q", "正文", text)
	}
	if thinking != "未闭合的思考" {
		t.Fatalf("expected thinking %q, got %q", "未闭合的思考", thinking)
	}
}

func TestStripThinkTagsFromContent_NoTags(t *testing.T) {
	var thinking string
	text := stripThinkTagsFromContent("普通正文", &thinking)
	if text != "普通正文" {
		t.Fatalf("expected text %q, got %q", "普通正文", text)
	}
	if thinking != "" {
		t.Fatalf("expected empty thinking, got %q", thinking)
	}
}

func TestStripThinkTagsFromContent_MultipleBlocks(t *testing.T) {
	var thinking string
	content := thinkOpenTag + "t1" + thinkCloseTag + "a" + thinkOpenTag + "t2" + thinkCloseTag + "b"
	text := stripThinkTagsFromContent(content, &thinking)
	if text != "ab" {
		t.Fatalf("expected text %q, got %q", "ab", text)
	}
	if thinking != "t1t2" {
		t.Fatalf("expected thinking %q, got %q", "t1t2", thinking)
	}
}

// === handleChoice 集成 ===

// TestCompletionsProvider_handleChoice_StripThinkTags 验证含内联 think 标签
// 的 content 增量被正确分流为 thinking/text 事件，BlockStart 状态正确同步。
func TestCompletionsProvider_handleChoice_StripThinkTags(t *testing.T) {
	p := NewCompletionsProvider("test-key")

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	stripper := newThinkTagStripper()

	// 第一个 chunk：开标签 + 推理内容
	events := p.handleChoice(chatCompletionChunkChoice{
		Delta: chatCompletionDelta{Content: thinkOpenTag + "reasoning"},
	}, &textBlockStarted, &thinkingBlockStarted, &stopSent, stripper)

	var thinking, text []string
	collect := func(evts []provider.StreamEvent) {
		for _, e := range evts {
			switch e.Delta.Type {
			case provider.StreamDeltaTypeThinking:
				thinking = append(thinking, string(e.Delta.Data.(provider.ThinkingData)))
			case provider.StreamDeltaTypeTextOutput:
				text = append(text, string(e.Delta.Data.(provider.TextData)))
			}
		}
	}
	collect(events)

	// 应产生 BlockStart(thinking)，且 thinkingBlockStarted 被同步
	if !thinkingBlockStarted {
		t.Fatal("expected thinkingBlockStarted=true after inline open tag")
	}
	var hasThinkingBlockStart bool
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeBlockStart {
			data, ok := e.Delta.Data.(provider.BlockStartData)
			if ok && data.BlockType == "thinking" {
				hasThinkingBlockStart = true
			}
		}
	}
	if !hasThinkingBlockStart {
		t.Fatal("expected BlockStart(thinking) from stripper")
	}

	// 第二个 chunk：闭标签 + 正文
	events = p.handleChoice(chatCompletionChunkChoice{
		Delta: chatCompletionDelta{Content: thinkCloseTag + "more"},
	}, &textBlockStarted, &thinkingBlockStarted, &stopSent, stripper)
	collect(events)

	if join(thinking) != "reasoning" {
		t.Fatalf("expected thinking %q, got %q", "reasoning", join(thinking))
	}
	if join(text) != "more" {
		t.Fatalf("expected text %q, got %q", "more", join(text))
	}
	// 正文首个增量前应合成 BlockStart(text)
	if !textBlockStarted {
		t.Fatal("expected textBlockStarted=true after text output")
	}
}

// TestCompletionsProvider_handleChoice_StripStrayCloseTag 验证 reasoning_content
// 泄漏到 content 的孤立闭标签被静默移除（用户实际遇到的 GLM 场景）。
func TestCompletionsProvider_handleChoice_StripStrayCloseTag(t *testing.T) {
	p := NewCompletionsProvider("test-key")

	textBlockStarted := false
	thinkingBlockStarted := false
	stopSent := false
	stripper := newThinkTagStripper()

	events := p.handleChoice(chatCompletionChunkChoice{
		Delta: chatCompletionDelta{Content: "回答内容" + thinkCloseTag},
	}, &textBlockStarted, &thinkingBlockStarted, &stopSent, stripper)

	var text []string
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeTextOutput {
			text = append(text, string(e.Delta.Data.(provider.TextData)))
		}
		if e.Delta.Type == provider.StreamDeltaTypeThinking {
			t.Fatalf("unexpected thinking event: %q", e.Delta.Data)
		}
	}
	if join(text) != "回答内容" {
		t.Fatalf("expected text %q, got %q", "回答内容", join(text))
	}
}

// === 辅助函数 ===

func assertTextEvents(t *testing.T, events []provider.StreamEvent, expected string) {
	t.Helper()
	var parts []string
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeTextOutput {
			parts = append(parts, string(e.Delta.Data.(provider.TextData)))
		}
	}
	if join(parts) != expected {
		t.Fatalf("expected text %q, got %q", expected, join(parts))
	}
}

func assertThinkingEvents(t *testing.T, events []provider.StreamEvent, expected string) {
	t.Helper()
	var parts []string
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeThinking {
			parts = append(parts, string(e.Delta.Data.(provider.ThinkingData)))
		}
	}
	if join(parts) != expected {
		t.Fatalf("expected thinking %q, got %q", expected, join(parts))
	}
}

func join(parts []string) string {
	result := ""
	for _, p := range parts {
		result += p
	}
	return result
}

package responses

import (
	"strings"
	"testing"
)

func TestSummarizeThinking_PlainShortText(t *testing.T) {
	got := summarizeThinking("Let me analyze this problem")
	if got != "Let me analyze this problem" {
		t.Fatalf("summarizeThinking() = %q, want original text", got)
	}
}

func TestSummarizeThinking_StripMarkdown(t *testing.T) {
	input := "## **Investigating the bug**\n\nSome details here"
	got := summarizeThinking(input)
	// 首行剥离标题与强调标记
	if got != "Investigating the bug" {
		t.Fatalf("summarizeThinking() = %q, want %q", got, "Investigating the bug")
	}
}

func TestSummarizeThinking_FirstLineWins(t *testing.T) {
	input := "First line summary\nSecond line\nThird line"
	got := summarizeThinking(input)
	if got != "First line summary" {
		t.Fatalf("summarizeThinking() = %q, want first line", got)
	}
}

func TestSummarizeThinking_SentenceBoundary(t *testing.T) {
	input := "先分析根因。再实现修复方案"
	got := summarizeThinking(input)
	if got != "先分析根因。" {
		t.Fatalf("summarizeThinking() = %q, want first sentence", got)
	}
}

func TestSummarizeThinking_EnglishSentenceBoundary(t *testing.T) {
	input := "Found the root cause! Now fixing it"
	got := summarizeThinking(input)
	if got != "Found the root cause!" {
		t.Fatalf("summarizeThinking() = %q, want first sentence", got)
	}
}

func TestSummarizeThinking_LongTextTruncated(t *testing.T) {
	input := strings.Repeat("长", maxSummaryRunes+50)
	got := summarizeThinking(input)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
	// 截断后 rune 数 = maxSummaryRunes + 1（省略号）
	if n := len([]rune(got)); n != maxSummaryRunes+1 {
		t.Fatalf("truncated rune count = %d, want %d", n, maxSummaryRunes+1)
	}
}

func TestSummarizeThinking_PureSymbols(t *testing.T) {
	// 纯符号/分隔线不适合做摘要
	for _, input := range []string{"---", "===\n***", "。。。", ""} {
		if got := summarizeThinking(input); got != "" {
			t.Errorf("summarizeThinking(%q) = %q, want empty", input, got)
		}
	}
}

func TestSummarizeThinking_CodeFenceStripped(t *testing.T) {
	input := "`npm install` 安装依赖"
	got := summarizeThinking(input)
	if got != "npm install 安装依赖" {
		t.Fatalf("summarizeThinking() = %q, want backticks stripped", got)
	}
}

func TestBuildReasoningSummary_EmptyExtraction(t *testing.T) {
	// 提取不出摘要时返回空数组（非 nil，保证 JSON 序列化为 []）
	summary := buildReasoningSummary("---")
	if summary == nil {
		t.Fatal("buildReasoningSummary() = nil, want empty slice")
	}
	if len(summary) != 0 {
		t.Fatalf("len = %d, want 0", len(summary))
	}
}

func TestBuildReasoningSummary_Extracted(t *testing.T) {
	summary := buildReasoningSummary("分析完成。开始实现")
	if len(summary) != 1 {
		t.Fatalf("len = %d, want 1", len(summary))
	}
	if summary[0].Type != "summary_text" {
		t.Errorf("Type = %q, want summary_text", summary[0].Type)
	}
	if summary[0].Text != "分析完成。" {
		t.Errorf("Text = %q, want %q", summary[0].Text, "分析完成。")
	}
}

func TestBuildReasoningContent(t *testing.T) {
	content := buildReasoningContent("full thinking text")
	if len(content) != 1 {
		t.Fatalf("len = %d, want 1", len(content))
	}
	if content[0].Type != "reasoning_text" {
		t.Errorf("Type = %q, want reasoning_text", content[0].Type)
	}
	if content[0].Text != "full thinking text" {
		t.Errorf("Text = %q", content[0].Text)
	}

	// 空文本返回空数组（非 nil）
	empty := buildReasoningContent("")
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty content = %v, want empty non-nil slice", empty)
	}
}

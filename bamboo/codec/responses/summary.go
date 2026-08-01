package responses

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxSummaryRunes summary 截断的最大 rune 数。
//
// summary 官方语义为推理摘要（非原文），过长会失去摘要意义并增加
// 下游客户端展示负担；120 rune 约一两句话，兼顾信息量与简洁度。
const maxSummaryRunes = 120

// summarizeThinking 从原始思考文本启发式提取摘要，用于 reasoning item 的 summary 字段。
//
// 官方对 summary 的定义是 "A summary of the reasoning output"。relay 层不具备
// LLM 摘要能力，采用纯启发式近似：剥离 Markdown 装饰 → 取首行/首句 → 超长截断。
// 提取不出有意义的内容时返回 ""（调用方将 summary 输出为空数组）。
func summarizeThinking(thinking string) string {
	sentence := firstSentence(stripMarkdownLite(thinking))
	if !hasMeaningfulRune(sentence) {
		return ""
	}
	return truncateRunes(sentence, maxSummaryRunes)
}

// buildReasoningSummary 构造 reasoning item 的 summary 数组。
//
// 官方 schema 中 summary 为 required 字段，空数组是合法的最小形态；
// 启发式无法提取摘要时返回空数组而非填充原文，保持 summary 的摘要语义。
func buildReasoningSummary(thinking string) []outputReasoningSummary {
	summary := summarizeThinking(thinking)
	if summary == "" {
		return []outputReasoningSummary{}
	}
	return []outputReasoningSummary{{Type: "summary_text", Text: summary}}
}

// buildReasoningContent 构造 reasoning item 的 content 数组（原始思考文本轨道）。
//
// 元素 type 固定为 "reasoning_text"，与官方 schema 的 ReasoningTextContent
// 定义一致；思考文本为空时返回空数组。
func buildReasoningContent(thinking string) []outputContent {
	if thinking == "" {
		return []outputContent{}
	}
	return []outputContent{{Type: "reasoning_text", Text: thinking}}
}

// stripMarkdownLite 剥离常见 Markdown 装饰符号，保留文本骨架。
//
// 仅处理三类高频装饰：行首标题标记（#）、成对强调标记（** 与 __）、
// 代码反引号。不做完整 Markdown 解析——summary 是展示用启发式摘要，
// 过度清洗反而可能损伤正文。
func stripMarkdownLite(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 行首标题标记：# ~ ###### 后跟空白
		if hashes := len(trimmed) - len(strings.TrimLeft(trimmed, "#")); hashes > 0 && hashes <= 6 && len(trimmed) > hashes && (trimmed[hashes] == ' ' || trimmed[hashes] == '\t') {
			trimmed = strings.TrimSpace(trimmed[hashes:])
		}
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")
		trimmed = strings.ReplaceAll(trimmed, "`", "")
		lines[i] = strings.TrimSpace(trimmed)
	}
	return strings.Join(lines, "\n")
}

// firstSentence 提取文本的首行或首句。
//
// 先按换行截取首行（思考文本通常逐行组织，首行多为概要），
// 再以中英文句末标点（。！？!?）为界截取首句；无边界时返回整体
// （最终长度由 truncateRunes 兜底）。
// 不使用英文句点 '.' 作为边界，避免误伤小数、缩写与文件扩展名。
func firstSentence(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)
	for i, r := range text {
		switch r {
		case '。', '！', '？', '!', '?':
			return strings.TrimSpace(text[:i+utf8.RuneLen(r)])
		}
	}
	return text
}

// hasMeaningfulRune 判断文本是否包含至少一个字母或数字。
//
// 纯标点/符号内容（分隔线、代码片段残骸等）不适合作为摘要。
func hasMeaningfulRune(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// truncateRunes 按 rune 数截断文本，超长时追加省略号。
func truncateRunes(text string, max int) string {
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "…"
}

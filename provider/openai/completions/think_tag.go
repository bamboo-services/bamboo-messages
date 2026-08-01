package completions

import (
	"strings"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// 内联 think 标签字面量。
//
// 部分 OpenAI 兼容端点（DeepSeek-R1 早期格式、GLM、QwQ 及部分代理转换层）
// 将推理内容以 XML 风格标签包裹后混入 content 字段，而非使用标准的
// reasoning_content 字段。标签常量以拼接方式定义，避免源码中出现
// 完整字面量被工具链误解析。
const (
	thinkOpenTag  = "<" + "think>"
	thinkCloseTag = "<" + "/think>"
)

// thinkTagStripper 从流式 content 增量中剥离内联 think 标签。
//
// 状态机逐字符扫描 content 增量，维护两种模式：
//   - 文本模式：普通内容作为 text_output 事件输出；遇到开标签时切换为
//     thinking 模式并发出 thinking 块开始事件
//   - thinking 模式：内容作为 thinking 事件输出；遇到闭标签时切回文本模式
//
// 跨 chunk 边界的标签（如 "<th" + "ink>"）通过 buf 缓冲识别；
// 非标签的 "<" 候选（如 "a < b"）在候选失败时原样释放，不误伤正文。
// 文本模式下出现的孤立闭标签（上游经 reasoning_content 传输推理后
// 泄漏到 content 的残余）会被静默吞掉。
//
// 非并发安全，与 Chat 的 SSE 事件循环在同一 goroutine 内使用。
type thinkTagStripper struct {
	thinking bool   // 当前是否处于 thinking 模式
	buf      string // 候选标签缓冲区（以 "<" 开头的潜在标签前缀）
}

// newThinkTagStripper 创建内联 think 标签剥离器。
func newThinkTagStripper() *thinkTagStripper {
	return &thinkTagStripper{}
}

// process 处理单个 content 增量，返回剥离后的事件序列。
//
// 返回的事件仅包含 BlockStart(thinking)、ThinkingDelta 和 TextDelta 三类；
// BlockStart(text) 不由剥离器发出，由适配器按既有 BlockStart 契约合成。
func (s *thinkTagStripper) process(chunk string) []provider.StreamEvent {
	var events []provider.StreamEvent
	var pending strings.Builder // 当前模式下的连续内容累积

	// flushPending 将累积内容按当前模式输出为对应类型事件
	flushPending := func() {
		if pending.Len() == 0 {
			return
		}
		text := pending.String()
		pending.Reset()
		if s.thinking {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewThinkingDelta(text),
			})
			return
		}
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta(text),
		})
	}

	// flushBuf 将候选缓冲区作为普通内容释放（候选标签识别失败时调用）
	flushBuf := func() {
		if s.buf == "" {
			return
		}
		pending.WriteString(s.buf)
		s.buf = ""
	}

	for i := 0; i < len(chunk); i++ {
		c := chunk[i]

		if s.buf != "" {
			// 正在识别候选标签：累积字符并校验是否仍为标签前缀
			s.buf += string(c)
			tag := thinkOpenTag
			if s.thinking {
				tag = thinkCloseTag
			}
			if isTagPrefix(s.buf, tag) {
				if s.buf == tag {
					// 完整标签识别成功
					s.buf = ""
					if s.thinking {
						// 闭标签：退出 thinking 模式，标签本身不产生事件
						s.thinking = false
					} else {
						// 开标签：进入 thinking 模式，发出块开始事件
						flushPending()
						s.thinking = true
						events = append(events, provider.StreamEvent{
							Type:  provider.StreamTypeDelta,
							Delta: provider.NewBlockStartDelta("thinking"),
						})
					}
				}
				continue
			}
			// 候选失败：缓冲区不是当前模式标签的前缀。
			// 取缓冲区的最长标签后缀重新候选化（KMP 回退思路）——
			// 正文与标签粘连时（如正文紧贴孤立闭标签），让标签在缓冲区
			// 末尾独立成形后被识别，否则会随正文作为字面文本输出。
			if idx := longestTagSuffix(s.buf); idx >= 0 {
				pending.WriteString(s.buf[:idx])
				s.buf = s.buf[idx:]
				// 重候选后缓冲区可能恰好是完整孤立闭标签（正文粘连且位于
				// chunk 末尾，不再有后续字符触发完成判定），直接吞掉
				if !s.thinking && s.buf == thinkCloseTag {
					s.buf = ""
				}
			} else {
				flushBuf()
			}
			continue
		}

		if c == '<' {
			// 潜在标签起点：先输出已累积内容，进入候选识别
			flushPending()
			s.buf = string(c)
			continue
		}

		pending.WriteByte(c)
	}

	flushPending()
	return events
}

// flush 在流结束时释放缓冲区残余。
//
// 不完整的候选标签（如正文末尾的孤立 "<"）按文本输出，保证内容不丢失；
// thinking 模式下缓冲区只可能是未完成的闭标签前缀，作为 thinking 内容输出。
func (s *thinkTagStripper) flush() []provider.StreamEvent {
	if s.buf == "" {
		return nil
	}
	text := s.buf
	s.buf = ""
	if s.thinking {
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(text),
		}}
	}
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta(text),
	}}
}

// isTagPrefix 判断 buf 是否为 tag 的前缀。
func isTagPrefix(buf, tag string) bool {
	return len(buf) <= len(tag) && tag[:len(buf)] == buf
}

// longestTagSuffix 返回 buf 的最长后缀起始下标，使其同时为 thinkOpenTag
// 或 thinkCloseTag 的前缀（即以 '<' 起始的潜在标签片段）。
//
// 两个标签共享 "<" + "think" 前缀，任何合法候选必然以 '<' 开头，
// 因此从右向左找到最后一个 '<' 后校验其后缀是否为任一标签的前缀即可。
// 下标 0 同样参与校验：整个缓冲区作为候选保持悬挂（如孤立闭标签逐字符
// 累积的场景），由调用方的完整标签判定与 flush 兜底处理。
// 不存在合法后缀时返回 -1。
func longestTagSuffix(buf string) int {
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i] != '<' {
			continue
		}
		suffix := buf[i:]
		if isTagPrefix(suffix, thinkOpenTag) || isTagPrefix(suffix, thinkCloseTag) {
			return i
		}
	}
	return -1
}

// stripThinkTagsFromContent 从非流式完整 content 中剥离内联 think 标签。
//
// 配对的开闭标签之间的内容作为推理内容返回（由调用方合并到
// CompletionResult.Thinking）；标签本身从返回值中移除。
// 防御性处理两类非标准情况：
//   - 孤立闭标签（reasoning_content 泄漏残余）：直接移除
//   - 未闭合的开标签（流被截断）：开标签之后的全部内容视为推理内容
func stripThinkTagsFromContent(content string, thinking *string) string {
	var text strings.Builder
	rest := content
	for {
		openIdx := strings.Index(rest, thinkOpenTag)
		closeIdx := strings.Index(rest, thinkCloseTag)

		// 无闭标签：剥离孤立闭标签后结束
		if closeIdx < 0 {
			if openIdx < 0 {
				text.WriteString(rest)
				break
			}
			// 未闭合开标签：之前为正文，之后全部为推理内容
			text.WriteString(rest[:openIdx])
			*thinking += rest[openIdx+len(thinkOpenTag):]
			break
		}

		// 闭标签先于开标签出现（或无开标签）：孤立闭标签，移除后继续
		if openIdx < 0 || closeIdx < openIdx {
			text.WriteString(rest[:closeIdx])
			rest = rest[closeIdx+len(thinkCloseTag):]
			continue
		}

		// 配对标签：提取中间推理内容，继续扫描剩余部分
		text.WriteString(rest[:openIdx])
		*thinking += rest[openIdx+len(thinkOpenTag) : closeIdx]
		rest = rest[closeIdx+len(thinkCloseTag):]
	}
	return text.String()
}

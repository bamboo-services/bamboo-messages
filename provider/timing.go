package provider

import (
	"math"
	"time"
	"unicode"
)

// ════════════════════════════════════════════════════════════════════════════
// 公共类型定义
// ════════════════════════════════════════════════════════════════════════════

// RateSampleKind 速率采样类型，区分思考阶段、输出阶段和工具调用阶段的 token 速率。
type RateSampleKind string

const (
	// RateSampleKindThinking 思考阶段速率采样（对应 thinking delta 的 token/s）。
	RateSampleKindThinking RateSampleKind = "thinking"

	// RateSampleKindOutput 输出阶段速率采样（对应 text delta 的 token/s）。
	RateSampleKindOutput RateSampleKind = "output"

	// RateSampleKindTool 工具调用阶段速率采样（对应 tool call delta 的 token/s）。
	RateSampleKindTool RateSampleKind = "tool"
)

// TimingStats 流式请求耗时统计。
//
// 记录单次流式请求中各阶段的耗时，用于性能审计和监控。
// 所有 Duration 字段在对应阶段未发生时为零值。
type TimingStats struct {
	// TotalDuration 总耗时 — 从 StreamTypeStart 到 StreamTypeStop。
	TotalDuration time.Duration

	// FirstByteDuration 首字耗时（TTFT）— 从 Start 到第一个内容 Delta（含思考）。
	FirstByteDuration time.Duration

	// ThinkingDuration 思考阶段耗时 — 从首个 thinking BlockStart 到思考阶段结束（即 text BlockStart 或 Stop）。
	ThinkingDuration time.Duration

	// ContentDuration 文本内容阶段耗时 — 从首个 text BlockStart 到内容阶段结束（即 tool BlockStart 或 Stop）。
	ContentDuration time.Duration

	// ToolDuration 工具调用阶段耗时 — 从首个 tool BlockStart/ToolCall 到 Stop。
	ToolDuration time.Duration

	// ThinkingTokens 思考阶段估算 token 数（基于 charCounter，CJK 1:1, Latin 4:1, Other 2:1）。
	ThinkingTokens int64

	// OutputTokens 输出阶段估算 token 数。
	OutputTokens int64

	// ToolTokens 工具调用阶段估算 token 数。
	ToolTokens int64

	// TotalTokens 总 token 数。provider 模式下 = UsageData.OutputTokens；calculate 模式下 = 三段合计。
	TotalTokens int64

	// TokenSource token 数据来源："provider"（Provider 上报）或 "calculate"（本地估算）。
	TokenSource string
}

// TokenRates Token 生成速率（.2f 精度）。
//
// 基于 TimingStats 的阶段耗时和字符计数估算的 token/s 速率。
// Token 数按 CJK ≈ 1 char/token、Latin ≈ 4 chars/token 估算。
type TokenRates struct {
	// ThinkingTokensPerSec 思考阶段的 token 生成速率。
	ThinkingTokensPerSec float64

	// OutputTokensPerSec 输出阶段的 token 生成速率。
	OutputTokensPerSec float64

	// ToolTokensPerSec 工具调用阶段的 token 生成速率。
	ToolTokensPerSec float64
}

// RateSample 速率采样点 — 记录某一时刻的 token/s 速率。
//
// 仅在启用 SmoothBuffer 时由 SmoothPacer 产生，
// 每次输出 tick 产生一条采样，记录该 tick 的瞬时速率和对应的流式经过秒数。
type RateSample struct {
	// ElapsedSec 从流开始到本采样点的经过秒数。
	ElapsedSec float64

	// TokensPerSec 该时刻的瞬时 token 生成速率。
	TokensPerSec float64

	// Kind 采样类型：thinking 或 output。
	Kind RateSampleKind
}

// ════════════════════════════════════════════════════════════════════════════
// 内部辅助
// ════════════════════════════════════════════════════════════════════════════

// collectorPhase 流式阶段标记。
type collectorPhase int

const (
	phaseInit     collectorPhase = iota // 初始状态
	phaseThinking                       // 思考阶段
	phaseContent                        // 内容输出阶段
	phaseTool                           // 工具调用阶段
)

// charCounter 按 CJK/Latin 分类统计字符，用于估算 token 数。
//
// 估算规则：
//   - CJK 字符（汉字/平假名/片假名/谚文）：≈ 1 token/char
//   - Latin 字母数字：≈ 4 chars/token
//   - 其他字符（标点/符号）：≈ 2 chars/token
type charCounter struct {
	cjk   int64
	latin int64
	other int64
}

// add 累加一段文本的字符统计。
func (c *charCounter) add(text string) {
	for _, r := range text {
		switch {
		case isCJKRune(r):
			c.cjk++
		case isLatinAlnumRune(r):
			c.latin++
		default:
			c.other++
		}
	}
}

// estimateTokens 基于字符统计估算 token 数。
func (c *charCounter) estimateTokens() int64 {
	return c.cjk + c.latin/4 + c.other/2
}

// isCJKRune 判断是否为 CJK 字符（汉字/平假名/片假名/谚文）。
func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// isLatinAlnumRune 判断是否为 ASCII Latin 字母或数字。
func isLatinAlnumRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

// round2 将浮点数四舍五入到小数点后两位（.2f 精度）。
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// ════════════════════════════════════════════════════════════════════════════
// TimingCollector
// ════════════════════════════════════════════════════════════════════════════

// TimingCollector 流式请求耗时收集器。
//
// 零侵入设计：用户在遍历 provider.StreamEvent channel 时调用 Observe(event)，
// 流结束后通过 Stats() / Rates() / RateSeries() 提取审计数据。
//
// 非并发安全 — 仅在读取流事件的同一 goroutine 中使用。
//
// 使用示例：
//
//	collector := provider.NewTimingCollector()
//	for event := range ch {
//	    collector.Observe(event)
//	    // ... 正常处理事件
//	}
//	stats := collector.Stats()
//	rates := collector.Rates()
//	series := collector.RateSeries() // 仅 SmoothBuffer 启用时非 nil
type TimingCollector struct {
	// 时间戳
	startTime     time.Time // StreamTypeStart 时刻
	firstByteTime time.Time // 第一个 Delta 时刻
	stopTime      time.Time // StreamTypeStop 时刻
	lastEventTime time.Time // 最后一个事件时刻（取消时回退使用）

	// 阶段时间戳
	thinkingStart time.Time // 首个 thinking BlockStart 时刻
	thinkingEnd   time.Time // 思考阶段结束时刻（text BlockStart 或后续阶段开始）
	contentStart  time.Time // 首个 text BlockStart 时刻
	contentEnd    time.Time // 内容阶段结束时刻（tool BlockStart 或 Stop）
	toolStart     time.Time // 首个 tool 事件时刻

	// 阶段标记
	phase collectorPhase

	// 字符计数（用于 token 估算）
	thinkingChars charCounter
	outputChars   charCounter
	toolChars      charCounter

	// Token 用量（来自 UsageDelta，可选）
	usage *UsageData

	// 速率采样序列（来自 SmoothPacer 回调）
	rateSamples []RateSample
}

// NewTimingCollector 创建耗时收集器。
func NewTimingCollector() *TimingCollector {
	return &TimingCollector{
		phase: phaseInit,
	}
}

// Observe 观察一个流事件，更新内部计时状态。
//
// 对每个从 provider.Chat() 返回的 StreamEvent 调用此方法。
// 方法内部根据事件类型和 Delta 类型驱动状态机转换。
func (tc *TimingCollector) Observe(event StreamEvent) {
	now := time.Now()
	tc.lastEventTime = now

	// 缺失 Start 事件时，fallback startTime 到首个非 Start 事件时间
	if tc.startTime.IsZero() && event.Type != StreamTypeStart {
		tc.startTime = now
	}

	switch event.Type {
	case StreamTypeStart:
		if tc.startTime.IsZero() {
			tc.startTime = now
		}

	case StreamTypeDelta:
		tc.handleDelta(event.Delta, now)

	case StreamTypeStop:
		tc.stopTime = now
	}
}

// handleDelta 处理 Delta 事件，驱动阶段状态机。
func (tc *TimingCollector) handleDelta(delta StreamDelta[any], now time.Time) {
	// 首字时间：第一个 Delta（含思考），即标准 TTFT
	if tc.firstByteTime.IsZero() {
		tc.firstByteTime = now
	}

	switch delta.Type {
	case StreamDeltaTypeBlockStart:
		if bs, ok := delta.Data.(BlockStartData); ok {
			switch bs.BlockType {
			case "thinking":
				if tc.thinkingStart.IsZero() {
					tc.thinkingStart = now
				}
				tc.phase = phaseThinking

			case "text":
				if tc.contentStart.IsZero() {
					tc.contentStart = now
				}
				// 结束思考阶段
				if tc.thinkingEnd.IsZero() && !tc.thinkingStart.IsZero() {
					tc.thinkingEnd = now
				}
				tc.phase = phaseContent

			case "tool_use":
				if tc.toolStart.IsZero() {
					tc.toolStart = now
				}
				// 结束内容阶段
				if tc.contentEnd.IsZero() && !tc.contentStart.IsZero() {
					tc.contentEnd = now
				}
				tc.phase = phaseTool
			}
		}

	case StreamDeltaTypeThinking:
		// 适配器可能不发 BlockStart("thinking")，首个 ThinkingDelta 时回退设置 thinkingStart
		if tc.thinkingStart.IsZero() {
			tc.thinkingStart = now
		}
		if data, ok := delta.Data.(ThinkingData); ok {
			tc.thinkingChars.add(string(data))
		}

	case StreamDeltaTypeTextOutput:
		// 适配器可能不发 BlockStart("text")，首个 TextDelta 时回退设置 contentStart
		if tc.contentStart.IsZero() {
			tc.contentStart = now
		}
		if tc.contentEnd.IsZero() && !tc.thinkingStart.IsZero() && tc.thinkingEnd.IsZero() {
			tc.thinkingEnd = now
		}
		if data, ok := delta.Data.(TextData); ok {
			tc.outputChars.add(string(data))
		}

	case StreamDeltaTypeToolCall:
		// Gemini 可能不发送 BlockStart("tool_use")，直接发 ToolCallDelta
		if tc.toolStart.IsZero() {
			tc.toolStart = now
		}
		if tc.contentEnd.IsZero() && !tc.contentStart.IsZero() {
			tc.contentEnd = now
		}
		tc.phase = phaseTool

	case StreamDeltaTypeToolCallDelta:
		if tc.toolStart.IsZero() {
			tc.toolStart = now
		}
		if tc.contentEnd.IsZero() && !tc.contentStart.IsZero() {
			tc.contentEnd = now
		}
		tc.phase = phaseTool
		switch data := delta.Data.(type) {
		case ToolCallDeltaData:
			tc.toolChars.add(string(data))
		case IndexedToolCallDeltaData:
			tc.toolChars.add(data.PartialJSON)
		}

	case StreamDeltaTypeUsage:
		if data, ok := delta.Data.(UsageData); ok {
			u := data
			tc.usage = &u
		}
	}
}

// RecordRateSample 记录一个速率采样点。
//
// 由 SmoothPacer 的速率回调（relay.WithRateSampleCallback）调用，
// 将平滑缓冲器每次 tick 的瞬时速率追加到采样序列。
// elapsedSec 和 tokensPerSec 会被四舍五入到 .2f 精度。
func (tc *TimingCollector) RecordRateSample(elapsedSec, tokensPerSec float64, kind RateSampleKind) {
	tc.rateSamples = append(tc.rateSamples, RateSample{
		ElapsedSec:   round2(elapsedSec),
		TokensPerSec: round2(tokensPerSec),
		Kind:         kind,
	})
}

// Stats 返回耗时统计。
//
// 各阶段 Duration 在对应阶段未发生时为零值。
// 流取消（无 StreamTypeStop）时，使用最后一个事件的时刻作为结束时间回退，
// 保证已记录的数据不丢失。
func (tc *TimingCollector) Stats() TimingStats {
	var stats TimingStats

	// effectiveEndTime 确定有效结束时间：优先 stopTime，回退 lastEventTime（取消场景）
	endTime := tc.stopTime
	if endTime.IsZero() {
		endTime = tc.lastEventTime
	}

	// 总耗时
	if !tc.startTime.IsZero() && !endTime.IsZero() {
		stats.TotalDuration = endTime.Sub(tc.startTime)
	}

	// 首字耗时
	if !tc.startTime.IsZero() && !tc.firstByteTime.IsZero() {
		stats.FirstByteDuration = tc.firstByteTime.Sub(tc.startTime)
	}

	// 思考阶段耗时
	if !tc.thinkingStart.IsZero() {
		end := tc.thinkingEnd
		if end.IsZero() {
			if !tc.contentStart.IsZero() {
				end = tc.contentStart
			} else if !tc.toolStart.IsZero() {
				end = tc.toolStart
			} else {
				end = endTime
			}
		}
		if !end.IsZero() {
			stats.ThinkingDuration = end.Sub(tc.thinkingStart)
		}
	}

	// 内容阶段耗时
	if !tc.contentStart.IsZero() {
		end := tc.contentEnd
		if end.IsZero() {
			if !tc.toolStart.IsZero() {
				end = tc.toolStart
			} else {
				end = endTime
			}
		}
		if !end.IsZero() {
			stats.ContentDuration = end.Sub(tc.contentStart)
		}
	}

	// 工具阶段耗时
	if !tc.toolStart.IsZero() && !endTime.IsZero() {
		stats.ToolDuration = endTime.Sub(tc.toolStart)
	}

	// Token 计数与来源
	stats.ThinkingTokens = tc.thinkingChars.estimateTokens()
	stats.OutputTokens = tc.outputChars.estimateTokens()
	stats.ToolTokens = tc.toolChars.estimateTokens()
	if tc.usage != nil {
		stats.TokenSource = "provider"
		stats.TotalTokens = tc.usage.OutputTokens
	} else {
		stats.TokenSource = "calculate"
		stats.TotalTokens = stats.ThinkingTokens + stats.OutputTokens + stats.ToolTokens
	}

	return stats
}

// Rates 返回 Token 生成速率（.2f 精度）。
//
// Token 数基于字符类型估算：CJK ≈ 1 char/token，Latin ≈ 4 chars/token。
// 速率为零值表示对应阶段未发生或耗时为零。
func (tc *TimingCollector) Rates() TokenRates {
	var rates TokenRates
	stats := tc.Stats()

	if stats.ThinkingDuration > 0 {
		tokens := tc.thinkingChars.estimateTokens()
		rates.ThinkingTokensPerSec = round2(float64(tokens) / stats.ThinkingDuration.Seconds())
	}

	if stats.ContentDuration > 0 {
		tokens := tc.outputChars.estimateTokens()
		rates.OutputTokensPerSec = round2(float64(tokens) / stats.ContentDuration.Seconds())
	}

	if stats.ToolDuration > 0 {
		tokens := tc.toolChars.estimateTokens()
		rates.ToolTokensPerSec = round2(float64(tokens) / stats.ToolDuration.Seconds())
	}

	return rates
}

// RateSeries 返回速率采样序列。
//
// 仅在启用 SmoothBuffer 并设置了 relay.WithRateSampleCallback 时有值。
// 返回的是切片副本，修改不影响内部状态。
// 未启用时返回 nil。
func (tc *TimingCollector) RateSeries() []RateSample {
	if len(tc.rateSamples) == 0 {
		return nil
	}
	result := make([]RateSample, len(tc.rateSamples))
	copy(result, tc.rateSamples)
	return result
}

// Usage 返回最后收到的 UsageDelta 数据（如有）。
//
// 部分适配器在流中发送 UsageDelta，部分在 Stop 事件中携带。
// 为 nil 表示流中未收到用量统计。
func (tc *TimingCollector) Usage() *UsageData {
	return tc.usage
}

package provider

import (
	"strings"
	"testing"
	"time"
)

// makeEvent 构造辅助函数。
func makeEvent(t StreamType) StreamEvent {
	return StreamEvent{Type: t}
}

func makeDeltaEvent(dt StreamDeltaType, data any) StreamEvent {
	return StreamEvent{
		Type: StreamTypeDelta,
		Delta: StreamDelta[any]{
			Type: dt,
			Data: data,
		},
	}
}

func makeStopEvent() StreamEvent {
	return StreamEvent{Type: StreamTypeStop}
}

func TestTimingCollector_FullStream(t *testing.T) {
	tc := NewTimingCollector()

	// Start
	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(5 * time.Millisecond)

	// Thinking BlockStart + ThinkingDelta
	tc.Observe(makeDeltaEvent(StreamDeltaTypeBlockStart, BlockStartData{BlockType: "thinking"}))
	time.Sleep(10 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeThinking, ThinkingData("正在分析问题")))
	time.Sleep(5 * time.Millisecond)

	// Text BlockStart + TextDelta
	tc.Observe(makeDeltaEvent(StreamDeltaTypeBlockStart, BlockStartData{BlockType: "text"}))
	time.Sleep(10 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("Hello world")))
	time.Sleep(5 * time.Millisecond)

	// ToolCall
	tc.Observe(makeDeltaEvent(StreamDeltaTypeToolCall, ToolCallData{ID: "tc1", Name: "search"}))
	time.Sleep(3 * time.Millisecond)

	// Stop
	tc.Observe(makeStopEvent())

	stats := tc.Stats()

	// 总耗时应大于 0
	if stats.TotalDuration <= 0 {
		t.Errorf("TotalDuration should be > 0, got %v", stats.TotalDuration)
	}

	// 首字耗时应 > 0 且 < 总耗时
	if stats.FirstByteDuration <= 0 {
		t.Errorf("FirstByteDuration should be > 0, got %v", stats.FirstByteDuration)
	}
	if stats.FirstByteDuration >= stats.TotalDuration {
		t.Errorf("FirstByteDuration (%v) should be < TotalDuration (%v)",
			stats.FirstByteDuration, stats.TotalDuration)
	}

	// 思考阶段耗时 > 0
	if stats.ThinkingDuration <= 0 {
		t.Errorf("ThinkingDuration should be > 0, got %v", stats.ThinkingDuration)
	}

	// 内容阶段耗时 > 0
	if stats.ContentDuration <= 0 {
		t.Errorf("ContentDuration should be > 0, got %v", stats.ContentDuration)
	}

	// 工具阶段耗时 > 0
	if stats.ToolDuration <= 0 {
		t.Errorf("ToolDuration should be > 0, got %v", stats.ToolDuration)
	}
}

func TestTimingCollector_TextOnlyStream(t *testing.T) {
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("hello")))
	time.Sleep(3 * time.Millisecond)

	tc.Observe(makeStopEvent())

	stats := tc.Stats()

	if stats.TotalDuration <= 0 {
		t.Errorf("TotalDuration should be > 0, got %v", stats.TotalDuration)
	}
	if stats.FirstByteDuration <= 0 {
		t.Errorf("FirstByteDuration should be > 0")
	}
	// 没有思考阶段
	if stats.ThinkingDuration != 0 {
		t.Errorf("ThinkingDuration should be 0 for text-only stream, got %v", stats.ThinkingDuration)
	}
	// 没有工具阶段
	if stats.ToolDuration != 0 {
		t.Errorf("ToolDuration should be 0 for text-only stream, got %v", stats.ToolDuration)
	}
}

func TestTimingCollector_ThinkingWithoutTextBlockStart(t *testing.T) {
	// 测试适配器不发 BlockStart("text") 但直接发 TextDelta 的情况
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(5 * time.Millisecond)

	// 直接 ThinkingDelta（无 BlockStart）
	tc.Observe(makeDeltaEvent(StreamDeltaTypeThinking, ThinkingData("思考中")))
	time.Sleep(5 * time.Millisecond)

	// 直接 TextDelta（无 BlockStart）
	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("回复")))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeStopEvent())

	stats := tc.Stats()

	// 首字耗时应记录（第一个 Delta）
	if stats.FirstByteDuration <= 0 {
		t.Errorf("FirstByteDuration should be > 0")
	}
}

func TestTimingCollector_GeminiToolCallWithoutBlockStart(t *testing.T) {
	// Gemini 直接发 ToolCallDelta 不带 BlockStart("tool_use")
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeBlockStart, BlockStartData{BlockType: "text"}))
	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("使用工具")))
	time.Sleep(5 * time.Millisecond)

	// 直接 ToolCallDelta（无 BlockStart("tool_use")）
	tc.Observe(makeDeltaEvent(StreamDeltaTypeToolCall, ToolCallData{ID: "tc1", Name: "calc"}))
	time.Sleep(3 * time.Millisecond)

	tc.Observe(makeStopEvent())

	stats := tc.Stats()

	if stats.ToolDuration <= 0 {
		t.Errorf("ToolDuration should be > 0 for Gemini-style ToolCall, got %v", stats.ToolDuration)
	}
}

func TestTimingCollector_Rates(t *testing.T) {
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(2 * time.Millisecond)

	// 思考阶段
	tc.Observe(makeDeltaEvent(StreamDeltaTypeBlockStart, BlockStartData{BlockType: "thinking"}))
	time.Sleep(10 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeThinking, ThinkingData("正在分析这个问题，需要计算多个因素")))
	time.Sleep(5 * time.Millisecond)

	// 内容阶段
	tc.Observe(makeDeltaEvent(StreamDeltaTypeBlockStart, BlockStartData{BlockType: "text"}))
	time.Sleep(10 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("The answer is forty two")))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeStopEvent())

	rates := tc.Rates()

	// 思考 token/s 应 > 0
	if rates.ThinkingTokensPerSec <= 0 {
		t.Errorf("ThinkingTokensPerSec should be > 0, got %v", rates.ThinkingTokensPerSec)
	}

	// 输出 token/s 应 > 0
	if rates.OutputTokensPerSec <= 0 {
		t.Errorf("OutputTokensPerSec should be > 0, got %v", rates.OutputTokensPerSec)
	}

	// 验证 .2f 精度（小数点后不超过 2 位）
	for _, r := range []float64{rates.ThinkingTokensPerSec, rates.OutputTokensPerSec} {
		rounded := float64(int64(r*100)) / 100
		diff := r - rounded
		if diff > 0.001 || diff < -0.001 {
			t.Errorf("Rate %v should be rounded to .2f precision", r)
		}
	}
}

func TestTimingCollector_RatesWithNoThinking(t *testing.T) {
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(2 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("hello")))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeStopEvent())

	rates := tc.Rates()

	if rates.ThinkingTokensPerSec != 0 {
		t.Errorf("ThinkingTokensPerSec should be 0 when no thinking, got %v", rates.ThinkingTokensPerSec)
	}
	if rates.OutputTokensPerSec <= 0 {
		t.Errorf("OutputTokensPerSec should be > 0, got %v", rates.OutputTokensPerSec)
	}
}

func TestTimingCollector_RateSeries(t *testing.T) {
	tc := NewTimingCollector()

	// 模拟 SmoothPacer 回调
	tc.RecordRateSample(1.0, 15.50, RateSampleKindThinking)
	tc.RecordRateSample(2.0, 22.30, RateSampleKindThinking)
	tc.RecordRateSample(3.0, 18.00, RateSampleKindOutput)
	tc.RecordRateSample(4.5, 25.75, RateSampleKindOutput)

	series := tc.RateSeries()

	if len(series) != 4 {
		t.Fatalf("Expected 4 samples, got %d", len(series))
	}

	// 验证第一条
	if series[0].ElapsedSec != 1.0 {
		t.Errorf("Expected ElapsedSec 1.0, got %v", series[0].ElapsedSec)
	}
	if series[0].TokensPerSec != 15.5 {
		t.Errorf("Expected TokensPerSec 15.5, got %v", series[0].TokensPerSec)
	}
	if series[0].Kind != RateSampleKindThinking {
		t.Errorf("Expected Kind %v, got %v", RateSampleKindThinking, series[0].Kind)
	}

	// 验证类型切换
	if series[2].Kind != RateSampleKindOutput {
		t.Errorf("Expected Kind %v at index 2, got %v", RateSampleKindOutput, series[2].Kind)
	}

	// 验证最后一条
	if series[3].ElapsedSec != 4.5 {
		t.Errorf("Expected ElapsedSec 4.5, got %v", series[3].ElapsedSec)
	}
	if series[3].TokensPerSec != 25.75 {
		t.Errorf("Expected TokensPerSec 25.75, got %v", series[3].TokensPerSec)
	}
}

func TestTimingCollector_RateSeriesNil(t *testing.T) {
	tc := NewTimingCollector()
	series := tc.RateSeries()
	if series != nil {
		t.Errorf("Expected nil RateSeries when no samples recorded, got %v", series)
	}
}

func TestTimingCollector_RateSeriesCopy(t *testing.T) {
	tc := NewTimingCollector()
	tc.RecordRateSample(1.0, 10.0, RateSampleKindOutput)

	series := tc.RateSeries()
	series[0].TokensPerSec = 999.0 // 修改返回值不应影响内部状态

	series2 := tc.RateSeries()
	if series2[0].TokensPerSec == 999.0 {
		t.Errorf("RateSeries should return a copy, not internal slice")
	}
}

func TestTimingCollector_UsageCapture(t *testing.T) {
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(2 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("hi")))
	time.Sleep(2 * time.Millisecond)

	// UsageDelta
	tc.Observe(makeDeltaEvent(StreamDeltaTypeUsage, UsageData{
		InputTokens:  100,
		OutputTokens: 50,
	}))

	tc.Observe(makeStopEvent())

	usage := tc.Usage()
	if usage == nil {
		t.Fatal("Expected non-nil Usage")
	}
	if usage.InputTokens != 100 {
		t.Errorf("Expected InputTokens 100, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("Expected OutputTokens 50, got %d", usage.OutputTokens)
	}
}

func TestTimingCollector_UsageNil(t *testing.T) {
	tc := NewTimingCollector()
	if tc.Usage() != nil {
		t.Errorf("Expected nil Usage on fresh collector")
	}
}

func TestCharCounter_EstimateTokens(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantTokens int64
	}{
		{
			name:       "pure CJK",
			text:       "你好世界",
			wantTokens: 4, // 4 CJK chars = 4 tokens
		},
		{
			name:       "pure Latin",
			text:       "hello",
			wantTokens: 1, // 5 latin chars / 4 = 1 (integer division)
		},
		{
			name:       "mixed CJK and Latin",
			text:       "你好hello世界world",
			wantTokens: 4 + 5/4 + 5/4, // 4 CJK + 1 + 1 = 6
		},
		{
			name:       "empty string",
			text:       "",
			wantTokens: 0,
		},
		{
			name:       "punctuation only",
			text:       "！！！",
			wantTokens: 3 / 2, // 3 other chars / 2 = 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c charCounter
			c.add(tt.text)
			got := c.estimateTokens()
			if got != tt.wantTokens {
				t.Errorf("estimateTokens() = %d, want %d", got, tt.wantTokens)
			}
		})
	}
}

func TestRound2(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{15.556, 15.56},
		{15.554, 15.55},
		{22.306, 22.31},
		{0.0, 0.0},
		{99.999, 100.0},
		{15.50, 15.5},
		{1.006, 1.01},
	}

	for _, tt := range tests {
		got := round2(tt.input)
		if got != tt.want {
			t.Errorf("round2(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTimingCollectorMissingStart(t *testing.T) {
	// 适配器未发送 Start 事件时，fallback startTime 到首个非 Start 事件时间
	tc := NewTimingCollector()

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("hello")))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeStopEvent())

	stats := tc.Stats()

	if stats.TotalDuration <= 0 {
		t.Errorf("TotalDuration should be > 0 when Start event is missing, got %v", stats.TotalDuration)
	}
	if stats.FirstByteDuration != 0 {
		t.Errorf("FirstByteDuration should be 0 when first event is also the first byte, got %v", stats.FirstByteDuration)
	}
	if stats.ContentDuration <= 0 {
		t.Errorf("ContentDuration should be > 0 when Start event is missing, got %v", stats.ContentDuration)
	}
}

func TestTimingCollector_NoStopEvent(t *testing.T) {
	// 流中断（无 Stop 事件）— 取消场景应保留已有记录
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("partial1")))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("partial2")))
	time.Sleep(3 * time.Millisecond)

	// 模拟 channel 直接 close，没有 StreamTypeStop

	stats := tc.Stats()

	// 取消场景：TotalDuration 应回退到最后一个事件时刻，不为零
	if stats.TotalDuration <= 0 {
		t.Errorf("TotalDuration should be > 0 on cancel (fallback to lastEventTime), got %v", stats.TotalDuration)
	}
	if stats.FirstByteDuration <= 0 {
		t.Errorf("FirstByteDuration should be recorded")
	}
	if stats.ContentDuration <= 0 {
		t.Errorf("ContentDuration should be recorded on cancel, got %v", stats.ContentDuration)
	}

	rates := tc.Rates()
	if rates.OutputTokensPerSec <= 0 {
		t.Errorf("OutputTokensPerSec should be > 0 on cancel, got %v", rates.OutputTokensPerSec)
	}
}

func TestTimingCollector_CancelDuringThinking(t *testing.T) {
	// 流在思考阶段被取消
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeBlockStart, BlockStartData{BlockType: "thinking"}))
	time.Sleep(10 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeThinking, ThinkingData("正在思考但被中断了")))
	time.Sleep(3 * time.Millisecond)

	// 没有 Stop，模拟取消

	stats := tc.Stats()

	if stats.TotalDuration <= 0 {
		t.Errorf("TotalDuration should be > 0 on cancel during thinking")
	}
	if stats.ThinkingDuration <= 0 {
		t.Errorf("ThinkingDuration should be > 0 on cancel during thinking, got %v", stats.ThinkingDuration)
	}

	rates := tc.Rates()
	if rates.ThinkingTokensPerSec <= 0 {
		t.Errorf("ThinkingTokensPerSec should be > 0 on cancel during thinking, got %v", rates.ThinkingTokensPerSec)
	}
}

func TestTimingCollector_CancelDuringToolCall(t *testing.T) {
	// 流在工具调用阶段被取消
	tc := NewTimingCollector()

	tc.Observe(makeEvent(StreamTypeStart))
	time.Sleep(3 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeBlockStart, BlockStartData{BlockType: "text"}))
	tc.Observe(makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("调用工具")))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeToolCall, ToolCallData{ID: "tc1", Name: "search"}))
	time.Sleep(5 * time.Millisecond)

	tc.Observe(makeDeltaEvent(StreamDeltaTypeToolCallDelta, ToolCallDeltaData(`{"q":"test`)))
	time.Sleep(3 * time.Millisecond)

	// 没有 Stop，模拟取消

	stats := tc.Stats()

	if stats.TotalDuration <= 0 {
		t.Errorf("TotalDuration should be > 0 on cancel during tool call")
	}
	if stats.ToolDuration <= 0 {
		t.Errorf("ToolDuration should be > 0 on cancel during tool call, got %v", stats.ToolDuration)
	}
}

// BenchmarkTimingCollector_Observe 基准测试 Observe 方法性能开销。
func BenchmarkTimingCollector_Observe(b *testing.B) {
	tc := NewTimingCollector()
	event := makeDeltaEvent(StreamDeltaTypeTextOutput, TextData("hello world"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.Observe(event)
	}
}

// BenchmarkCharCounter 大文本字符统计基准。
func BenchmarkCharCounter(b *testing.B) {
	// 构造混合文本
	text := strings.Repeat("你好world", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var c charCounter
		c.add(text)
		_ = c.estimateTokens()
	}
}

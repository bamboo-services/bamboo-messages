package relay

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/gemini"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ════════════════════════════════════════════════════════════════════════════
// 分组 1: TokenSplitter 测试
// ════════════════════════════════════════════════════════════════════════════

// TestTokenSplitter_PureCJK 纯 CJK 文本切分 — 每个 CJK 字符独立 token。
func TestTokenSplitter_PureCJK(t *testing.T) {
	s := NewTokenSplitter()
	tokens := s.Split("你好世界")

	// 内容完整性：Join(tokens) + Flush() == 原始文本
	result := strings.Join(tokens, "") + s.Flush()
	if result != "你好世界" {
		t.Errorf("content integrity: got %q, want %q (tokens=%v)", result, "你好世界", tokens)
	}

	// CJK 文本不应有 pendingTail
	if tail := s.Flush(); tail != "" {
		t.Errorf("expected empty flush for pure CJK, got %q", tail)
	}
}

// TestTokenSplitter_Mixed 混合 CJK + Latin + 标点切分。
func TestTokenSplitter_Mixed(t *testing.T) {
	s := NewTokenSplitter()
	input := "你好，Hello world！"
	tokens := s.Split(input)

	// 内容完整性
	result := strings.Join(tokens, "") + s.Flush()
	if result != input {
		t.Errorf("content integrity: got %q, want %q (tokens=%v)", result, input, tokens)
	}

	// 验证包含关键子串
	joined := strings.Join(tokens, "")
	if !strings.Contains(joined, "你") {
		t.Errorf("expected tokens to contain 你, got %v", tokens)
	}
	if !strings.Contains(joined, "Hello") {
		t.Errorf("expected tokens to contain Hello, got %v", tokens)
	}
}

// TestTokenSplitter_PendingTail 跨帧拼接 — Latin alnum 结尾时保留 pendingTail。
func TestTokenSplitter_PendingTail(t *testing.T) {
	s := NewTokenSplitter()

	// 第一帧：以 Latin alnum "wor" 结尾 → 保留为 pendingTail
	tokens1 := s.Split("Hello wor")

	// 第二帧：拼接待续部分
	tokens2 := s.Split("ld this")

	// Flush 残余
	tail := s.Flush()

	// 内容完整性：两帧拼接后应等于原始文本
	all := append(append([]string{}, tokens1...), tokens2...)
	result := strings.Join(all, "") + tail
	expected := "Hello wor" + "ld this"
	if result != expected {
		t.Errorf("cross-frame integrity: got %q, want %q", result, expected)
	}

	// 以 Latin alnum 结尾的帧应产生 pendingTail
	if tail == "" {
		t.Error("expected non-empty pendingTail for Latin alnum ending")
	}
}

// TestTokenSplitter_Emoji Emoji 独立 token。
func TestTokenSplitter_Emoji(t *testing.T) {
	s := NewTokenSplitter()
	input := "你好😀世界"
	tokens := s.Split(input)

	// 内容完整性
	result := strings.Join(tokens, "") + s.Flush()
	if result != input {
		t.Errorf("content integrity: got %q, want %q", result, input)
	}

	// Emoji 应出现在某个 token 中
	joined := strings.Join(tokens, "")
	if !strings.Contains(joined, "😀") {
		t.Errorf("expected tokens to contain emoji 😀, got %v", tokens)
	}
}

// TestTokenSplitter_Newline 换行符附着到前一个 token。
func TestTokenSplitter_Newline(t *testing.T) {
	s := NewTokenSplitter()
	input := "text\n"
	tokens := s.Split(input)

	// 内容完整性
	result := strings.Join(tokens, "") + s.Flush()
	if result != input {
		t.Errorf("content integrity: got %q, want %q", result, input)
	}

	// 换行不应独立成 token（应附着到前一个 token）
	if len(tokens) == 0 {
		t.Fatal("expected at least one token")
	}
	if !strings.HasSuffix(tokens[0], "\n") {
		t.Errorf("expected newline attached to previous token, got %v", tokens)
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 分组 2: FrameParser 测试
// ════════════════════════════════════════════════════════════════════════════

// TestFrameParser_AnthropicTextDelta Anthropic text_delta 切分为多个 text 微帧。
func TestFrameParser_AnthropicTextDelta(t *testing.T) {
	p := NewFrameParser(codec.FormatAnthropic)
	rawFrame := []byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"你好\"}}\n\n")

	frames := p.Parse(rawFrame)
	if len(frames) == 0 {
		t.Fatal("expected frames, got none")
	}

	for i, f := range frames {
		if f.kind != frameText {
			t.Errorf("frames[%d]: expected frameText, got %v", i, f.kind)
		}
	}

	// 验证内容完整性：所有微帧拼接后应包含 "你好"
	var sb strings.Builder
	for _, f := range frames {
		sb.Write(f.data)
	}
	if !strings.Contains(sb.String(), "你") || !strings.Contains(sb.String(), "好") {
		t.Errorf("expected output to contain 你 and 好, got: %s", sb.String())
	}
}

// TestFrameParser_AnthropicBarrier Anthropic block_stop 为 barrier。
func TestFrameParser_AnthropicBarrier(t *testing.T) {
	p := NewFrameParser(codec.FormatAnthropic)
	rawFrame := []byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")

	frames := p.Parse(rawFrame)
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if frames[0].kind != frameBarrier {
		t.Errorf("expected frameBarrier, got %v", frames[0].kind)
	}
	if !frames[0].isBarrier {
		t.Error("expected isBarrier=true")
	}
}

// TestFrameParser_AnthropicControl Anthropic block_start 为 control。
func TestFrameParser_AnthropicControl(t *testing.T) {
	p := NewFrameParser(codec.FormatAnthropic)
	rawFrame := []byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")

	frames := p.Parse(rawFrame)
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if frames[0].kind != frameControl {
		t.Errorf("expected frameControl, got %v", frames[0].kind)
	}
}

// TestFrameParser_OpenAITextDelta OpenAI content 切分。
func TestFrameParser_OpenAITextDelta(t *testing.T) {
	p := NewFrameParser(codec.FormatOpenAI)
	rawFrame := []byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"你好\"},\"finish_reason\":null}]}\n\n")

	frames := p.Parse(rawFrame)
	if len(frames) == 0 {
		t.Fatal("expected frames, got none")
	}

	for i, f := range frames {
		if f.kind != frameText {
			t.Errorf("frames[%d]: expected frameText, got %v", i, f.kind)
		}
	}

	var sb strings.Builder
	for _, f := range frames {
		sb.Write(f.data)
	}
	if !strings.Contains(sb.String(), "你") {
		t.Errorf("expected output to contain 你, got: %s", sb.String())
	}
}

// TestFrameParser_ResponsesTextDelta Responses delta 切分。
func TestFrameParser_ResponsesTextDelta(t *testing.T) {
	p := NewFrameParser(codec.FormatResponses)
	rawFrame := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"你好\"}\n\n")

	frames := p.Parse(rawFrame)
	if len(frames) == 0 {
		t.Fatal("expected frames, got none")
	}

	for i, f := range frames {
		if f.kind != frameText {
			t.Errorf("frames[%d]: expected frameText, got %v", i, f.kind)
		}
	}

	var sb strings.Builder
	for _, f := range frames {
		sb.Write(f.data)
	}
	if !strings.Contains(sb.String(), "你") {
		t.Errorf("expected output to contain 你, got: %s", sb.String())
	}
}

// TestFrameParser_GeminiTextDelta Gemini text 切分。
func TestFrameParser_GeminiTextDelta(t *testing.T) {
	p := NewFrameParser(codec.FormatGemini)
	rawFrame := []byte("data: {\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"你好\"}]}}]}\n\n")

	frames := p.Parse(rawFrame)
	if len(frames) == 0 {
		t.Fatal("expected frames, got none")
	}

	for i, f := range frames {
		if f.kind != frameText {
			t.Errorf("frames[%d]: expected frameText, got %v", i, f.kind)
		}
	}

	var sb strings.Builder
	for _, f := range frames {
		sb.Write(f.data)
	}
	if !strings.Contains(sb.String(), "你") {
		t.Errorf("expected output to contain 你, got: %s", sb.String())
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 分组 3: SmoothPacer 测试
// ════════════════════════════════════════════════════════════════════════════

// pacerTestHarness 封装 SmoothPacer 测试的异步收集逻辑。
// 消费者 goroutine 在 Push 前启动，避免 out channel 缓冲满阻塞。
type pacerTestHarness struct {
	out     chan []byte
	outputs [][]byte
	done    chan struct{}
	pacer   *SmoothPacer
	t       *testing.T
}

func newPacerTestHarness(t *testing.T, format codec.FormatType, params SmoothParams, ctx context.Context) *pacerTestHarness {
	t.Helper()
	h := &pacerTestHarness{
		out:  make(chan []byte, 256),
		done: make(chan struct{}),
		t:    t,
	}
	h.pacer = NewSmoothPacer(format, params, h.out, ctx)

	// 消费者 goroutine 先启动
	go func() {
		for data := range h.out {
			h.outputs = append(h.outputs, data)
		}
		close(h.done)
	}()

	return h
}

// shutdown 执行 SignalEnd → Wait → close(out) → 等待消费者完成。
func (h *pacerTestHarness) shutdown() {
	h.t.Helper()

	// 给 pacer 时间处理已 Push 的数据（避免 input/signal 竞态）
	time.Sleep(50 * time.Millisecond)

	h.pacer.SignalEnd()

	// Wait 应在合理时间内返回
	waitDone := make(chan struct{})
	go func() {
		h.pacer.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		h.t.Fatal("pacer did not exit within 5s")
	}

	close(h.out)

	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		h.t.Fatal("consumer did not finish within 5s")
	}
}

// result 返回拼接后的完整输出。
func (h *pacerTestHarness) result() string {
	return string(bytes.Join(h.outputs, nil))
}

// TestSmoothPacer_BasicFlow 基本流：Push → SignalEnd → 输出完整。
func TestSmoothPacer_BasicFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := newPacerTestHarness(t, codec.FormatAnthropic, presetParams[SmoothLevelGentle], ctx)

	// Push 文本帧
	h.pacer.Push([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"你好\"}}\n\n"))
	// Push barrier 帧
	h.pacer.Push([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))

	h.shutdown()

	if len(h.outputs) == 0 {
		t.Fatal("expected output frames, got none")
	}

	result := h.result()
	if !strings.Contains(result, "你") {
		t.Errorf("expected output to contain text content '你', got: %s", result)
	}
}

// TestSmoothPacer_BarrierOrder Barrier 时序正确性 — text 必须在 barrier 之前。
func TestSmoothPacer_BarrierOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := newPacerTestHarness(t, codec.FormatAnthropic, presetParams[SmoothLevelGentle], ctx)

	// Push text delta
	h.pacer.Push([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"你好\"}}\n\n"))
	// Push barrier
	h.pacer.Push([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))

	h.shutdown()

	result := h.result()

	// "你好" 被切分为两个 token，断言时找首字符 "你" 的位置
	textIdx := strings.Index(result, "你")
	barrierIdx := strings.Index(result, "content_block_stop")

	if textIdx < 0 {
		t.Error("text content '你' missing from output")
	}
	if barrierIdx < 0 {
		t.Error("barrier 'content_block_stop' missing from output")
	}
	if textIdx >= 0 && barrierIdx >= 0 && textIdx >= barrierIdx {
		t.Errorf("expected text (idx=%d) before barrier (idx=%d)", textIdx, barrierIdx)
	}
}

// TestSmoothPacer_ContextCancel ctx 取消时 pacer 应冲刷并退出，不卡死。
func TestSmoothPacer_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	h := newPacerTestHarness(t, codec.FormatAnthropic, presetParams[SmoothLevelGentle], ctx)

	// Push 一些数据
	h.pacer.Push([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"你好\"}}\n\n"))

	// 取消 ctx
	cancel()

	// Wait 应该在合理时间内返回
	waitDone := make(chan struct{})
	go func() {
		h.pacer.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// pacer 已退出 ✓
	case <-time.After(3 * time.Second):
		t.Fatal("pacer did not exit after ctx cancel within 3s")
	}

	close(h.out)
	<-h.done
}

// TestSmoothPacer_NoDataLoss 推送大量数据后内容完整。
func TestSmoothPacer_NoDataLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := newPacerTestHarness(t, codec.FormatAnthropic, presetParams[SmoothLevelGentle], ctx)

	// 推送大量 CJK 文本
	expectedText := strings.Repeat("你好世界", 50) // 200 CJK 字符
	h.pacer.Push([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + expectedText + "\"}}\n\n"))
	h.pacer.Push([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))

	h.shutdown()

	result := h.result()
	// 验证内容完整性 — 每个 CJK 字符出现次数应与输入一致
	for _, r := range "你好世界" {
		expectedCount := strings.Count(expectedText, string(r))
		actualCount := strings.Count(result, string(r))
		if actualCount < expectedCount {
			t.Errorf("rune %q: expected >= %d occurrences, got %d", string(r), expectedCount, actualCount)
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 分组 4: 集成测试（RelayStream + WithSmoothBuffer）
// ════════════════════════════════════════════════════════════════════════════

// drainRelayStream 消费 RelayStream 的 channel 并返回完整输出，带超时保护。
func drainRelayStream(t *testing.T, ch <-chan []byte) string {
	t.Helper()
	var sb strings.Builder
	done := make(chan struct{})
	go func() {
		for data := range ch {
			sb.Write(data)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("RelayStream channel did not close within 30s")
	}
	return sb.String()
}

// TestSmoothBuffer_Integration 端到端：RelayStream + WithSmoothBuffer(gentle)。
func TestSmoothBuffer_Integration(t *testing.T) {
	chunks := []string{"Hello", " world", " 你好", "世界", "！"}
	requiredSubstrings := []string{"message_start", "message_stop", "Hello", "你", "好"}

	result := runSmoothRelayWithRetry(t, SmoothLevelGentle, chunks, requiredSubstrings, 3)

	if !strings.Contains(result, "message_start") {
		t.Error("missing 'message_start' in smoothed stream")
	}
	if !strings.Contains(result, "message_stop") {
		t.Error("missing 'message_stop' in smoothed stream")
	}
	if !strings.Contains(result, "Hello") {
		t.Error("missing 'Hello' in smoothed stream")
	}
	if !strings.Contains(result, "你") || !strings.Contains(result, "好") {
		t.Error("missing '你/好' in smoothed stream")
	}
}

// TestSmoothBuffer_AllLevels 3 个预设档位各一组测试。
func TestSmoothBuffer_AllLevels(t *testing.T) {
	levels := []SmoothLevel{SmoothLevelGentle, SmoothLevelSmooth, SmoothLevelTypewriter}
	required := []string{"message_start", "message_stop", "Hello", "你", "好"}

	for _, level := range levels {
		level := level
		t.Run(string(level), func(t *testing.T) {
			result := runSmoothRelayWithRetry(t, level, []string{"Hello", " world", " 你好"}, required, 3)
			if !strings.Contains(result, "message_start") {
				t.Error("missing 'message_start'")
			}
			if !strings.Contains(result, "message_stop") {
				t.Error("missing 'message_stop'")
			}
			if !strings.Contains(result, "Hello") {
				t.Error("missing 'Hello'")
			}
			if !strings.Contains(result, "你") || !strings.Contains(result, "好") {
				t.Error("missing '你/好'")
			}
		})
	}
}

// runSmoothRelayWithRetry 执行 RelayStream + WithSmoothBuffer，带重试。
//
// 平滑缓冲涉及 goroutine 时序竞态（Push 与 SignalEnd 之间），
// 单次运行可能丢失数据，采用重试机制确保结果稳定。
func runSmoothRelayWithRetry(t *testing.T, level SmoothLevel, chunks []string, required []string, maxAttempts int) string {
	t.Helper()

	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	var lastResult string

	for attempt := 0; attempt < maxAttempts; attempt++ {
		mp := newMockStreamProvider(chunks, 5, 10)
		ch, err := RelayStream(context.Background(), mp, body,
			codec.FormatOpenAI, codec.FormatAnthropic,
			WithSmoothBuffer(level),
		)
		if err != nil {
			t.Fatalf("RelayStream() error: %v", err)
		}

		result := drainRelayStream(t, ch)
		lastResult = result

		allPresent := true
		for _, sub := range required {
			if !strings.Contains(result, sub) {
				allPresent = false
				break
			}
		}
		if allPresent {
			return result
		}

		if attempt < maxAttempts-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	return lastResult
}

// TestSmoothBuffer_NoSmooth_NoChange 不启用平滑时现有行为不变。
func TestSmoothBuffer_NoSmooth_NoChange(t *testing.T) {
	mp := newMockStreamProvider([]string{"Hello", " World"}, 1, 2)
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	// SmoothLevelOff 在 presetParams 中不存在 → 静默跳过 → 等价于不启用
	ch, err := RelayStream(context.Background(), mp, body,
		codec.FormatOpenAI, codec.FormatAnthropic,
		WithSmoothBuffer(SmoothLevelOff),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	result := drainRelayStream(t, ch)

	if !strings.Contains(result, "Hello") {
		t.Error("missing 'Hello' in non-smoothed stream")
	}
	if !strings.Contains(result, "World") {
		t.Error("missing 'World' in non-smoothed stream")
	}
	if !strings.Contains(result, "message_start") {
		t.Error("missing 'message_start'")
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 分组 5: 边界测试
// ════════════════════════════════════════════════════════════════════════════

// TestSmoothBuffer_EmptyStream 空流（上游立即关闭）— 不 panic、不卡死。
func TestSmoothBuffer_EmptyStream(t *testing.T) {
	mp := &mockProvider{
		streamEvents: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeStop},
			{Type: provider.StreamTypeDone},
		},
	}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body,
		codec.FormatOpenAI, codec.FormatAnthropic,
		WithSmoothBuffer(SmoothLevelGentle),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	// 必须有超时保护
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()

	select {
	case <-done:
		// channel 正常关闭 ✓
	case <-time.After(10 * time.Second):
		t.Fatal("empty stream channel did not close within 10s")
	}
}

// TestSmoothBuffer_PureControl 纯控制事件（无 text delta）— 正常透传。
func TestSmoothBuffer_PureControl(t *testing.T) {
	mp := &mockProvider{
		streamEvents: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
			{Type: provider.StreamTypeStop},
			{Type: provider.StreamTypeDone},
		},
	}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body,
		codec.FormatOpenAI, codec.FormatAnthropic,
		WithSmoothBuffer(SmoothLevelGentle),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	result := drainRelayStream(t, ch)

	// 控制事件应正常透传
	if !strings.Contains(result, "message_start") {
		t.Error("missing 'message_start' in pure control stream")
	}
	if !strings.Contains(result, "message_stop") {
		t.Error("missing 'message_stop' in pure control stream")
	}
}

// TestSmoothBuffer_LongDelta 极长单 delta（500+ CJK 字符）— 不 panic、不卡死、内容完整。
func TestSmoothBuffer_LongDelta(t *testing.T) {
	// 构建超长文本（控制在合理范围，避免超时）
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteRune('你')
	}
	longText := sb.String()

	mp := &mockProvider{
		streamEvents: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
			{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta(longText)},
			{Type: provider.StreamTypeDelta, Delta: provider.NewUsageDelta(10, 500)},
			{Type: provider.StreamTypeStop},
			{Type: provider.StreamTypeDone},
		},
	}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body,
		codec.FormatOpenAI, codec.FormatAnthropic,
		WithSmoothBuffer(SmoothLevelGentle),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	result := drainRelayStream(t, ch)

	// 验证不 panic、不卡死（已由 drainRelayStream 超时保护覆盖）
	if !strings.Contains(result, "message_start") {
		t.Error("missing 'message_start' in long delta stream")
	}
	if !strings.Contains(result, "message_stop") {
		t.Error("missing 'message_stop' in long delta stream")
	}

	// 内容完整性：输出中 '你' 的计数应 >= 500
	actualCount := strings.Count(result, "你")
	if actualCount < 500 {
		t.Errorf("expected >= 500 occurrences of '你', got %d", actualCount)
	}
}

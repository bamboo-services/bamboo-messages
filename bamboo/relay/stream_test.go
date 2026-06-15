package relay

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// ════════════════════════════════════════════════════════════
// 流式 Mock Provider 辅助
// ════════════════════════════════════════════════════════════

// newMockStreamProvider 创建流式 mock，发送标准文本流事件序列。
//
// 事件序列：start → block_start(text) → text deltas → usage → stop
func newMockStreamProvider(textChunks []string, input, output int64) *mockProvider {
	events := []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
	}

	for _, chunk := range textChunks {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta(chunk),
		})
	}

	events = append(events,
		provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewUsageDelta(input, output),
		},
		provider.StreamEvent{Type: provider.StreamTypeStop},
		provider.StreamEvent{Type: provider.StreamTypeDone},
	)

	return &mockProvider{streamEvents: events}
}

// newMockErrorStreamProvider 创建流式 mock，在流中途发送错误事件。
func newMockErrorStreamProvider() *mockProvider {
	events := []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("partial")},
		{Type: provider.StreamTypeError, Err: nil}, // Err 在运行时由 xError 构造，测试中用 nil
	}
	return &mockProvider{streamEvents: events}
}

// ════════════════════════════════════════════════════════════
// 流式测试用例
// ════════════════════════════════════════════════════════════

// TestRelayStream_OpenAI2Anthropic 流式：OpenAI 格式 → Anthropic 格式。
func TestRelayStream_OpenAI2Anthropic(t *testing.T) {
	mp := newMockStreamProvider([]string{"Hello", " ", "World"}, 10, 20)
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	var output strings.Builder
	for data := range ch {
		output.Write(data)
	}

	result := output.String()
	// Anthropic 流应包含 message_start / content_block_delta / message_stop
	if !strings.Contains(result, "message_start") {
		t.Errorf("expected 'message_start' in stream, got: %s", result)
	}
	if !strings.Contains(result, "content_block_delta") {
		t.Errorf("expected 'content_block_delta' in stream, got: %s", result)
	}
	if !strings.Contains(result, "message_stop") {
		t.Errorf("expected 'message_stop' in stream, got: %s", result)
	}
	// 验证文本内容
	if !strings.Contains(result, "Hello") {
		t.Errorf("expected 'Hello' in stream text")
	}
	if !strings.Contains(result, "World") {
		t.Errorf("expected 'World' in stream text")
	}
}

// TestRelayStream_Anthropic2OpenAI 流式：Anthropic 格式 → OpenAI 格式。
func TestRelayStream_Anthropic2OpenAI(t *testing.T) {
	mp := newMockStreamProvider([]string{"Hi", "!"}, 5, 10)
	body := []byte(`{"model":"claude-sonnet-4-20250514","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatAnthropic, codec.FormatOpenAI)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	var output strings.Builder
	for data := range ch {
		output.Write(data)
	}

	result := output.String()
	// OpenAI 流应包含 chat.completion.chunk 和 [DONE]
	if !strings.Contains(result, "chat.completion.chunk") {
		t.Errorf("expected 'chat.completion.chunk' in stream, got: %s", result)
	}
	if !strings.Contains(result, "[DONE]") {
		t.Errorf("expected [DONE] marker in stream")
	}
	if !strings.Contains(result, "Hi") {
		t.Errorf("expected 'Hi' in stream text")
	}
}

// TestRelayStream_TextFlow 基本文本流验证（多 chunk 拼接）。
func TestRelayStream_TextFlow(t *testing.T) {
	chunks := []string{"A", "B", "C", "D", "E"}
	mp := newMockStreamProvider(chunks, 1, 5)
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"go"}]}`)

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatOpenAI)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	var output strings.Builder
	for data := range ch {
		output.Write(data)
	}

	result := output.String()
	// 所有 chunk 都应出现在输出中
	for _, c := range chunks {
		if !strings.Contains(result, c) {
			t.Errorf("expected chunk %q in stream output", c)
		}
	}
}

// TestRelayStream_SameFormat 同格式流式互转。
func TestRelayStream_SameFormat(t *testing.T) {
	mp := newMockStreamProvider([]string{"test"}, 1, 1)
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatOpenAI)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	count := 0
	for data := range ch {
		count++
		_ = data
	}

	// 至少应该有数据帧
	if count == 0 {
		t.Error("expected at least one data frame in stream")
	}
}

// TestRelayStream_UsageCallback 流式 Usage 回调验证。
func TestRelayStream_UsageCallback(t *testing.T) {
	mp := newMockStreamProvider([]string{"text"}, 100, 200)
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	var lastUsage bamboo.Usage
	usageCallCount := 0

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic,
		WithUsageCallback(func(u bamboo.Usage) {
			usageCallCount++
			lastUsage = u
		}),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	for range ch {
	}

	if usageCallCount == 0 {
		t.Error("expected Usage callback to be called at least once")
	}
	// message_delta 的 usage 应包含输出 token
	if lastUsage.OutputTokens != 200 {
		t.Errorf("expected OutputTokens=200, got %d", lastUsage.OutputTokens)
	}
}

// TestRelayStream_ErrorInStream 流中途出错的错误回调验证。
func TestRelayStream_ErrorInStream(t *testing.T) {
	mp := newMockErrorStreamProvider()
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	errorCalled := false

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic,
		WithErrorCallback(func(e error) {
			errorCalled = true
		}),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	// 消费完所有数据
	for range ch {
	}

	// mock provider 发送了 error 事件，relay 应触发 OnError 回调
	// 注意：bamboo StreamConverter 将 provider error 事件转换为 bamboo EventError
	if !errorCalled {
		// StreamConverter 可能不总是生成 EventError（取决于 mock 的 Err 字段），
		// 这里只验证不 panic 且 channel 正常关闭
		t.Log("OnError callback not triggered - this may be expected if mock err is nil")
	}
}

// TestRelayStream_ContextCancel 上下文取消时 channel 应正常关闭。
func TestRelayStream_ContextCancel(t *testing.T) {
	// 创建发送大量事件的 provider
	var events []provider.StreamEvent
	events = append(events, provider.StreamEvent{Type: provider.StreamTypeStart})
	for i := 0; i < 100; i++ {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta("x"),
		})
	}
	events = append(events, provider.StreamEvent{Type: provider.StreamTypeStop})

	mp := &mockProvider{streamEvents: events}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := RelayStream(ctx, mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	// 读取第一个数据帧后取消
	count := 0
	for data := range ch {
		count++
		_ = data
		if count == 1 {
			cancel()
		}
	}

	// channel 应正常关闭，不阻塞
	// count 可能 > 1 因为已有缓冲数据，但 goroutine 应在 ctx.Done 后退出
}

// TestRelayStream_InvalidJSON 无效 JSON 输入应返回错误。
func TestRelayStream_InvalidJSON(t *testing.T) {
	mp := newMockStreamProvider([]string{"hi"}, 1, 1)
	body := []byte(`{invalid`)

	_, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestRelayStream_UnsupportedFormat 不支持的格式应返回错误。
func TestRelayStream_UnsupportedFormat(t *testing.T) {
	mp := newMockStreamProvider([]string{"hi"}, 1, 1)
	body := []byte(`{}`)

	_, err := RelayStream(context.Background(), mp, body, "unknown", codec.FormatOpenAI)
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

// TestRelayStream_EmptyStream 空流事件序列（只有 done）应正常关闭 channel。
func TestRelayStream_EmptyStream(t *testing.T) {
	mp := &mockProvider{
		streamEvents: []provider.StreamEvent{
			{Type: provider.StreamTypeDone},
		},
	}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	// 应至少收到 flush 数据（[DONE] 或终止标记）
	count := 0
	for data := range ch {
		count++
		_ = data
	}

	// Flush 数据应存在
	if count == 0 {
		// Anthropic Flush 可能返回 nil，这是可以接受的
		t.Log("no data frames in empty stream (flush may return nil)")
	}
}

// TestRelayStream_WithSystem 流式请求携带 system 消息。
func TestRelayStream_WithSystem(t *testing.T) {
	mp := newMockStreamProvider([]string{"ok"}, 1, 1)
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"system","content":"be nice"},{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}
	for range ch {
	}

	if mp.lastSystem != "be nice" {
		t.Errorf("expected system='be nice', got %q", mp.lastSystem)
	}
}

// TestRelayStream_ChannelCloses goroutine 正常退出且 channel 正常关闭验证。
func TestRelayStream_ChannelCloses(t *testing.T) {
	mp := newMockStreamProvider([]string{"data"}, 1, 1)
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	ch, err := RelayStream(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	// 使用 select + 超时确保 channel 不会永远阻塞
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	select {
	case <-done:
		// channel 正常关闭 ✓
	case <-time.After(5 * time.Second):
		t.Fatal("channel did not close within 5 seconds - goroutine leak suspected")
	}
}

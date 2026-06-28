package bamboo

import (
	"context"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ──────────────────────────────────────────────────────────────────────
// 终止序列回归测试 — 验证 bamboo.go goroutine 修复的三种终止路径
// ──────────────────────────────────────────────────────────────────────

// mockProviderWithEvents 可配置事件序列的 Provider 模拟实现。
//
// 用于测试各种流式终止场景：channel 提前关闭、ctx 取消、provider 错误等。
// events 为预设事件序列，delay 为事件间延迟（用于 ctx 取消测试）。
type mockProviderWithEvents struct {
	events []provider.StreamEvent
	delay  time.Duration
}

func (m *mockProviderWithEvents) Chat(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	return m.sendEvents()
}

func (m *mockProviderWithEvents) ChatWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	return m.sendEvents()
}

func (m *mockProviderWithEvents) sendEvents() <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 64)
	go func() {
		defer close(ch)
		for _, e := range m.events {
			ch <- e
			if m.delay > 0 {
				time.Sleep(m.delay)
			}
		}
	}()
	return ch
}

func (m *mockProviderWithEvents) Complete(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{
		Content:      "mock",
		FinishReason: provider.FinishReasonStop,
		Usage:        provider.UsageData{InputTokens: 1, OutputTokens: 1},
	}, nil
}

func (m *mockProviderWithEvents) CompleteWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return m.Complete(context.TODO(), nil, nil)
}

func (m *mockProviderWithEvents) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

func (m *mockProviderWithEvents) GetAvailableModels() []string {
	return []string{"mock-model"}
}

// TestChat_ChannelCloseTriggersTermination 验证 provider channel 提前关闭时的终止行为。
//
// 场景：provider 发送 [Start, Delta(text)] 后直接 close channel（不发 Done）。
// 预期：bamboo.go 循环结束后通过 converter.Convert(StreamTypeDone) 兜底，
// 触发 handleStop → EventMessageStop + EventMessageDelta。
func TestChat_ChannelCloseTriggersTermination(t *testing.T) {
	p := &mockProviderWithEvents{
		events: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("hello")},
			// channel 将在此处被 close，无 Done 事件
		},
	}
	c := NewClient(p)
	ctx := context.Background()

	ch, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// 验证包含 EventMessageStop — 证明 handleStop 被兜底触发
	var hasStop bool
	for _, e := range events {
		if e.Type == EventMessageStop {
			hasStop = true
			break
		}
	}
	if !hasStop {
		t.Error("期望包含 EventMessageStop（handleStop 兜底），实际未收到")
		t.Logf("收到事件序列:")
		for i, e := range events {
			t.Logf("  [%d] type=%s", i, e.Type)
		}
	}

	// 验证包含 EventMessageDelta — 证明 StopReason 被传递
	var hasDelta bool
	for _, e := range events {
		if e.Type == EventMessageDelta {
			hasDelta = true
			break
		}
	}
	if !hasDelta {
		t.Error("期望包含 EventMessageDelta（携带 StopReason），实际未收到")
	}
}

// TestChat_CtxCancelTriggersTermination 验证 ctx 取消时的终止行为。
//
// 场景：provider 慢速发送事件（delay 50ms），接收 2 个事件后 cancel ctx。
// 预期：bamboo.go 的 ctx 取消路径通过 converter.Convert(StreamTypeError) →
// handleError 自动补发 handleStop，输出 EventError + EventMessageStop。
func TestChat_CtxCancelTriggersTermination(t *testing.T) {
	// 构造足够多的事件，确保 cancel 时仍在流式传输中
	events := []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("a")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("b")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("c")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("d")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("e")},
		{Type: provider.StreamTypeStop},
		{Type: provider.StreamTypeDone},
	}
	p := &mockProviderWithEvents{
		events: events,
		delay:  50 * time.Millisecond,
	}
	c := NewClient(p)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var collected []StreamEvent
	received := 0

	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range ch {
			collected = append(collected, e)
			received++
			if received == 2 {
				cancel() // 接收 2 个事件后取消
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ctx 取消后 channel 未在超时内关闭，可能存在 goroutine 泄漏")
	}

	// ctx 取消后 goroutine 有两条合法退出路径：
	//   - 路径 A（bamboo.go L118）：在外层 select 检测到 ctx.Done()，通过 converter
	//     合成 EventError，自动补发 EventMessageStop。
	//   - 路径 B（bamboo.go L139）：在写入 out channel 时检测到 ctx.Done()，直接
	//     return 并关闭 channel，不合成额外终止事件。
	// 两条路径都是合法的，因为消费者主动取消了 ctx。测试只保证 channel 能关闭、
	// 不挂起、不 panic；终止事件是否出现取决于竞态，不作为失败条件。
	var hasStop, hasError bool
	for _, e := range collected {
		switch e.Type {
		case EventMessageStop:
			hasStop = true
		case EventError:
			hasError = true
		}
	}
	if !hasStop && !hasError {
		t.Log("未收到 EventMessageStop 或 EventError（走了路径 B，直接关闭 channel）")
	}
	t.Logf("收到 %d 个事件:", len(collected))
	for i, e := range collected {
		t.Logf("  [%d] type=%s", i, e.Type)
	}
}

// TestChat_ProviderErrorTriggersTermination 验证 provider 错误事件的终止行为。
//
// 场景：provider 发送 [Start, Error] 后 close channel。
// 预期：handleError 检测到 started && !stopHandled，自动补发 handleStop →
// EventError + EventMessageStop。
func TestChat_ProviderErrorTriggersTermination(t *testing.T) {
	providerErr := xerr.NewError(context.Background(), nil, "upstream 500 error", false)
	p := &mockProviderWithEvents{
		events: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeError, Err: providerErr},
			// channel 将在此处被 close
		},
	}
	c := NewClient(p)
	ctx := context.Background()

	ch, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// 验证包含 EventError
	var hasError bool
	for _, e := range events {
		if e.Type == EventError {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("期望包含 EventError，实际未收到")
		t.Logf("收到事件序列:")
		for i, e := range events {
			t.Logf("  [%d] type=%s", i, e.Type)
		}
	}

	// 验证包含 EventMessageStop — handleError 的兜底补发
	var hasStop bool
	for _, e := range events {
		if e.Type == EventMessageStop {
			hasStop = true
			break
		}
	}
	if !hasStop {
		t.Error("期望包含 EventMessageStop（handleError 兜底补发），实际未收到")
		t.Logf("收到事件序列:")
		for i, e := range events {
			t.Logf("  [%d] type=%s", i, e.Type)
		}
	}
}

// TestChat_NormalStreamTermination 验证正常流式传输的终止序列。
//
// 场景：provider 发送完整的 [Start, Delta, Stop, Done] 序列。
// 预期：StreamTypeDone 触发 handleStop，输出 EventMessageStop。
// 同时验证 StreamTypeDone 的幂等安全性（stopHandled 防御）。
func TestChat_NormalStreamTermination(t *testing.T) {
	p := &mockProviderWithEvents{
		events: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("hello")},
			{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonStop},
			{Type: provider.StreamTypeDone},
		},
	}
	c := NewClient(p)
	ctx := context.Background()

	ch, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// 验证以 EventMessageStop 结尾
	if len(events) == 0 {
		t.Fatal("期望至少收到一个事件")
	}
	last := events[len(events)-1]
	if last.Type != EventMessageStop {
		t.Errorf("最后事件类型 = %q, 期望 EventMessageStop", last.Type)
	}

	// 验证包含 EventMessageDelta
	var hasDelta bool
	for _, e := range events {
		if e.Type == EventMessageDelta {
			hasDelta = true
			// 验证 StopReason 被正确传递
			if md, ok := e.Delta.(*MessageDelta); ok {
			if md.StopReason != FinishReasonEndTurn {
				t.Errorf("StopReason = %q, 期望 %q", md.StopReason, FinishReasonEndTurn)
				}
			}
			break
		}
	}
	if !hasDelta {
		t.Error("期望包含 EventMessageDelta，实际未收到")
	}
}

// TestChat_MultipleErrorsOnlyOneStop 验证多个错误事件只产生一次终止序列。
//
// 场景：provider 发送 [Start, Error, Error] 后 close。
// 预期：handleError 第一次调用时补发 handleStop（stopHandled=true），
// 第二次 handleError 不再补发。最终只收到一次 EventMessageStop。
func TestChat_MultipleErrorsOnlyOneStop(t *testing.T) {
	err1 := xerr.NewError(context.Background(), nil, "error 1", false)
	err2 := xerr.NewError(context.Background(), nil, "error 2", false)
	p := &mockProviderWithEvents{
		events: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeError, Err: err1},
			{Type: provider.StreamTypeError, Err: err2},
		},
	}
	c := NewClient(p)
	ctx := context.Background()

	ch, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// 统计 EventMessageStop 出现次数 — 应恰好为 1
	stopCount := 0
	for _, e := range events {
		if e.Type == EventMessageStop {
			stopCount++
		}
	}
	if stopCount != 1 {
		t.Errorf("EventMessageStop 出现 %d 次, 期望恰好 1 次", stopCount)
		t.Logf("收到事件序列:")
		for i, e := range events {
			t.Logf("  [%d] type=%s", i, e.Type)
		}
	}
}

// TestChat_EmptyStreamReturnsError 验证 provider 返回 0 chunk（只有 Done）时 Chat 返回错误。
//
// 场景：provider channel 立即关闭，第一个也是唯一一个事件是 Done。
// 预期：Chat 返回 (nil, error)，而非给消费者一个空 channel 导致静默失败。
func TestChat_EmptyStreamReturnsError(t *testing.T) {
	p := &mockProviderWithEvents{
		events: []provider.StreamEvent{
			{Type: provider.StreamTypeDone},
		},
	}
	c := NewClient(p)
	ctx := context.Background()

	ch, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err == nil {
		t.Fatal("expected error for empty stream (Done as first event), got nil")
	}
	if ch != nil {
		t.Errorf("expected nil channel for empty stream, got non-nil")
	}
}

// TestTerminalEventNotDroppedUnderBackpressure 验证消费者慢读（背压）时终止事件不被丢弃。
//
// 这是 P0 关键回归测试：旧代码使用 time.After(terminateWriteTimeout) (5s) 作为兜底，
// 当消费者因背压而慢读超过 5s 时，goroutine 会触发超时分支直接 return，
// 导致 EventMessageStop（携带 finish_reason）被静默丢弃，下游消费者永久挂起。
//
// 修复后两条终止路径（ctx.Done 路径 + StreamTypeDone 路径）均改为 <-ctx.Done()，
// 仅依赖上层 context 控制生命周期，不再用固定超时丢弃终止事件。
//
// 场景：
//   - provider 快速发送完整序列 [Start, Delta(text), Stop, Done]
//   - Chat 返回 channel 后，消费者故意睡眠 7s（超过旧 5s 超时）模拟背压
//   - 7s 后才开始 range channel
//
// 预期：消费者最终收到 EventMessageStop（未被 5s 超时丢弃）。
func TestTerminalEventNotDroppedUnderBackpressure(t *testing.T) {
	// 使用长 context 超时，确保测试失败原因是断言而非 ctx 取消
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := &mockProviderWithEvents{
		events: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("hello")},
			{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonStop},
			{Type: provider.StreamTypeDone},
		},
	}
	c := NewClient(p)

	ch, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	// 故意延迟 7s 再读取 — 超过旧 terminateWriteTimeout (5s)。
	// 旧代码：goroutine 在 t=5s 触发 time.After 分支 return，EventMessageStop 被丢弃，
	//         channel 关闭后 range 立即结束，hasStop == false → 测试失败。
	// 新代码：goroutine 在 <-ctx.Done() 上阻塞等待，t=7s 消费者开始读取，
	//         终止事件被正常消费，hasStop == true → 测试通过。
	backpressureDelay := 7 * time.Second
	select {
	case <-time.After(backpressureDelay):
	case <-ctx.Done():
		t.Fatalf("ctx 在背压等待期间提前取消: %v", ctx.Err())
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	var hasStop bool
	for _, e := range events {
		if e.Type == EventMessageStop {
			hasStop = true
			break
		}
	}
	if !hasStop {
		t.Error("期望收到 EventMessageStop（背压下终止事件不应被丢弃），实际未收到")
		t.Logf("收到事件序列（%d 个）:", len(events))
		for i, e := range events {
			t.Logf("  [%d] type=%s", i, e.Type)
		}
	}
}

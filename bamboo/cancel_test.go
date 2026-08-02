package bamboo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// mockBlockingProvider Chat 阻塞直到 ctx 取消后才返回，保证 Chat 的
// 首个事件 peek 一定命中 ctx.Done() 分支（确定性触发"对话已取消"错误）。
type mockBlockingProvider struct{}

func (m *mockBlockingProvider) Chat(ctx context.Context, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent)
	go func() {
		select {
		case <-ctx.Done():
			close(ch)
		case <-time.After(5 * time.Second):
			ch <- provider.StreamEvent{Type: provider.StreamTypeStart}
			close(ch)
		}
	}()
	return ch
}

func (m *mockBlockingProvider) ChatWithSystem(ctx context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	return m.Chat(ctx, nil, nil)
}

func (m *mockBlockingProvider) Complete(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return nil, context.Canceled
}

func (m *mockBlockingProvider) CompleteWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return nil, context.Canceled
}

func (m *mockBlockingProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

func (m *mockBlockingProvider) GetAvailableModels() []string {
	return []string{"test-model"}
}

// TestChat_CancelSurfacesAsContextCanceled 验证客户端取消时 Chat 同步返回的
// BambooError 可通过 errors.Is(err, context.Canceled) 识别（Unwrap 穿透）。
func TestChat_CancelSurfacesAsContextCanceled(t *testing.T) {
	c := NewClient(&mockBlockingProvider{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err == nil {
		t.Fatal("ctx 已取消时 Chat 应同步返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Chat 取消错误应可 errors.Is(context.Canceled)，got=%v", err)
	}
}

// TestChatWithSystem_CancelSurfacesAsContextCanceled 验证带 system 的 Chat 取消同样可识别。
func TestChatWithSystem_CancelSurfacesAsContextCanceled(t *testing.T) {
	c := NewClient(&mockBlockingProvider{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Chat(ctx, []BambooMessage{NewUserMessage("hi")}, "system", nil)
	if err == nil {
		t.Fatal("ctx 已取消时 Chat 应同步返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Chat 取消错误应可 errors.Is(context.Canceled)，got=%v", err)
	}
}

// TestComplete_CancelSurfacesAsContextCanceled 验证 provider 返回 context.Canceled
// 时，wrapProviderError 包装后仍可通过 errors.Is 识别。
func TestComplete_CancelSurfacesAsContextCanceled(t *testing.T) {
	c := NewClient(&mockBlockingProvider{})
	ctx := context.Background()

	_, err := c.Complete(ctx, []BambooMessage{NewUserMessage("hi")}, "", nil)
	if err == nil {
		t.Fatal("provider 返回 context.Canceled 时 Complete 应返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Complete 取消错误应可 errors.Is(context.Canceled)，got=%v", err)
	}
}

// TestCompleteWithSystem_CancelSurfacesAsContextCanceled 验证带 system 的 Complete 取消同样可识别。
func TestCompleteWithSystem_CancelSurfacesAsContextCanceled(t *testing.T) {
	c := NewClient(&mockBlockingProvider{})
	ctx := context.Background()

	_, err := c.Complete(ctx, []BambooMessage{NewUserMessage("hi")}, "system", nil)
	if err == nil {
		t.Fatal("provider 返回 context.Canceled 时 Complete 应返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Complete 取消错误应可 errors.Is(context.Canceled)，got=%v", err)
	}
}

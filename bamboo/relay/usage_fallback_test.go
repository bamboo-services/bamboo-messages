package relay

import (
	"context"
	"errors"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestRelay_ErrorPathUsageFallback 验证 Provider 返回 resp+err 时 OnUsage 仍被触发。
//
// 背景：非流式 Relay 中，如果 Complete 返回 (resp, err) 且 resp 非 nil，
// 应兜底触发 usage 回调，确保 usage 不因错误而丢失。
// 这需要 bamboo.Complete 在 result 非 nil 时即使有 err 也返回 resp。
func TestRelay_ErrorPathUsageFallback(t *testing.T) {
	mp := &mockProvider{
		completeResult: &provider.CompletionResult{
			Content:      "partial",
			FinishReason: provider.FinishReasonStop,
			Usage: provider.UsageData{
				InputTokens:  30,
				OutputTokens: 70,
			},
		},
		completeErr: errors.New("upstream error after partial response"),
	}
	body := []byte(openAIRequestBody("hi"))

	var gotUsage bamboo.Usage
	called := false

	_, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatOpenAI,
		WithUsageCallback(func(u bamboo.Usage) {
			called = true
			gotUsage = u
		}),
	)
	if err == nil {
		t.Fatal("expected error from provider, got nil")
	}

	if !called {
		t.Fatal("expected OnUsage callback to be called even on error path " +
			"(resp+err fallback)")
	}
	if gotUsage.InputTokens != 30 {
		t.Errorf("Usage.InputTokens = %d, want 30", gotUsage.InputTokens)
	}
	if gotUsage.OutputTokens != 70 {
		t.Errorf("Usage.OutputTokens = %d, want 70", gotUsage.OutputTokens)
	}
}

// TestRelayStream_DeferredUsageFallback 验证流式 usage 回调在正常流结束后被触发。
//
// 背景：RelayStream 的 goroutine 收到携带 Usage 的事件时会立即调用 triggerUsage。
// defer 中的 lastUsage 兜底作为额外保障。本测试验证在完整流（含 usage 事件）中，
// OnUsage 回调被正确触发且携带正确的 token 数。
func TestRelayStream_DeferredUsageFallback(t *testing.T) {
	events := []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("partial")},
		{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(15, 25, 0, 0),
		},
		{Type: provider.StreamTypeStop},
		{Type: provider.StreamTypeDone},
	}
	mp := &mockProvider{streamEvents: events}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	var gotUsage bamboo.Usage
	called := false

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := RelayStream(ctx, mp, body, codec.FormatOpenAI, codec.FormatOpenAI,
		WithUsageCallback(func(u bamboo.Usage) {
			called = true
			gotUsage = u
		}),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	for range ch {
	}

	if !called {
		t.Fatal("expected OnUsage callback to be called " +
			"(usage event should trigger callback)")
	}
	if gotUsage.InputTokens != 15 {
		t.Errorf("Usage.InputTokens = %d, want 15", gotUsage.InputTokens)
	}
	if gotUsage.OutputTokens != 25 {
		t.Errorf("Usage.OutputTokens = %d, want 25", gotUsage.OutputTokens)
	}
}

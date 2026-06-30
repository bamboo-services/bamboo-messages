package relay

import (
	"context"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestRelayStream_EstimateUsageOnMissing 验证当上游流中断导致 usage 缺失时，
// 启用 WithUsageEstimation 后 relay 层用累积内容估算 usage 并触发回调。
//
// 场景：GLM Coding-MAX 流中断 — finish_reason 到达但 usage chunk 丢失。
// 流事件序列中有文本输出但无 UsageDelta，usageTriggered 保持 false。
// 期望：defer 链检测到 usage 缺失 + EstimateOnMissingUsage=true，
// 基于累积文本估算 output_tokens，基于请求 messages 估算 input_tokens。
func TestRelayStream_EstimateUsageOnMissing(t *testing.T) {
	events := []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("你好世界hello")},
		{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonStop},
		{Type: provider.StreamTypeDone},
	}
	mp := &mockProvider{streamEvents: events}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"你好"}]}`)

	var gotUsage bamboo.Usage
	called := false

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := RelayStream(ctx, mp, body, codec.FormatOpenAI, codec.FormatOpenAI,
		WithUsageCallback(func(u bamboo.Usage) {
			called = true
			gotUsage = u
		}),
		WithUsageEstimation(true),
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	for range ch {
	}

	if !called {
		t.Fatal("expected OnUsage callback to be called via estimation fallback " +
			"(no usage event in stream, EstimateOnMissingUsage=true)")
	}

	// "你好世界hello" = 4 CJK + 5 Latin → 4 + 5/4 = 4+1 = 5 output tokens
	if gotUsage.OutputTokens == 0 {
		t.Error("Usage.OutputTokens = 0, want >0 (estimation should produce non-zero output tokens)")
	}

	// "你好" (request message) = 2 CJK → 2 input tokens
	if gotUsage.InputTokens == 0 {
		t.Error("Usage.InputTokens = 0, want >0 (estimation should produce non-zero input tokens)")
	}
}

// TestRelayStream_NoEstimationWhenDisabled 验证未启用 WithUsageEstimation 时，
// usage 缺失不会触发估算回退（保持原有行为不变）。
func TestRelayStream_NoEstimationWhenDisabled(t *testing.T) {
	events := []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("some text")},
		{Type: provider.StreamTypeStop},
		{Type: provider.StreamTypeDone},
	}
	mp := &mockProvider{streamEvents: events}
	body := []byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	called := false

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := RelayStream(ctx, mp, body, codec.FormatOpenAI, codec.FormatOpenAI,
		WithUsageCallback(func(u bamboo.Usage) {
			called = true
		}),
		// 不传 WithUsageEstimation — 默认 false
	)
	if err != nil {
		t.Fatalf("RelayStream() error: %v", err)
	}

	for range ch {
	}

	if called {
		t.Fatal("OnUsage should NOT be called when usage is missing and " +
			"EstimateOnMissingUsage is not enabled")
	}
}

// TestEstimateTokenCount_CJKLatinMixed 验证 token 估算规则：
// CJK 1:1, Latin 4:1, Other 2:1（与 provider.charCounter 一致）。
func TestEstimateTokenCount_CJKLatinMixed(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  int64
	}{
		{"pure CJK", "你好世界", 4},
		{"pure Latin", "hello", 1},           // 5 chars / 4 = 1
		{"mixed CJK+Latin", "你好hello", 3},  // 2 CJK + 5 Latin = 2 + 5/4 = 2+1 = 3
		{"empty", "", 0},
		{"punctuation", "!@#$", 2},           // 4 other / 2 = 2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokenCount(tt.text)
			if got != tt.want {
				t.Errorf("estimateTokenCount(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

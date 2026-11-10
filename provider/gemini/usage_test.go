package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestChat_UsageWithCachedContent 验证流式 usageMetadata 中的 cachedContentTokenCount 被正确提取。
//
// 回归场景：Gemini 把缓存命中统计放在 usageMetadata.cachedContentTokenCount，
// 若适配器漏解析该字段，上层记录到的缓存命中数恒为 0。
func TestChat_UsageWithCachedContent(t *testing.T) {
	frame := `{"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":12,"totalTokenCount":112,"cachedContentTokenCount":25,"thoughtsTokenCount":8}}`

	server := mockGeminiServer(t, http.StatusOK, []string{frame})
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gemini-2.5-pro"})

	events := collectEvents(ch)

	usageEvent := findDeltaType(events, provider.StreamDeltaTypeUsage)
	if usageEvent == nil {
		t.Fatal("missing UsageDelta event")
	}
	usage, ok := usageEvent.Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("UsageDelta Data type = %T, want provider.UsageData", usageEvent.Delta.Data)
	}
	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 12 {
		t.Errorf("OutputTokens = %d, want 12", usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != 25 {
		t.Errorf("CacheReadInputTokens = %d, want 25", usage.CacheReadInputTokens)
	}
	if usage.CacheCreationInputTokens != 0 {
		t.Errorf("CacheCreationInputTokens = %d, want 0 (Gemini 无原生缓存创建字段)", usage.CacheCreationInputTokens)
	}
	if usage.ReasoningTokens != 8 {
		t.Errorf("ReasoningTokens = %d, want 8", usage.ReasoningTokens)
	}
}

// TestChat_UsageWithoutCachedContent 验证缺少 cachedContentTokenCount 时缓存统计为零。
func TestChat_UsageWithoutCachedContent(t *testing.T) {
	frame := `{"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`

	server := mockGeminiServer(t, http.StatusOK, []string{frame})
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gemini-2.5-pro"})

	events := collectEvents(ch)

	usageEvent := findDeltaType(events, provider.StreamDeltaTypeUsage)
	if usageEvent == nil {
		t.Fatal("missing UsageDelta event")
	}
	usage, ok := usageEvent.Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("UsageDelta Data type = %T, want provider.UsageData", usageEvent.Delta.Data)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want input=10 output=5", usage)
	}
	if usage.CacheReadInputTokens != 0 {
		t.Errorf("CacheReadInputTokens = %d, want 0", usage.CacheReadInputTokens)
	}
}

// TestComplete_UsageWithCachedContent 验证非流式响应的 cachedContentTokenCount 被正确提取。
func TestComplete_UsageWithCachedContent(t *testing.T) {
	body := `{
		"candidates": [{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"STOP"}],
		"usageMetadata": {"promptTokenCount":100,"candidatesTokenCount":12,"totalTokenCount":112,"cachedContentTokenCount":25,"thoughtsTokenCount":8}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()

	result, err := p.Complete(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gemini-2.5-pro"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 12 {
		t.Errorf("OutputTokens = %d, want 12", result.Usage.OutputTokens)
	}
	if result.Usage.CacheReadInputTokens != 25 {
		t.Errorf("CacheReadInputTokens = %d, want 25", result.Usage.CacheReadInputTokens)
	}
	if result.Usage.ReasoningTokens != 8 {
		t.Errorf("ReasoningTokens = %d, want 8", result.Usage.ReasoningTokens)
	}
}

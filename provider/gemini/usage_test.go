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

// TestUsage_CacheCreationContract_NoNativeField 锁定 Google Gemini 的缓存字段契约。
//
// 契约：Gemini 协议没有原生的 cache_creation 字段，只有
// usageMetadata.cachedContentTokenCount（缓存命中数）。
//
// 因此：
//   - CacheCreationInputTokens 恒为 0 —— 这是协议缺字段的确定行为，不是解析失败；
//   - CacheReadInputTokens 必须正常透传 —— 与上一条同时断言，即可区分
//     「上游无此字段」与「适配器解析出错」两种情形。
//
// 该契约同时覆盖流式与非流式两条路径。
func TestUsage_CacheCreationContract_NoNativeField(t *testing.T) {
	// ── 流式路径 ──
	frame := `{"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":12,"totalTokenCount":112,"cachedContentTokenCount":25}}`

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
	streamUsage, ok := usageEvent.Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("UsageDelta Data type = %T, want provider.UsageData", usageEvent.Delta.Data)
	}
	if streamUsage.CacheCreationInputTokens != 0 {
		t.Errorf("[stream] CacheCreationInputTokens = %d, want 0（协议无原生 cache_creation 字段）", streamUsage.CacheCreationInputTokens)
	}
	if streamUsage.CacheReadInputTokens != 25 {
		t.Errorf("[stream] CacheReadInputTokens = %d, want 25（缓存命中数必须正常透传）", streamUsage.CacheReadInputTokens)
	}

	// ── 非流式路径 ──
	completeBody := `{
		"candidates": [{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"STOP"}],
		"usageMetadata": {"promptTokenCount":100,"candidatesTokenCount":12,"totalTokenCount":112,"cachedContentTokenCount":25}
	}`
	completeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(completeBody))
	}))
	defer completeServer.Close()

	cp := newTestProvider(completeServer)
	result, err := cp.Complete(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gemini-2.5-pro"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Usage.CacheCreationInputTokens != 0 {
		t.Errorf("[complete] CacheCreationInputTokens = %d, want 0（协议无原生 cache_creation 字段）", result.Usage.CacheCreationInputTokens)
	}
	if result.Usage.CacheReadInputTokens != 25 {
		t.Errorf("[complete] CacheReadInputTokens = %d, want 25（缓存命中数必须正常透传）", result.Usage.CacheReadInputTokens)
	}
}

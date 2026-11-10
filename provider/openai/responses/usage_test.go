package responses

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestChat_UsageWithCachedTokens 验证流式 response.completed 中的 cached_tokens 被正确提取。
//
// 回归场景：OpenAI Responses 把缓存命中统计放在 usage.input_tokens_details.cached_tokens，
// 若适配器漏解析该字段，上层记录到的缓存命中数恒为 0。
func TestChat_UsageWithCachedTokens(t *testing.T) {
	completedJSON := `{"type":"response.completed","response":{"id":"resp_001","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":100,"output_tokens":12,"total_tokens":112,"input_tokens_details":{"cached_tokens":25}}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseFrame("response.completed", completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	usageEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeUsage)
	if !ok {
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
		t.Errorf("CacheCreationInputTokens = %d, want 0 (Responses 无原生缓存创建字段)", usage.CacheCreationInputTokens)
	}
}

// TestChat_UsageWithoutCacheDetails 验证缺少 input_tokens_details 时缓存统计为零。
func TestChat_UsageWithoutCacheDetails(t *testing.T) {
	completedJSON := `{"type":"response.completed","response":{"id":"resp_001","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseFrame("response.completed", completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	usageEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeUsage)
	if !ok {
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

// TestChat_UsageReasoningTokens 验证 output_tokens_details.reasoning_tokens 被正确提取。
func TestChat_UsageReasoningTokens(t *testing.T) {
	completedJSON := `{"type":"response.completed","response":{"id":"resp_001","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":100,"output_tokens":12,"total_tokens":112,"input_tokens_details":{"cached_tokens":25},"output_tokens_details":{"reasoning_tokens":8}}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseFrame("response.completed", completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	usageEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeUsage)
	if !ok {
		t.Fatal("missing UsageDelta event")
	}
	usage, ok := usageEvent.Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("UsageDelta Data type = %T, want provider.UsageData", usageEvent.Delta.Data)
	}
	if usage.ReasoningTokens != 8 {
		t.Errorf("ReasoningTokens = %d, want 8", usage.ReasoningTokens)
	}
	if usage.CacheReadInputTokens != 25 {
		t.Errorf("CacheReadInputTokens = %d, want 25", usage.CacheReadInputTokens)
	}
}

// TestComplete_UsageWithCachedTokens 验证非流式响应的 cached_tokens 被正确提取。
func TestComplete_UsageWithCachedTokens(t *testing.T) {
	body := `{
		"id": "resp_001",
		"object": "response",
		"status": "completed",
		"model": "gpt-4o",
		"output": [{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hi"}]}],
		"usage": {"input_tokens":100,"output_tokens":12,"total_tokens":112,"input_tokens_details":{"cached_tokens":25},"output_tokens_details":{"reasoning_tokens":8}}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := p.Complete(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gpt-4o"})
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

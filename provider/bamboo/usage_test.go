package bamboo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestChat_UsageCacheStatsPropagated 验证流式 usage 的缓存统计完整透传。
//
// 回归场景：bamboo 原生协议在 message_start 携带输入与缓存统计、
// 在 message_delta 携带最终 output_tokens，两处都必须被提取，
// 否则上层记录到的缓存创建 / 缓存命中数会缺失。
func TestChat_UsageCacheStatsPropagated(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":175,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","usage":{"input_tokens":175,"output_tokens":12}}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	var usageDeltas []provider.UsageData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeUsage {
			d, _ := ev.Delta.Data.(provider.UsageData)
			usageDeltas = append(usageDeltas, d)
		}
	}
	if len(usageDeltas) != 2 {
		t.Fatalf("expected 2 usage deltas, got %d", len(usageDeltas))
	}

	// 第一段（message_start）：输入与缓存统计
	first := usageDeltas[0]
	if first.InputTokens != 175 {
		t.Errorf("first InputTokens = %d, want 175", first.InputTokens)
	}
	if first.CacheCreationInputTokens != 50 {
		t.Errorf("first CacheCreationInputTokens = %d, want 50", first.CacheCreationInputTokens)
	}
	if first.CacheReadInputTokens != 25 {
		t.Errorf("first CacheReadInputTokens = %d, want 25", first.CacheReadInputTokens)
	}

	// 第二段（message_delta）：最终输出 token
	if usageDeltas[1].OutputTokens != 12 {
		t.Errorf("second OutputTokens = %d, want 12", usageDeltas[1].OutputTokens)
	}
}

// TestChat_UsageWithoutCacheStats 验证上游未上报缓存时缓存统计为零且不报错。
func TestChat_UsageWithoutCacheStats(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	usageEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeUsage)
	if !ok {
		t.Fatal("missing UsageDelta event")
	}
	usage, ok := usageEvent.Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("UsageDelta Data type = %T, want provider.UsageData", usageEvent.Delta.Data)
	}
	if usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", usage.InputTokens)
	}
	if usage.CacheCreationInputTokens != 0 || usage.CacheReadInputTokens != 0 {
		t.Errorf("cache stats = %+v, want both zero", usage)
	}
}

// TestComplete_UsageCacheStatsPropagated 验证非流式响应的缓存统计完整透传。
func TestComplete_UsageCacheStatsPropagated(t *testing.T) {
	body := `{
		"id": "resp_001",
		"type": "message",
		"role": "assistant",
		"content": [{"type":"text","text":"Hi"}],
		"model": "test-model",
		"stop_reason": "end_turn",
		"usage": {"input_tokens":175,"output_tokens":12,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Usage.InputTokens != 175 {
		t.Errorf("InputTokens = %d, want 175", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 12 {
		t.Errorf("OutputTokens = %d, want 12", result.Usage.OutputTokens)
	}
	if result.Usage.CacheCreationInputTokens != 50 {
		t.Errorf("CacheCreationInputTokens = %d, want 50", result.Usage.CacheCreationInputTokens)
	}
	if result.Usage.CacheReadInputTokens != 25 {
		t.Errorf("CacheReadInputTokens = %d, want 25", result.Usage.CacheReadInputTokens)
	}
}

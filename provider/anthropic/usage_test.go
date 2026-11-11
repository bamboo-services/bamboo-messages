package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestUsage_CacheCreationContract_NativeField 锁定 Anthropic 协议的缓存字段契约。
//
// 契约（与 OpenAI / Responses / Gemini 形成对照）：
//   - Anthropic 原生提供 cache_creation_input_tokens 与 cache_read_input_tokens
//     两个独立字段，因此两者都必须原样透传，绝不能被当作「无原生字段」而清零；
//   - 三个无原生 cache_creation 字段的协议恒为 0 属协议缺字段的确定行为，
//     而 Anthropic 恒为非零值则证明缓存字段的解析链路本身是通的 ——
//
// //     二者共同构成「缺字段」与「解析失败」的判别依据。
//
// 该契约同时覆盖流式与非流式两条路径。
func TestUsage_CacheCreationContract_NativeField(t *testing.T) {
	// ── 流式路径 ──
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":12,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}`},
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
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
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
	if usageDeltas[0].CacheCreationInputTokens != 50 {
		t.Errorf("[stream] CacheCreationInputTokens = %d, want 50（原生字段必须原样透传）", usageDeltas[0].CacheCreationInputTokens)
	}
	if usageDeltas[0].CacheReadInputTokens != 25 {
		t.Errorf("[stream] CacheReadInputTokens = %d, want 25（缓存命中数必须正常透传）", usageDeltas[0].CacheReadInputTokens)
	}
	// message_delta 顶层的 output_tokens 必须能解析出来。
	if usageDeltas[1].OutputTokens != 12 {
		t.Errorf("[stream] OutputTokens = %d, want 12（message_delta 顶层 usage 必须被解析）", usageDeltas[1].OutputTokens)
	}
	// message_delta 顶层的 usage 是累计值（含 cache 字段），必须原样透传，不得清零。
	if usageDeltas[1].CacheCreationInputTokens != 50 {
		t.Errorf("[stream] CacheCreationInputTokens = %d, want 50（原生字段必须原样透传）", usageDeltas[1].CacheCreationInputTokens)
	}
	if usageDeltas[1].CacheReadInputTokens != 25 {
		t.Errorf("[stream] CacheReadInputTokens = %d, want 25（缓存命中数必须正常透传）", usageDeltas[1].CacheReadInputTokens)
	}

	// ── 非流式路径 ──
	completeBody := `{
		"id": "msg_001",
		"type": "message",
		"role": "assistant",
		"content": [{"type":"text","text":"Hi"}],
		"model": "claude-sonnet-4-20250514",
		"stop_reason": "end_turn",
		"usage": {"input_tokens":100,"output_tokens":12,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}
	}`
	completeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(completeBody))
	}))
	defer completeServer.Close()

	cp := newMockProvider(t, completeServer)
	result, err := cp.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Usage.CacheCreationInputTokens != 50 {
		t.Errorf("[complete] CacheCreationInputTokens = %d, want 50（原生字段必须原样透传）", result.Usage.CacheCreationInputTokens)
	}
	if result.Usage.CacheReadInputTokens != 25 {
		t.Errorf("[complete] CacheReadInputTokens = %d, want 25（缓存命中数必须正常透传）", result.Usage.CacheReadInputTokens)
	}
}

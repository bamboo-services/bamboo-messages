package bamboo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider/anthropic"
)

// anthropicCacheSSE 模拟 Anthropic 官方流式响应。
//
// 关键点：Anthropic 把 usage 拆成两处——message_start 携带 input_tokens 与缓存统计，
// message_delta 顶层携带最终 output_tokens。
const anthropicCacheSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-4","content":[],"stop_reason":null,"usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":12}}

event: message_stop
data: {"type":"message_stop"}

`

// newAnthropicCacheServer 启动一个返回固定 Anthropic SSE 的 mock 上游。
func newAnthropicCacheServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(anthropicCacheSSE))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestChat_AnthropicCacheUsageRecorded 验证客户端层能完整记录 Anthropic 的缓存与输出统计。
//
// 回归场景：message_delta 顶层的 usage 此前未被解析，导致 output_tokens 丢失；
// 且后到的 usage 增量会覆盖 message_start 的缓存统计。
func TestChat_AnthropicCacheUsageRecorded(t *testing.T) {
	srv := newAnthropicCacheServer(t)

	p := anthropic.NewProviderWithOptions(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(srv.URL),
	)
	client := NewClientWithOptions(WithProvider(p), WithDefaultModel("claude-sonnet-4"))

	events, err := client.Chat(context.Background(), []BambooMessage{
		{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}},
	}, "", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	var final *Usage
	var ping *Usage
	for ev := range events {
		if ev.Usage == nil {
			continue
		}
		usage := *ev.Usage
		switch ev.Type {
		case EventPing:
			ping = &usage
		case EventMessageDelta:
			final = &usage
		}
	}

	if final == nil {
		t.Fatal("expected final usage in message_delta event")
	}
	if final.InputTokens != 175 {
		t.Errorf("final InputTokens = %d, want 175", final.InputTokens)
	}
	if final.OutputTokens != 12 {
		t.Errorf("final OutputTokens = %d, want 12", final.OutputTokens)
	}
	if final.CacheCreationInputTokens != 50 {
		t.Errorf("final CacheCreationInputTokens = %d, want 50", final.CacheCreationInputTokens)
	}
	if final.CacheReadInputTokens != 25 {
		t.Errorf("final CacheReadInputTokens = %d, want 25", final.CacheReadInputTokens)
	}

	// 流中途的 usage 增量（ping）必须同样带上缓存统计，供中断场景兜底。
	if ping == nil {
		t.Fatal("expected usage in ping event")
	}
	if ping.CacheCreationInputTokens != 50 || ping.CacheReadInputTokens != 25 {
		t.Errorf("ping cache usage = %+v, want cache_creation=50 cache_read=25", *ping)
	}
}

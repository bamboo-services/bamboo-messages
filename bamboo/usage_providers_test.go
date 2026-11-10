package bamboo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	bambooProvider "github.com/bamboo-services/bamboo-messages/provider/bamboo"
	"github.com/bamboo-services/bamboo-messages/provider/gemini"
	"github.com/bamboo-services/bamboo-messages/provider/openai/completions"
	"github.com/bamboo-services/bamboo-messages/provider/openai/responses"
)

// usageCase 描述一个协议在客户端层的 usage / 缓存统计预期。
type usageCase struct {
	name string
	// body 为上游返回的原始响应体（流式为 SSE，非流式为 JSON）。
	body string
	// contentType 为上游响应的 Content-Type。
	contentType string
	// provider 构造指向 mock server 的协议适配器。
	provider func(baseURL string) provider.Provider
	// want 为客户端层最终应记录到的用量。
	want Usage
}

// TestChat_UsageAcrossProviders 验证各协议适配器的 usage / 缓存统计在客户端层被完整记录。
//
// 回归场景：
//   - Anthropic 把 usage 拆为 message_start（输入+缓存）与 message_delta 顶层（输出）；
//   - OpenAI / Responses / Gemini 在末尾一次性给出 usage；
//   - bamboo 原生协议在 message_start 与 message_delta 各给一段。
//
// 任一环节丢失字段，客户端最终记录到的用量就会缺项。
func TestChat_UsageAcrossProviders(t *testing.T) {
	cases := []usageCase{
		{
			name:        "openai_completions",
			contentType: "text/event-stream",
			body: `data: {"id":"1","choices":[{"index":0,"delta":{"content":"Hi"}}]}

data: {"id":"2","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":12,"total_tokens":112,"prompt_tokens_details":{"cached_tokens":25}}}

data: [DONE]

`,
			provider: func(baseURL string) provider.Provider {
				return completions.NewCompletionsProviderWithOptions(
					completions.WithAPIKey("test-key"),
					completions.WithBaseURL(baseURL),
				)
			},
			want: Usage{InputTokens: 100, OutputTokens: 12, CacheReadInputTokens: 25},
		},
		{
			name:        "openai_responses",
			contentType: "text/event-stream",
			body: `event: response.completed
data: {"type":"response.completed","response":{"id":"resp_001","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":100,"output_tokens":12,"total_tokens":112,"input_tokens_details":{"cached_tokens":25}}}}

`,
			provider: func(baseURL string) provider.Provider {
				return responses.NewResponsesProviderWithOptions(
					responses.WithAPIKey("test-key"),
					responses.WithBaseURL(baseURL),
				)
			},
			want: Usage{InputTokens: 100, OutputTokens: 12, CacheReadInputTokens: 25},
		},
		{
			name:        "gemini",
			contentType: "text/event-stream",
			body: `data: {"candidates":[{"content":{"parts":[{"text":"Hi"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":12,"totalTokenCount":112,"cachedContentTokenCount":25}}

`,
			provider: func(baseURL string) provider.Provider {
				return gemini.NewProviderWithOptions(
					gemini.WithAPIKey("test-key"),
					gemini.WithBaseURL(baseURL),
				)
			},
			want: Usage{InputTokens: 100, OutputTokens: 12, CacheReadInputTokens: 25},
		},
		{
			name:        "bamboo_native",
			contentType: "text/event-stream",
			body: `event: message_start
data: {"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":175,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","usage":{"input_tokens":175,"output_tokens":12}}}

event: message_stop
data: {"type":"message_stop"}

`,
			provider: func(baseURL string) provider.Provider {
				return bambooProvider.NewProviderWithOptions(
					bambooProvider.WithAPIKey("test-key"),
					bambooProvider.WithBaseURL(baseURL),
				)
			},
			want: Usage{
				InputTokens:              175,
				OutputTokens:             12,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     25,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			client := NewClientWithOptions(WithProvider(tc.provider(srv.URL)))

			events, err := client.Chat(context.Background(), []BambooMessage{
				{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hello")}},
			}, "", &RequestConfig{Model: "test-model"})
			if err != nil {
				t.Fatalf("Chat() error = %v", err)
			}

			var final *Usage
			for ev := range events {
				if ev.Type == EventMessageDelta && ev.Usage != nil {
					usage := *ev.Usage
					final = &usage
				}
			}
			if final == nil {
				t.Fatal("expected final usage in message_delta event")
			}
			if final.InputTokens != tc.want.InputTokens {
				t.Errorf("InputTokens = %d, want %d", final.InputTokens, tc.want.InputTokens)
			}
			if final.OutputTokens != tc.want.OutputTokens {
				t.Errorf("OutputTokens = %d, want %d", final.OutputTokens, tc.want.OutputTokens)
			}
			if final.CacheCreationInputTokens != tc.want.CacheCreationInputTokens {
				t.Errorf("CacheCreationInputTokens = %d, want %d",
					final.CacheCreationInputTokens, tc.want.CacheCreationInputTokens)
			}
			if final.CacheReadInputTokens != tc.want.CacheReadInputTokens {
				t.Errorf("CacheReadInputTokens = %d, want %d",
					final.CacheReadInputTokens, tc.want.CacheReadInputTokens)
			}
		})
	}
}

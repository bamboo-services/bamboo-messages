package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/bamboo"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/gemini"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
	geminiprovider "github.com/bamboo-services/bamboo-messages/provider/gemini"
)

// newMockCachedStreamProvider 创建流式 mock provider，模拟上游分段上报 usage：
// 先给输入与缓存统计（message_start 阶段），再给最终输出 token（message_delta 阶段）。
func newMockCachedStreamProvider() *mockProvider {
	return &mockProvider{
		streamEvents: []provider.StreamEvent{
			{Type: provider.StreamTypeStart},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewUsageDeltaWithCache(175, 0, 30, 70),
			},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewTextDelta("hello"),
			},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewUsageDeltaWithCache(0, 12, 0, 0),
			},
			{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonStop},
			{Type: provider.StreamTypeDone},
		},
	}
}

// TestRelayStream_CacheUsageAcrossFormats 验证缓存统计在跨协议输出时不被丢弃。
//
// 回归场景：relay 层把 Provider 的 usage 映射到各目标协议的原生字段，
// 任一目标协议漏映射，下游就会记录不到缓存命中数。
func TestRelayStream_CacheUsageAcrossFormats(t *testing.T) {
	mp := newMockCachedStreamProvider()

	cases := []struct {
		name string
		// outFormat 目标协议格式。
		outFormat codec.FormatType
		// want 期望在输出帧中出现的字符串片段。
		want []string
	}{
		{
			name:      "anthropic",
			outFormat: codec.FormatAnthropic,
			want:      []string{`"cache_creation_input_tokens":30`, `"cache_read_input_tokens":70`},
		},
		{
			name:      "openai",
			outFormat: codec.FormatOpenAI,
			// OpenAI 无 cache_creation 字段，仅映射 cached_tokens
			want: []string{`"cached_tokens":70`},
		},
		{
			name:      "responses",
			outFormat: codec.FormatResponses,
			want:      []string{`"cached_tokens":70`},
		},
		{
			name:      "gemini",
			outFormat: codec.FormatGemini,
			want:      []string{`"cachedContentTokenCount":70`},
		},
		{
			name:      "bamboo",
			outFormat: codec.FormatBamboo,
			want:      []string{`"cache_creation_input_tokens":30`, `"cache_read_input_tokens":70`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch, err := RelayStream(context.Background(), mp, bambooBody,
				codec.FormatBamboo, tc.outFormat)
			if err != nil {
				t.Fatalf("RelayStream() error = %v", err)
			}

			var sb strings.Builder
			for chunk := range ch {
				sb.Write(chunk)
			}
			out := sb.String()
			if out == "" {
				t.Fatal("expected non-empty output")
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("expected %s in %s output, got: %s", want, tc.name, out)
				}
			}
		})
	}
}

// TestRelay_CacheUsageAcrossFormats 验证非流式跨协议输出同样保留缓存统计。
func TestRelay_CacheUsageAcrossFormats(t *testing.T) {
	mp := newMockCacheProvider()

	cases := []struct {
		name      string
		outFormat codec.FormatType
		want      []string
	}{
		{
			name:      "anthropic",
			outFormat: codec.FormatAnthropic,
			want:      []string{`"cache_creation_input_tokens":30`, `"cache_read_input_tokens":70`},
		},
		{
			name:      "openai",
			outFormat: codec.FormatOpenAI,
			want:      []string{`"cached_tokens":70`},
		},
		{
			name:      "responses",
			outFormat: codec.FormatResponses,
			want:      []string{`"cached_tokens":70`},
		},
		{
			name:      "gemini",
			outFormat: codec.FormatGemini,
			want:      []string{`"cachedContentTokenCount":70`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, tc.outFormat)
			if err != nil {
				t.Fatalf("Relay() error = %v", err)
			}

			// 输出必须是合法 JSON
			var parsed map[string]any
			if err := json.Unmarshal(out, &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out)
			}
			for _, want := range tc.want {
				if !strings.Contains(string(out), want) {
					t.Errorf("expected %s in %s output, got: %s", want, tc.name, out)
				}
			}
		})
	}
}

// TestRelayStream_UsageCallbackCarriesCacheStats 验证 usage 回调携带完整缓存统计。
//
// 回归场景：上游分两段上报 usage（先输入+缓存，后输出），
// 若 relay 层只取最后一段，缓存统计就会在回调中丢失。
func TestRelayStream_UsageCallbackCarriesCacheStats(t *testing.T) {
	mp := newMockCachedStreamProvider()

	var usages []bamboo.Usage
	ch, err := RelayStream(context.Background(), mp, bambooBody,
		codec.FormatBamboo, codec.FormatBamboo,
		WithUsageCallback(func(u bamboo.Usage) {
			usages = append(usages, u)
		}))
	if err != nil {
		t.Fatalf("RelayStream() error = %v", err)
	}
	for range ch {
	}

	if len(usages) == 0 {
		t.Fatal("expected usage callback")
	}
	last := usages[len(usages)-1]
	if last.CacheCreationInputTokens != 30 {
		t.Errorf("CacheCreationInputTokens = %d, want 30", last.CacheCreationInputTokens)
	}
	if last.CacheReadInputTokens != 70 {
		t.Errorf("CacheReadInputTokens = %d, want 70", last.CacheReadInputTokens)
	}
	if last.InputTokens != 175 {
		t.Errorf("InputTokens = %d, want 175", last.InputTokens)
	}
	if last.OutputTokens != 12 {
		t.Errorf("OutputTokens = %d, want 12", last.OutputTokens)
	}
}

// TestRelay_OpenAIToGemini_CachedContentAndExtraPassthrough 验证 Chat Completions 请求
// 转发到 Gemini 上游时的完整链路：
//   - forced_tool_name 在同时携带 seed 等字段时不被 ProviderExtra 覆盖；
//   - cached_content 透传为 Gemini 顶层 cachedContent，并自动剥离冲突的 systemInstruction/tools/toolConfig。
func TestRelay_OpenAIToGemini_CachedContentAndExtraPassthrough(t *testing.T) {
	t.Run("forced_tool_name preserved alongside seed", func(t *testing.T) {
		var captured map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode upstream request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":5,"cachedContentTokenCount":30}}`))
		}))
		defer srv.Close()

		gp := geminiprovider.NewProviderWithOptions(
			geminiprovider.WithAPIKey("test-key"),
			geminiprovider.WithBaseURL(srv.URL),
		)

		reqBody := []byte(`{
			"model": "gemini-2.5-pro",
			"messages": [{"role": "user", "content": "hello"}],
			"tools": [{"type": "function", "function": {"name": "get_weather"}}],
			"tool_choice": {"type": "function", "function": {"name": "get_weather"}},
			"seed": 7
		}`)

		out, err := Relay(context.Background(), gp, reqBody, codec.FormatOpenAI, codec.FormatOpenAI)
		if err != nil {
			t.Fatalf("Relay() error = %v", err)
		}

		tc, ok := captured["toolConfig"].(map[string]any)
		if !ok {
			t.Fatalf("expected toolConfig in upstream request, got %v", captured["toolConfig"])
		}
		fcc, ok := tc["functionCallingConfig"].(map[string]any)
		if !ok {
			t.Fatalf("expected functionCallingConfig, got %v", tc["functionCallingConfig"])
		}
		allowed, ok := fcc["allowedFunctionNames"].([]any)
		if !ok || len(allowed) != 1 || allowed[0] != "get_weather" {
			t.Errorf("upstream allowedFunctionNames = %v, want [get_weather] (forced_tool_name must not be overwritten)", fcc["allowedFunctionNames"])
		}
		if !strings.Contains(string(out), `"cached_tokens":30`) {
			t.Errorf("expected cached_tokens:30 in OpenAI response, got: %s", out)
		}
	})

	t.Run("cached_content passthrough strips conflicting fields", func(t *testing.T) {
		var captured map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode upstream request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":5,"cachedContentTokenCount":30}}`))
		}))
		defer srv.Close()

		gp := geminiprovider.NewProviderWithOptions(
			geminiprovider.WithAPIKey("test-key"),
			geminiprovider.WithBaseURL(srv.URL),
		)

		reqBody := []byte(`{
			"model": "gemini-2.5-pro",
			"messages": [
				{"role": "system", "content": "you are helpful"},
				{"role": "user", "content": "hello"}
			],
			"tools": [{"type": "function", "function": {"name": "get_weather"}}],
			"tool_choice": {"type": "function", "function": {"name": "get_weather"}},
			"cached_content": "cachedContents/chat-to-google-1",
			"seed": 7
		}`)

		out, err := Relay(context.Background(), gp, reqBody, codec.FormatOpenAI, codec.FormatOpenAI)
		if err != nil {
			t.Fatalf("Relay() error = %v", err)
		}

		if captured["cachedContent"] != "cachedContents/chat-to-google-1" {
			t.Errorf("upstream cachedContent = %v, want cachedContents/chat-to-google-1", captured["cachedContent"])
		}
		for _, forbidden := range []string{"systemInstruction", "tools", "toolConfig"} {
			if _, exists := captured[forbidden]; exists {
				t.Errorf("upstream %s must be stripped when cachedContent is set, got %v", forbidden, captured[forbidden])
			}
		}
		if !strings.Contains(string(out), `"cached_tokens":30`) {
			t.Errorf("expected cached_tokens:30 in OpenAI response, got: %s", out)
		}
	})
}

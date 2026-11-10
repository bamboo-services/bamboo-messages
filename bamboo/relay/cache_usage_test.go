package relay

import (
	"context"
	"encoding/json"
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

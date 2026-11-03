package relay

import (
	"context"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/bamboo"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/gemini"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestRelay_ServiceTier_CrossProtocol 验证各协议输入通过 Relay 管道时 ServiceTier 是否完整且正确地传递给下游 Provider。
func TestRelay_ServiceTier_CrossProtocol(t *testing.T) {
	tests := []struct {
		name            string
		inFormat        codec.FormatType
		outFormat       codec.FormatType
		body            []byte
		wantServiceTier string
	}{
		{
			name:      "Anthropic speed=fast → Provider",
			inFormat:  codec.FormatAnthropic,
			outFormat: codec.FormatAnthropic,
			body: []byte(`{
				"model": "claude-opus-5-5",
				"max_tokens": 1024,
				"speed": "fast",
				"messages": [{"role": "user", "content": "hello"}]
			}`),
			wantServiceTier: "priority",
		},
		{
			name:      "Anthropic service_tier=auto → Provider",
			inFormat:  codec.FormatAnthropic,
			outFormat: codec.FormatAnthropic,
			body: []byte(`{
				"model": "claude-opus-5-5",
				"max_tokens": 1024,
				"service_tier": "auto",
				"messages": [{"role": "user", "content": "hello"}]
			}`),
			wantServiceTier: "auto",
		},
		{
			name:      "Responses service_tier=priority → Provider",
			inFormat:  codec.FormatResponses,
			outFormat: codec.FormatOpenAI,
			body: []byte(`{
				"model": "gpt-4o",
				"input": "hello",
				"service_tier": "priority"
			}`),
			wantServiceTier: "priority",
		},
		{
			name:      "Responses service_tier=flex → Provider",
			inFormat:  codec.FormatResponses,
			outFormat: codec.FormatOpenAI,
			body: []byte(`{
				"model": "gpt-4o",
				"input": "hello",
				"service_tier": "flex"
			}`),
			wantServiceTier: "flex",
		},
		{
			name:      "Gemini service_tier=priority → Provider",
			inFormat:  codec.FormatGemini,
			outFormat: codec.FormatAnthropic,
			body: []byte(`{
				"contents": [{"role": "user", "parts": [{"text": "hello"}]}],
				"service_tier": "priority"
			}`),
			wantServiceTier: "priority",
		},
		{
			name:      "OpenAI service_tier=priority → Provider",
			inFormat:  codec.FormatOpenAI,
			outFormat: codec.FormatResponses,
			body: []byte(`{
				"model": "gpt-4o",
				"messages": [{"role": "user", "content": "hello"}],
				"service_tier": "priority"
			}`),
			wantServiceTier: "priority",
		},
		{
			name:      "Bamboo native service_tier=priority → Provider",
			inFormat:  codec.FormatBamboo,
			outFormat: codec.FormatBamboo,
			body: []byte(`{
				"messages": [{"role": "user", "content": [{"type": "text", "text": "hello"}]}],
				"config": {"model": "test-model", "service_tier": "priority"}
			}`),
			wantServiceTier: "priority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := &mockProvider{
				completeResult: &provider.CompletionResult{
					Content: "ok",
				},
			}

			_, err := Relay(context.Background(), mp, tt.body, tt.inFormat, tt.outFormat)
			if err != nil {
				t.Fatalf("Relay() error = %v", err)
			}

			if mp.lastConfig == nil {
				t.Fatal("expected mp.lastConfig to be non-nil")
			}

			if mp.lastConfig.ServiceTier != tt.wantServiceTier {
				t.Errorf("lastConfig.ServiceTier = %q, want %q", mp.lastConfig.ServiceTier, tt.wantServiceTier)
			}
		})
	}
}

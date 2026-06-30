package completions

import (
	"encoding/json"
	"testing"

	anthropicCodec "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestIntegration_AnthropicAdaptiveToLegacyEnabled 验证跨协议链路：
// Anthropic 入口 → parseRequest → RelayRequest.Config → provider.ChatConfig
// → CompletionsProvider.buildParams (legacyCompat=true) → params["thinking"]
// 期望：原始 thinking {type:"adaptive"} 被归一化为 {type:"enabled"}。
func TestIntegration_AnthropicAdaptiveToLegacyEnabled(t *testing.T) {
	// 1. 构造 Anthropic 请求（含 thinking:{type:"adaptive"}）
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"thinking": {"type": "adaptive"},
		"messages": [{"role": "user", "content": "Think"}]
	}`)

	// 2. Anthropic codec → RelayRequest
	relayReq, err := anthropicCodec.Codec.ParseRequest(body)
	if err != nil {
		t.Fatalf("anthropic.Codec.ParseRequest error = %v", err)
	}

	// 3. RelayRequest.Config → provider.ChatConfig
	rawThinking, ok := relayReq.Config.ProviderExtra["thinking"]
	if !ok {
		t.Fatal("ProviderExtra[thinking] missing after parseRequest")
	}
	t.Logf("ProviderExtra[thinking] raw = %s (type %T)", mustJSONString(rawThinking), rawThinking)
	if _, isRaw := rawThinking.(json.RawMessage); !isRaw {
		t.Errorf("expected ProviderExtra[thinking] to be json.RawMessage, got %T", rawThinking)
	}

	chatCfg := &provider.ChatConfig{
		Model:          relayReq.Config.Model,
		MaxTokens:      relayReq.Config.MaxTokens,
		ThinkingConfig: relayReq.Config.ThinkingConfig,
		ProviderExtra:  relayReq.Config.ProviderExtra,
	}

	// 4. CompletionsProvider (legacyCompat=true) buildParams
	p := newLegacyProvider(t)
	params := p.buildParams("", nil, chatCfg)

	// 5. 检查 params["thinking"] = {"type":"enabled"}
	thinking, ok := params["thinking"]
	if !ok {
		t.Fatal("expected 'thinking' in params")
	}
	t.Logf("params[thinking] = %s (type %T)", mustJSONString(thinking), thinking)

	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("thinking value type = %T, want map[string]any", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Errorf("thinking.type = %v, want 'enabled' (normalized from adaptive)", thinkingMap["type"])
	}

	// 6. ThinkingConfig.Effort 也应被 parseThinking 正确填充为 "high"
	if chatCfg.ThinkingConfig == nil {
		t.Fatal("ThinkingConfig should not be nil after parseRequest")
	}
	if chatCfg.ThinkingConfig.Effort != "high" {
		t.Errorf("ThinkingConfig.Effort = %q, want 'high'", chatCfg.ThinkingConfig.Effort)
	}
}

// mustJSONString 将任意值序列化为 JSON 字符串用于日志输出。
func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "<marshal error>"
	}
	return string(b)
}

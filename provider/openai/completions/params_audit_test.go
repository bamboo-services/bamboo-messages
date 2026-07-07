package completions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestAudit_Prediction_TypeAssertion 验证 Prediction 参数通过 ProviderExtra 透传。
//
// 去 SDK 化后，Prediction 通过 GetExtraAny 获取后直接写入 params["prediction"]，
// 不再做 SDK 类型断言，因此 map[string]any 和其他可序列化类型都能正确透传。
func TestAudit_Prediction_TypeAssertion(t *testing.T) {
	p := NewCompletionsProvider("test-key")

	// 通过 ProviderExtra 传入 map[string]any（模拟 JSON 反序列化后的值）
	config := &provider.ChatConfig{
		Model:     "gpt-4o",
		MaxTokens: 1024,
		ProviderExtra: map[string]any{
			"prediction": map[string]any{
				"type":    "content",
				"content": "Hello world",
			},
		},
	}

	params := p.buildParams("", nil, config)

	pred, ok := params["prediction"]
	if !ok {
		t.Fatal("Prediction should be set from ProviderExtra")
	}
	predMap, ok := pred.(map[string]any)
	if !ok {
		t.Fatalf("Prediction type = %T, want map[string]any", pred)
	}
	if predMap["type"] != "content" {
		t.Errorf("Prediction type = %v, want 'content'", predMap["type"])
	}
}

// TestBuildParams_LegacyCompat_FieldFiltering 验证 Legacy 模式过滤 SystemCacheControl 与 PromptCacheKey 字段泄漏。
func TestBuildParams_LegacyCompat_FieldFiltering(t *testing.T) {
	// === Legacy 模式 ===
	legacyP := newLegacyProvider(t)
	legacyConfig := &provider.ChatConfig{
		Model:              "glm-4",
		MaxTokens:          1024,
		PromptCacheKey:     "session-abc-123",
		SystemCacheControl: provider.NewEphemeralCacheControl(),
		ProviderExtra: map[string]any{
			"thinking": map[string]any{"type": "enabled"},
		},
	}
	legacyParams := legacyP.buildParams("", nil, legacyConfig)
	legacyJSON, err := json.Marshal(legacyParams)
	if err != nil {
		t.Fatalf("Legacy: json.Marshal failed: %v", err)
	}
	legacyStr := string(legacyJSON)

	if strings.Contains(legacyStr, "system_cache_control") {
		t.Errorf("Legacy JSON should NOT contain 'system_cache_control' key, got:\n%s", legacyStr)
	}
	if strings.Contains(legacyStr, "prompt_cache_key") {
		t.Errorf("Legacy JSON should NOT contain 'prompt_cache_key' key, got:\n%s", legacyStr)
	}
	if !strings.Contains(legacyStr, `"thinking"`) {
		t.Errorf("Legacy JSON should contain 'thinking' extra field, got:\n%s", legacyStr)
	}

	// === 默认模式（回归保护）===
	defaultP := newDefaultProvider(t)
	defaultConfig := &provider.ChatConfig{
		Model:          "gpt-4o",
		MaxTokens:      1024,
		PromptCacheKey: "session-xyz-789",
	}
	defaultParams := defaultP.buildParams("", nil, defaultConfig)
	defaultJSON, err := json.Marshal(defaultParams)
	if err != nil {
		t.Fatalf("Default: json.Marshal failed: %v", err)
	}
	defaultStr := string(defaultJSON)

	if !strings.Contains(defaultStr, "prompt_cache_key") {
		t.Errorf("Default JSON should contain 'prompt_cache_key' key, got:\n%s", defaultStr)
	}
}

// TestAudit_ResponseFormat_Mapping 验证 ResponseFormat 映射到 Chat Completions。
func TestAudit_ResponseFormat_Mapping(t *testing.T) {
	p := NewCompletionsProvider("test-key")

	tests := []struct {
		name           string
		responseFormat string
		wantType       string
	}{
		{"json_object", "json_object", "json_object"},
		{"text", "text", "text"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &provider.ChatConfig{
				Model:          "gpt-4o",
				MaxTokens:      1024,
				ResponseFormat: tt.responseFormat,
			}

			params := p.buildParams("", nil, config)

			if tt.wantType == "" {
				if _, exists := params["response_format"]; exists {
					t.Errorf("response_format should not be set for empty input")
				}
				return
			}
			rf, ok := params["response_format"].(map[string]any)
			if !ok {
				t.Fatalf("response_format not set or wrong type for input %q", tt.responseFormat)
			}
			if rf["type"] != tt.wantType {
				t.Errorf("response_format.type = %v, want %q", rf["type"], tt.wantType)
			}
		})
	}
}

func TestAudit_PromptCacheKey_NonLegacy(t *testing.T) {
	p := NewCompletionsProvider("test-key")
	config := &provider.ChatConfig{
		Model:           "gpt-4o",
		MaxTokens:       1024,
		PromptCacheKey:  "session-abc",
	}
	params := p.buildParams("", nil, config)
	if v, ok := params["prompt_cache_key"].(string); !ok || v != "session-abc" {
		t.Errorf("prompt_cache_key = %v, want %q", params["prompt_cache_key"], "session-abc")
	}
}

func TestAudit_PromptCacheKey_LegacyDefault_Skipped(t *testing.T) {
	p := NewCompletionsProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		Model:          "gpt-4o",
		MaxTokens:      1024,
		PromptCacheKey: "session-abc",
	}
	params := p.buildParams("", nil, config)
	if _, ok := params["prompt_cache_key"]; ok {
		t.Error("prompt_cache_key should NOT be sent in Legacy mode without WithLegacyCacheKey")
	}
}

func TestAudit_PromptCacheKey_LegacyWithCacheKey_Enabled(t *testing.T) {
	p := NewCompletionsProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat(), WithLegacyCacheKey(true))
	config := &provider.ChatConfig{
		Model:          "gpt-4o",
		MaxTokens:      1024,
		PromptCacheKey: "session-abc",
	}
	params := p.buildParams("", nil, config)
	if v, ok := params["prompt_cache_key"].(string); !ok || v != "session-abc" {
		t.Errorf("prompt_cache_key = %v, want %q", params["prompt_cache_key"], "session-abc")
	}
}

func TestAudit_PromptCacheKey_LegacyProviderExtra(t *testing.T) {
	p := NewCompletionsProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat(), WithLegacyCacheKey(true))
	config := &provider.ChatConfig{
		Model:         "gpt-4o",
		MaxTokens:     1024,
		ProviderExtra: map[string]any{"prompt_cache_key": "extra-key"},
	}
	params := p.buildParams("", nil, config)
	if v, ok := params["prompt_cache_key"].(string); !ok || v != "extra-key" {
		t.Errorf("prompt_cache_key = %v, want %q", params["prompt_cache_key"], "extra-key")
	}
}

func TestAudit_PromptCacheKey_LegacyExplicitOverridesProviderExtra(t *testing.T) {
	p := NewCompletionsProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat(), WithLegacyCacheKey(true))
	config := &provider.ChatConfig{
		Model:          "gpt-4o",
		MaxTokens:      1024,
		PromptCacheKey: "explicit-key",
		ProviderExtra:  map[string]any{"prompt_cache_key": "extra-key"},
	}
	params := p.buildParams("", nil, config)
	if v, ok := params["prompt_cache_key"].(string); !ok || v != "explicit-key" {
		t.Errorf("prompt_cache_key = %v, want %q (explicit should override extra)", params["prompt_cache_key"], "explicit-key")
	}
}

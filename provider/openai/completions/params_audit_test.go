package completions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
)

// TestAudit_Prediction_TypeAssertion 验证 Prediction 参数的类型断言。
//
// Severity: P1
// File:Line: provider/openai/completions/params.go:96
// Issue: Prediction 通过 ProviderExtra 存储时，使用 GetExtraAny 获取后做
//        pred.(openai.ChatCompletionPredictionContentParam) 类型断言。
//        如果 Prediction 值是通过 JSON 反序列化或 WithPrediction(any) 传入的，
//        实际类型可能是 map[string]any 而非 openai.ChatCompletionPredictionContentParam，
//        导致断言失败，Prediction 被静默丢弃。
// Affected: Any→OpenAI Completions with Prediction set via ProviderExtra
func TestAudit_Prediction_TypeAssertion(t *testing.T) {
	p := NewCompletionsProvider("test-key")

	// 场景1：通过 ProviderExtra 传入 map[string]any（模拟 JSON 反序列化后的值）
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

	// Fix: map[string]any 类型断言失败后，通过 JSON marshal/unmarshal 回退转换
	// Prediction 应该成功设置（通过 JSON 回退）
	if params.Prediction.Type == "" {
		t.Errorf("FIXED: Prediction from ProviderExtra (map[string]any) should be recovered via JSON fallback")
	} else {
		t.Logf("FIXED: Prediction from ProviderExtra (map[string]any) correctly recovered via JSON fallback, type=%q", params.Prediction.Type)
	}

	// 场景2：通过 WithPrediction 传入正确类型
	config.ProviderExtra["prediction"] = openai.ChatCompletionPredictionContentParam{
		Type:    "content",
		Content: openai.ChatCompletionPredictionContentContentUnionParam{OfString: param.NewOpt("Hello")},
	}

	params = p.buildParams("", nil, config)

	if params.Prediction.Type == "" {
		t.Errorf("Prediction with correct type was silently dropped")
	}
}

// TestBuildParams_LegacyCompat_FieldFiltering 验证 Legacy 模式过滤 SystemCacheControl 与 PromptCacheKey 字段泄漏。
//
// 智谱 GLM / Kimi 等第三方 OpenAI 兼容端点不支持 system_cache_control 和 prompt_cache_key 字段，
// 发送这些字段会导致请求被拒绝或返回异常结果。Legacy 模式应过滤这些字段。
//
// 断言：
//  1. Legacy 模式下 JSON 不含 "system_cache_control" key
//  2. Legacy 模式下 JSON 不含 "prompt_cache_key" key
//  3. Legacy 模式下 ProviderExtra["thinking"] 透传的 "thinking" 字段正常出现（设计行为，不应被过滤）
//  4. 默认模式下 "prompt_cache_key" 正常出现（回归保护）
func TestBuildParams_LegacyCompat_FieldFiltering(t *testing.T) {
	// === 场景 1-3: Legacy 模式 ===
	legacyP := newLegacyProvider(t)
	legacyConfig := &provider.ChatConfig{
		Model:           "glm-4",
		MaxTokens:       1024,
		PromptCacheKey:  "session-abc-123",
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

	// 断言 1: system_cache_control 不应出现
	if strings.Contains(legacyStr, "system_cache_control") {
		t.Errorf("Legacy JSON should NOT contain 'system_cache_control' key, got:\n%s", legacyStr)
	}

	// 断言 2: prompt_cache_key 不应出现
	if strings.Contains(legacyStr, "prompt_cache_key") {
		t.Errorf("Legacy JSON should NOT contain 'prompt_cache_key' key, got:\n%s", legacyStr)
	}

	// 断言 3: thinking 字段应正常出现（设计行为，不应被过滤）
	if !strings.Contains(legacyStr, `"thinking"`) {
		t.Errorf("Legacy JSON should contain 'thinking' extra field (design behavior), got:\n%s", legacyStr)
	}

	// === 场景 4: 默认模式（回归保护）===
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

	// 断言 4: 默认模式下 prompt_cache_key 正常出现
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
				return
			}
			if params.ResponseFormat.OfJSONObject == nil && params.ResponseFormat.OfText == nil {
				t.Errorf("ResponseFormat not set for input %q", tt.responseFormat)
			}
		})
	}
}

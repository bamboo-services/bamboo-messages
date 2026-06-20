package completions

import (
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

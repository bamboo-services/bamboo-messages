package bamboo

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

func TestWithTopK(t *testing.T) {
	cfg := &RequestConfig{}
	WithTopK(50.0)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyTopK] != 50.0 {
		t.Errorf("TopK = %v, 期望 50.0", cfg.ProviderExtra[provider.ProviderExtraKeyTopK])
	}
}

func TestWithFrequencyPenalty(t *testing.T) {
	cfg := &RequestConfig{}
	WithFrequencyPenalty(1.5)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyFrequencyPenalty] != 1.5 {
		t.Errorf("FrequencyPenalty = %v, 期望 1.5", cfg.ProviderExtra[provider.ProviderExtraKeyFrequencyPenalty])
	}
}

func TestWithPresencePenalty(t *testing.T) {
	cfg := &RequestConfig{}
	WithPresencePenalty(0.8)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyPresencePenalty] != 0.8 {
		t.Errorf("PresencePenalty = %v, 期望 0.8", cfg.ProviderExtra[provider.ProviderExtraKeyPresencePenalty])
	}
}

func TestWithSeed(t *testing.T) {
	cfg := &RequestConfig{}
	WithSeed(42)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeySeed] != int64(42) {
		t.Errorf("Seed = %v, 期望 42", cfg.ProviderExtra[provider.ProviderExtraKeySeed])
	}
}

func TestWithToolChoice(t *testing.T) {
	cfg := &RequestConfig{}
	choice := map[string]any{"type": "auto"}
	WithToolChoice(choice)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	got, ok := cfg.ProviderExtra[provider.ProviderExtraKeyToolChoice]
	if !ok {
		t.Fatal("tool_choice 键不存在")
	}
	choiceMap, ok := got.(map[string]any)
	if !ok {
		t.Fatal("tool_choice 类型不是 map[string]any")
	}
	if choiceMap["type"] != "auto" {
		t.Errorf("tool_choice.type = %v, 期望 auto", choiceMap["type"])
	}
}

func TestWithResponseFormat(t *testing.T) {
	cfg := &RequestConfig{}
	format := map[string]any{"type": "json_object"}
	WithResponseFormat(format)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	got, ok := cfg.ProviderExtra[provider.ProviderExtraKeyResponseFormat]
	if !ok {
		t.Fatal("response_format 键不存在")
	}
	formatMap, ok := got.(map[string]any)
	if !ok {
		t.Fatal("response_format 类型不是 map[string]any")
	}
	if formatMap["type"] != "json_object" {
		t.Errorf("response_format.type = %v, 期望 json_object", formatMap["type"])
	}
}

func TestWithExtra(t *testing.T) {
	cfg := &RequestConfig{}
	WithExtra("custom_key", "custom_value")(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra["custom_key"] != "custom_value" {
		t.Errorf("custom_key = %v, 期望 custom_value", cfg.ProviderExtra["custom_key"])
	}
}

func TestWithExtra_ExistingMap(t *testing.T) {
	cfg := &RequestConfig{
		ProviderExtra: map[string]any{"existing": true},
	}
	WithExtra("new_key", "new_value")(cfg)

	if cfg.ProviderExtra["existing"] != true {
		t.Errorf("existing = %v, 期望 true", cfg.ProviderExtra["existing"])
	}
	if cfg.ProviderExtra["new_key"] != "new_value" {
		t.Errorf("new_key = %v, 期望 new_value", cfg.ProviderExtra["new_key"])
	}
}

func TestMultipleOptions(t *testing.T) {
	cfg := &RequestConfig{}

	opts := []RequestOption{
		WithTopK(30.0),
		WithFrequencyPenalty(0.3),
		WithPresencePenalty(0.6),
		WithSeed(123),
		WithExtra("custom", "value"),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyTopK] != 30.0 {
		t.Errorf("TopK = %v, 期望 30.0", cfg.ProviderExtra[provider.ProviderExtraKeyTopK])
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyFrequencyPenalty] != 0.3 {
		t.Errorf("FrequencyPenalty = %v, 期望 0.3", cfg.ProviderExtra[provider.ProviderExtraKeyFrequencyPenalty])
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyPresencePenalty] != 0.6 {
		t.Errorf("PresencePenalty = %v, 期望 0.6", cfg.ProviderExtra[provider.ProviderExtraKeyPresencePenalty])
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeySeed] != int64(123) {
		t.Errorf("Seed = %v, 期望 123", cfg.ProviderExtra[provider.ProviderExtraKeySeed])
	}
	if cfg.ProviderExtra["custom"] != "value" {
		t.Errorf("custom = %v, 期望 value", cfg.ProviderExtra["custom"])
	}
}

// TestWithUser 验证 WithUser 正确将用户标识写入 ProviderExtra。
func TestWithUser(t *testing.T) {
	cfg := &RequestConfig{}
	WithUser("user-001")(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyUser] != "user-001" {
		t.Errorf("User = %v, 期望 user-001", cfg.ProviderExtra[provider.ProviderExtraKeyUser])
	}
}

// TestWithStore 验证 WithStore 正确将存储策略写入 ProviderExtra。
func TestWithStore(t *testing.T) {
	cfg := &RequestConfig{}
	WithStore(true)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyStore] != true {
		t.Errorf("Store = %v, 期望 true", cfg.ProviderExtra[provider.ProviderExtraKeyStore])
	}
}

// TestWithModalities 验证 WithModalities 正确将输出模态写入 ProviderExtra。
func TestWithModalities(t *testing.T) {
	cfg := &RequestConfig{}
	modalities := []string{"text", "image"}
	WithModalities(modalities)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	got, ok := cfg.ProviderExtra[provider.ProviderExtraKeyModalities]
	if !ok {
		t.Fatal("modalities 键不存在")
	}
	slice, ok := got.([]string)
	if !ok {
		t.Fatal("modalities 类型不是 []string")
	}
	if len(slice) != 2 || slice[0] != "text" || slice[1] != "image" {
		t.Errorf("Modalities = %v, 期望 [text image]", slice)
	}
}

// TestWithPrediction 验证 WithPrediction 正确将预测内容写入 ProviderExtra。
func TestWithPrediction(t *testing.T) {
	cfg := &RequestConfig{}
	prediction := map[string]any{"type": "content", "content": "expected"}
	WithPrediction(prediction)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	got, ok := cfg.ProviderExtra[provider.ProviderExtraKeyPrediction]
	if !ok {
		t.Fatal("prediction 键不存在")
	}
	predMap, ok := got.(map[string]any)
	if !ok {
		t.Fatal("prediction 类型不是 map[string]any")
	}
	if predMap["type"] != "content" {
		t.Errorf("prediction.type = %v, 期望 content", predMap["type"])
	}
}

// TestWithParallelToolCalls 验证 WithParallelToolCalls 正确将并行工具调用标志写入 ProviderExtra。
func TestWithParallelToolCalls(t *testing.T) {
	cfg := &RequestConfig{}
	WithParallelToolCalls(true)(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyParallelToolCalls] != true {
		t.Errorf("ParallelToolCalls = %v, 期望 true", cfg.ProviderExtra[provider.ProviderExtraKeyParallelToolCalls])
	}
}

// TestWithTruncation 验证 WithTruncation 正确将消息截断策略写入 ProviderExtra。
func TestWithTruncation(t *testing.T) {
	cfg := &RequestConfig{}
	WithTruncation("auto")(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyTruncation] != "auto" {
		t.Errorf("Truncation = %v, 期望 auto", cfg.ProviderExtra[provider.ProviderExtraKeyTruncation])
	}
}

// TestWithPreviousResponseID 验证 WithPreviousResponseID 正确将前置响应 ID 写入 ProviderExtra。
func TestWithPreviousResponseID(t *testing.T) {
	cfg := &RequestConfig{}
	WithPreviousResponseID("resp_abc123")(cfg)

	if cfg.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if cfg.ProviderExtra[provider.ProviderExtraKeyPreviousResponseID] != "resp_abc123" {
		t.Errorf("PreviousResponseID = %v, 期望 resp_abc123", cfg.ProviderExtra[provider.ProviderExtraKeyPreviousResponseID])
	}
}

// TestMultipleOptions_ThroughConfigToProvider 验证多个 option 经过 configToProvider 后仍正确透传。
func TestMultipleOptions_ThroughConfigToProvider(t *testing.T) {
	cfg := &RequestConfig{
		Model:     "test-model",
		MaxTokens: 1024,
	}
	WithTopK(25.0)(cfg)
	WithSeed(99)(cfg)

	result := configToProvider(cfg)
	if result == nil {
		t.Fatal("期望非 nil 结果")
	}
	if result.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if result.ProviderExtra[provider.ProviderExtraKeyTopK] != 25.0 {
		t.Errorf("TopK = %v, 期望 25.0", result.ProviderExtra[provider.ProviderExtraKeyTopK])
	}
	if result.ProviderExtra[provider.ProviderExtraKeySeed] != int64(99) {
		t.Errorf("Seed = %v, 期望 99", result.ProviderExtra[provider.ProviderExtraKeySeed])
	}
}

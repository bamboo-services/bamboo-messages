package bamboo

import (
	"testing"
)

func TestWithToolChoice(t *testing.T) {
	cfg := &RequestConfig{}
	WithToolChoice("auto")(cfg)

	if cfg.ToolChoice != "auto" {
		t.Errorf("ToolChoice = %q, 期望 auto", cfg.ToolChoice)
	}
}

func TestWithResponseFormat(t *testing.T) {
	cfg := &RequestConfig{}
	WithResponseFormat("json_object")(cfg)

	if cfg.ResponseFormat != "json_object" {
		t.Errorf("ResponseFormat = %q, 期望 json_object", cfg.ResponseFormat)
	}
}

func TestWithUserID(t *testing.T) {
	cfg := &RequestConfig{}
	WithUserID("user-001")(cfg)

	if cfg.UserID != "user-001" {
		t.Errorf("UserID = %q, 期望 user-001", cfg.UserID)
	}
}

func TestWithParallelToolCalls(t *testing.T) {
	cfg := &RequestConfig{}
	WithParallelToolCalls(true)(cfg)

	if cfg.ParallelToolCalls != true {
		t.Errorf("ParallelToolCalls = %v, 期望 true", cfg.ParallelToolCalls)
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
		WithToolChoice("auto"),
		WithResponseFormat("json_object"),
		WithUserID("user-123"),
		WithParallelToolCalls(true),
		WithExtra("custom", "value"),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.ToolChoice != "auto" {
		t.Errorf("ToolChoice = %q, 期望 auto", cfg.ToolChoice)
	}
	if cfg.ResponseFormat != "json_object" {
		t.Errorf("ResponseFormat = %q, 期望 json_object", cfg.ResponseFormat)
	}
	if cfg.UserID != "user-123" {
		t.Errorf("UserID = %q, 期望 user-123", cfg.UserID)
	}
	if cfg.ParallelToolCalls != true {
		t.Errorf("ParallelToolCalls = %v, 期望 true", cfg.ParallelToolCalls)
	}
	if cfg.ProviderExtra["custom"] != "value" {
		t.Errorf("custom = %v, 期望 value", cfg.ProviderExtra["custom"])
	}
}

// TestMultipleOptions_ThroughConfigToProvider 验证多个 option 经过 configToProvider 后仍正确透传。
func TestMultipleOptions_ThroughConfigToProvider(t *testing.T) {
	cfg := &RequestConfig{
		Model:     "test-model",
		MaxTokens: 1024,
	}
	WithToolChoice("required")(cfg)
	WithUserID("user-456")(cfg)
	WithExtra("top_k", 25.0)(cfg)

	result := configToProvider(cfg)
	if result == nil {
		t.Fatal("期望非 nil 结果")
	}
	if result.ToolChoice != "required" {
		t.Errorf("ToolChoice = %q, 期望 required", result.ToolChoice)
	}
	if result.UserID != "user-456" {
		t.Errorf("UserID = %q, 期望 user-456", result.UserID)
	}
	if result.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if result.ProviderExtra["top_k"] != 25.0 {
		t.Errorf("top_k = %v, 期望 25.0", result.ProviderExtra["top_k"])
	}
}

package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// float64Ptr 返回 float64 指针，避免依赖 bamboo 包的 PtrFloat64。
func float64Ptr(v float64) *float64 { return &v }

// ==============================
// buildParams 测试
// ==============================

func TestBuildParams_NilConfig(t *testing.T) {
	p := NewProvider("test-api-key")
	params := p.buildParams("system prompt", nil, nil)

	// nil config 不 panic，内部替换为空 ChatConfig
	if params.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0", params.MaxTokens)
	}
	if params.Model != "" {
		t.Errorf("Model = %q, want empty", params.Model)
	}
	// systemPrompt 应正常设置（字符串格式）
	sysStr, ok := params.System.(string)
	if !ok || sysStr != "system prompt" {
		t.Errorf("System = %#v, want string 'system prompt'", params.System)
	}
}

func TestBuildParams_EmptyConfig(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{}
	params := p.buildParams("", nil, config)

	if params.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0", params.MaxTokens)
	}
	if params.Model != "" {
		t.Errorf("Model = %q, want empty", params.Model)
	}
	if params.Temperature != nil {
		t.Error("Temperature should be nil for empty config")
	}
	if params.TopP != nil {
		t.Error("TopP should be nil for empty config")
	}
	if params.System != nil {
		t.Errorf("System should be nil, got %v", params.System)
	}
}

func TestBuildParams_SystemPrompt(t *testing.T) {
	p := NewProvider("test-api-key")
	params := p.buildParams("system instruction", nil, nil)

	sysStr, ok := params.System.(string)
	if !ok {
		t.Fatalf("System type = %T, want string", params.System)
	}
	if sysStr != "system instruction" {
		t.Errorf("System = %q, want %q", sysStr, "system instruction")
	}
}

func TestBuildParams_Temperature(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		Temperature: float64Ptr(0.7),
	}
	params := p.buildParams("", nil, config)

	if params.Temperature == nil {
		t.Fatal("expected Temperature to be non-nil")
	}
	if *params.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", *params.Temperature)
	}
}

func TestBuildParams_TopP(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		TopP: float64Ptr(0.95),
	}
	params := p.buildParams("", nil, config)

	if params.TopP == nil {
		t.Fatal("expected TopP to be non-nil")
	}
	if *params.TopP != 0.95 {
		t.Errorf("TopP = %v, want 0.95", *params.TopP)
	}
}

func TestBuildParams_StopSequences(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		Stop: []string{"stop1", "stop2"},
	}
	params := p.buildParams("", nil, config)

	if len(params.StopSequences) != 2 {
		t.Errorf("StopSequences length = %d, want 2", len(params.StopSequences))
	}
	if params.StopSequences[0] != "stop1" || params.StopSequences[1] != "stop2" {
		t.Errorf("StopSequences = %v, want [stop1, stop2]", params.StopSequences)
	}
}

func TestBuildParams_Tools(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		Tools: []provider.Tool{
			{
				Type: "function",
				Function: provider.FunctionDef{
					Name:        "test_tool",
					Description: "A test tool",
					Parameters: map[string]any{
						"type": "object",
					},
				},
			},
		},
	}
	params := p.buildParams("", nil, config)

	if params.Tools == nil {
		t.Fatal("expected Tools to be non-nil")
	}
	if len(params.Tools) != 1 {
		t.Fatalf("Tools length = %d, want 1", len(params.Tools))
	}
	if params.Tools[0]["name"] != "test_tool" {
		t.Errorf("Tool name = %v, want 'test_tool'", params.Tools[0]["name"])
	}
	if params.Tools[0]["description"] != "A test tool" {
		t.Errorf("Tool description = %v, want 'A test tool'", params.Tools[0]["description"])
	}
}

func TestBuildParams_ThinkingConfig(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=high")
	}
	if params.Thinking.Type != "adaptive" {
		t.Errorf("Thinking.Type = %q, want 'adaptive'", params.Thinking.Type)
	}
}

func TestBuildParams_TopK(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"top_k": 40.0},
	}
	params := p.buildParams("", nil, config)

	if params.TopK == nil {
		t.Fatal("expected TopK to be non-nil")
	}
	if *params.TopK != 40 {
		t.Errorf("TopK = %d, want 40", *params.TopK)
	}
}

func TestBuildParams_TopKFromCodecInt64(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"top_k": int64(40)},
	}
	params := p.buildParams("", nil, config)

	if params.TopK == nil {
		t.Fatal("expected TopK to be non-nil for int64 ProviderExtra")
	}
	if *params.TopK != 40 {
		t.Errorf("TopK = %d, want 40", *params.TopK)
	}
}

func TestBuildParams_CacheNormalizationRemovesDynamicMetadataOnlyWhenEnabled(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		UserID:         "dynamic-user",
		Metadata:       map[string]string{"trace_id": "req-123"},
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
		ProviderExtra: map[string]any{
			"thinking":                  []byte(`{"type":"enabled","budget_tokens":1024}`),
			"anthropic_cache_normalize": true,
		},
	}
	params := p.buildParams("system prompt", []provider.Message{{Role: provider.RoleUser, Content: "hello"}}, config)

	// cache normalize 启用时 metadata 不发送
	if params.Metadata != nil {
		t.Fatalf("metadata should be nil when cache normalization is enabled, got %#v", params.Metadata)
	}
	// 非 Legacy 模式下，ProviderExtra["thinking"] 原始配置应被保留（enabled + budget_tokens）
	if params.Thinking == nil {
		t.Fatal("Thinking should be non-nil")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled' (preserved from ProviderExtra)", params.Thinking.Type)
	}
	if params.Thinking.BudgetTokens != 1024 {
		t.Errorf("Thinking.BudgetTokens = %d, want 1024", params.Thinking.BudgetTokens)
	}
	// system prompt 应保持字符串格式
	sysStr, ok := params.System.(string)
	if !ok || sysStr != "system prompt" {
		t.Fatalf("system prompt changed: %#v", params.System)
	}
	if len(params.Messages) != 1 {
		t.Fatalf("messages changed: %#v", params.Messages)
	}
}

func TestBuildParams_CacheNormalizationDefaultOff(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		UserID:   "dynamic-user",
		Metadata: map[string]string{"trace_id": "req-123"},
		ProviderExtra: map[string]any{
			"thinking": []byte(`{"type":"enabled","budget_tokens":1024}`),
		},
	}
	params := p.buildParams("", nil, config)

	if params.Metadata == nil {
		t.Fatal("metadata should be preserved by default")
	}
	if params.Metadata.UserID != "dynamic-user" {
		t.Errorf("metadata.user_id = %q, want 'dynamic-user'", params.Metadata.UserID)
	}
}

func TestBuildParams_ToolChoiceAuto(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{ToolChoice: "auto"}
	params := p.buildParams("", nil, config)

	tc, ok := params.ToolChoice.(map[string]any)
	if !ok {
		t.Fatalf("expected ToolChoice to be map[string]any, got %T", params.ToolChoice)
	}
	if tc["type"] != "auto" {
		t.Errorf("ToolChoice type = %v, want 'auto'", tc["type"])
	}
}

func TestBuildParams_ToolChoiceForced(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{ToolChoice: "forced"}
	params := p.buildParams("", nil, config)

	tc, ok := params.ToolChoice.(map[string]any)
	if !ok {
		t.Fatalf("expected ToolChoice to be map[string]any, got %T", params.ToolChoice)
	}
	if tc["type"] != "any" {
		t.Errorf("ToolChoice type = %v, want 'any' for forced", tc["type"])
	}
}

func TestBuildParams_ToolChoiceNone(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{ToolChoice: "none"}
	params := p.buildParams("", nil, config)

	tc, ok := params.ToolChoice.(map[string]any)
	if !ok {
		t.Fatalf("expected ToolChoice to be map[string]any, got %T", params.ToolChoice)
	}
	if tc["type"] != "none" {
		t.Errorf("ToolChoice type = %v, want 'none'", tc["type"])
	}
}

func TestBuildParams_UserIDAndMetadata(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		UserID: "user-123",
	}
	params := p.buildParams("", nil, config)

	if params.Metadata == nil {
		t.Fatal("expected Metadata to be non-nil")
	}
	if params.Metadata.UserID != "user-123" {
		t.Errorf("Metadata.UserID = %q, want 'user-123'", params.Metadata.UserID)
	}
}

// TestBuildParams_SystemCacheControl 验证带 cache_control 的 system 块数组格式。
func TestBuildParams_SystemCacheControl(t *testing.T) {
	p := NewProvider("test-api-key")
	cc := provider.NewEphemeralCacheControl(provider.CacheTTL1h)
	config := &provider.ChatConfig{
		SystemCacheControl: cc,
	}
	params := p.buildParams("You are a helpful assistant.", nil, config)

	sysBlocks, ok := params.System.([]map[string]any)
	if !ok {
		t.Fatalf("expected System to be []map[string]any, got %T", params.System)
	}
	if len(sysBlocks) != 1 {
		t.Fatalf("System blocks = %d, want 1", len(sysBlocks))
	}
	ccField, ok := sysBlocks[0]["cache_control"].(map[string]any)
	if !ok {
		t.Fatalf("expected cache_control to be map[string]any, got %T", sysBlocks[0]["cache_control"])
	}
	if ccField["type"] != "ephemeral" {
		t.Errorf("CacheControl.Type = %v, want 'ephemeral'", ccField["type"])
	}
}

// TestBuildParams_StreamFlag 验证 stream 字段默认不设置（由 chat.go 设置）。
func TestBuildParams_StreamFlag(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	params := p.buildParams("", nil, config)

	if params.Stream {
		t.Error("Stream should be false by default (set by chat.go)")
	}
}

// TestBuildParams_Marshalable 验证 buildParams 结果可被 json.Marshal 序列化。
func TestBuildParams_Marshalable(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      1024,
		Temperature:    float64Ptr(0.7),
		TopP:           float64Ptr(0.95),
		Stop:           []string{"END"},
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
		ToolChoice:     "auto",
		UserID:         "user-1",
	}
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}
	params := p.buildParams("Be helpful.", messages, config)

	if _, err := json.Marshal(params); err != nil {
		t.Fatalf("buildParams result not marshalable: %v", err)
	}
}

// ==============================
// Legacy 兼容模式测试
// ==============================

func TestBuildParams_LegacyCompat_ThinkingAdaptiveFromEffort(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=high in legacy mode")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled' (legacy mode synthesizes enabled)", params.Thinking.Type)
	}
}

func TestBuildParams_LegacyCompat_ThinkingFromExtraAdaptive(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
		ProviderExtra: map[string]any{
			"thinking": json.RawMessage(`{"type":"adaptive"}`),
		},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled' (adaptive→enabled in legacy mode)", params.Thinking.Type)
	}
}

func TestBuildParams_LegacyCompat_ThinkingFromExtraEnabledWithBudget(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "medium"},
		ProviderExtra: map[string]any{
			"thinking": json.RawMessage(`{"type":"enabled","budget_tokens":10000}`),
		},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled'", params.Thinking.Type)
	}
	if params.Thinking.BudgetTokens != 10000 {
		t.Errorf("Thinking.BudgetTokens = %d, want 10000 (preserved from ProviderExtra)", params.Thinking.BudgetTokens)
	}
}

func TestBuildParams_NonLegacy_ThinkingFromExtraEnabledWithBudget(t *testing.T) {
	p := NewProvider("test-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "medium"},
		ProviderExtra: map[string]any{
			"thinking": json.RawMessage(`{"type":"enabled","budget_tokens":10000}`),
		},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled' (preserved in non-legacy mode)", params.Thinking.Type)
	}
	if params.Thinking.BudgetTokens != 10000 {
		t.Errorf("Thinking.BudgetTokens = %d, want 10000", params.Thinking.BudgetTokens)
	}
}

func TestBuildParams_NonLegacy_ThinkingFromExtraAdaptive(t *testing.T) {
	p := NewProvider("test-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
		ProviderExtra: map[string]any{
			"thinking": json.RawMessage(`{"type":"adaptive"}`),
		},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil")
	}
	if params.Thinking.Type != "adaptive" {
		t.Errorf("Thinking.Type = %q, want 'adaptive' (preserved in non-legacy mode)", params.Thinking.Type)
	}
}

func TestBuildParams_LegacyCompat_ThinkingNoneDisabled(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "none"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=none")
	}
	if params.Thinking.Type != "disabled" {
		t.Errorf("Thinking.Type = %q, want 'disabled'", params.Thinking.Type)
	}
}

func TestBuildParams_NonLegacy_ThinkingNoneDisabled(t *testing.T) {
	p := NewProvider("test-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "none"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=none")
	}
	if params.Thinking.Type != "disabled" {
		t.Errorf("Thinking.Type = %q, want 'disabled'", params.Thinking.Type)
	}
}

func TestBuildParams_LegacyCompat_NoThinkingConfig(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{}
	params := p.buildParams("", nil, config)

	if params.Thinking != nil {
		t.Errorf("Thinking should be nil when no ThinkingConfig, got %+v", params.Thinking)
	}
}

func TestBuildParams_LegacyCompat_ThinkingHighWithMaxTokens40000(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		MaxTokens:      40000,
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=high in legacy mode")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled'", params.Thinking.Type)
	}
	if params.Thinking.BudgetTokens != 32000 {
		t.Errorf("Thinking.BudgetTokens = %d, want 32000", params.Thinking.BudgetTokens)
	}
}

func TestBuildParams_LegacyCompat_ThinkingXHighWithMaxTokens50000(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		MaxTokens:      50000,
		ThinkingConfig: &provider.ThinkingConfig{Effort: "xhigh"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=xhigh in legacy mode")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled'", params.Thinking.Type)
	}
	if params.Thinking.BudgetTokens != 48000 {
		t.Errorf("Thinking.BudgetTokens = %d, want 48000", params.Thinking.BudgetTokens)
	}
}

func TestBuildParams_LegacyCompat_ThinkingMaxWithMaxTokens65000(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		MaxTokens:      65000,
		ThinkingConfig: &provider.ThinkingConfig{Effort: "max"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=max in legacy mode")
	}
	if params.Thinking.Type != "enabled" {
		t.Errorf("Thinking.Type = %q, want 'enabled'", params.Thinking.Type)
	}
	if params.Thinking.BudgetTokens != 64000 {
		t.Errorf("Thinking.BudgetTokens = %d, want 64000", params.Thinking.BudgetTokens)
	}
}

func TestBuildParams_LegacyCompat_ThinkingWithMaxTokens500(t *testing.T) {
	p := NewProviderWithOptions(WithAPIKey("test-key"), WithLegacyCompat())
	config := &provider.ChatConfig{
		MaxTokens:      500,
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking != nil {
		t.Errorf("Thinking should be nil when max_tokens=500 (<1025), got %+v", params.Thinking)
	}
}

func TestBuildParams_ThinkingConfig_DisplaySet(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high", Display: "omitted"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=high")
	}
	if params.Thinking.Type != "adaptive" {
		t.Errorf("Thinking.Type = %q, want 'adaptive'", params.Thinking.Type)
	}
	if params.Thinking.Display != "omitted" {
		t.Errorf("Thinking.Display = %q, want 'omitted'", params.Thinking.Display)
	}
}

func TestBuildParams_ThinkingConfig_DisplayEmpty(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil for effort=high")
	}
	if params.Thinking.Type != "adaptive" {
		t.Errorf("Thinking.Type = %q, want 'adaptive'", params.Thinking.Type)
	}
	if params.Thinking.Display != "" {
		t.Errorf("Thinking.Display = %q, want empty", params.Thinking.Display)
	}
}

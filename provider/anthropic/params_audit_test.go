package anthropic

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestAudit_ResponseFormat_Dropped 验证 Anthropic 适配器对 ResponseFormat 的 best-effort 适配。
//
// Anthropic 不原生支持 ResponseFormat，当设置为 "json_object" 时，
// buildParams 会注入系统提示指令 "Respond with valid JSON only." 作为替代方案。
func TestAudit_ResponseFormat_Dropped(t *testing.T) {
	p := NewProvider("test-key")

	config := &provider.ChatConfig{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      1024,
		ResponseFormat: "json_object",
	}

	params := p.buildParams("original system", nil, config)

	// ResponseFormat=json_object 时应注入 JSON 指令到 system prompt
	sysStr, ok := params.System.(string)
	if !ok {
		t.Fatalf("expected System to be string, got %T", params.System)
	}
	if sysStr == "original system" {
		t.Error("expected system prompt to contain JSON instruction, got unchanged prompt")
	}
}

// TestAudit_ParallelToolCalls_Dropped 验证 ParallelToolCalls 在 Anthropic 适配器中被忽略。
//
// Anthropic API 不支持 parallel_tool_calls 参数，buildParams 仅记录 debug 日志。
func TestAudit_ParallelToolCalls_Dropped(t *testing.T) {
	p := NewProvider("test-key")

	config := &provider.ChatConfig{
		Model:             "claude-sonnet-4-20250514",
		MaxTokens:         1024,
		ParallelToolCalls: true,
	}

	params := p.buildParams("", nil, config)

	// ParallelToolCalls 不影响 params（Anthropic 不支持此字段）
	_ = params // 不 panic 即通过
}

// TestAudit_SystemCacheControl_Mapping 验证 SystemCacheControl 映射到 Anthropic system block。
func TestAudit_SystemCacheControl_Mapping(t *testing.T) {
	p := NewProvider("test-key")

	cc := provider.NewEphemeralCacheControl(provider.CacheTTL1h)
	config := &provider.ChatConfig{
		Model:              "claude-sonnet-4-20250514",
		MaxTokens:          1024,
		SystemCacheControl: cc,
	}

	params := p.buildParams("You are a helpful assistant.", nil, config)

	sysBlocks, ok := params.System.([]map[string]any)
	if !ok {
		t.Fatalf("System type = %T, want []map[string]any", params.System)
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

// TestAudit_Anthropic_ServiceTierAndSpeed 验证 Anthropic 适配器对 ServiceTier 与 speed (Fast mode) 的映射。
func TestAudit_Anthropic_ServiceTierAndSpeed(t *testing.T) {
	p := NewProvider("test-key")

	// 场景 1：ServiceTier = "priority" → Speed="fast", ServiceTier="auto"
	cfg1 := &provider.ChatConfig{
		Model:       "claude-opus-5-5",
		ServiceTier: "priority",
	}
	params1 := p.buildParams("", nil, cfg1)
	if params1.Speed != "fast" {
		t.Errorf("expected Speed 'fast', got %q", params1.Speed)
	}
	if params1.ServiceTier != "auto" {
		t.Errorf("expected ServiceTier 'auto', got %q", params1.ServiceTier)
	}

	// 场景 2：ServiceTier = "fast" → Speed="fast"
	cfg2 := &provider.ChatConfig{
		Model:       "claude-opus-5-5",
		ServiceTier: "fast",
	}
	params2 := p.buildParams("", nil, cfg2)
	if params2.Speed != "fast" {
		t.Errorf("expected Speed 'fast', got %q", params2.Speed)
	}

	// 场景 3：通过 ProviderExtra 显式传 speed 和 service_tier
	cfg3 := &provider.ChatConfig{
		Model: "claude-opus-5-5",
		ProviderExtra: map[string]any{
			"speed":        "fast",
			"service_tier": "standard_only",
		},
	}
	params3 := p.buildParams("", nil, cfg3)
	if params3.Speed != "fast" {
		t.Errorf("expected Speed 'fast', got %q", params3.Speed)
	}
	if params3.ServiceTier != "standard_only" {
		t.Errorf("expected ServiceTier 'standard_only', got %q", params3.ServiceTier)
	}

	// 场景 4：跨协议安全：OpenAI/Gemini 的 "flex" 传入时应在 Anthropic 中忽略（避免上游 400 拒绝）
	cfg4 := &provider.ChatConfig{
		Model:       "claude-opus-5-5",
		ServiceTier: "flex",
	}
	params4 := p.buildParams("", nil, cfg4)
	if params4.ServiceTier != "" {
		t.Errorf("expected ServiceTier to be empty for 'flex', got %q", params4.ServiceTier)
	}

	// 场景 5：跨协议归一化：OpenAI 的 "default" 传入时归一化为 "standard_only"
	cfg5 := &provider.ChatConfig{
		Model:       "claude-opus-5-5",
		ServiceTier: "default",
	}
	params5 := p.buildParams("", nil, cfg5)
	if params5.ServiceTier != "standard_only" {
		t.Errorf("expected ServiceTier 'standard_only' (normalized from default), got %q", params5.ServiceTier)
	}
}

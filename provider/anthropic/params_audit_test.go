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

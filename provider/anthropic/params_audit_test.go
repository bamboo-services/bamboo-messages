package anthropic

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestAudit_ResponseFormat_Dropped 验证 Anthropic 适配器丢弃 ResponseFormat 字段。
//
// Severity: P1
// File:Line: provider/anthropic/params.go:19-92 (buildParams 缺失 ResponseFormat 处理)
// Issue: buildParams 不处理 config.ResponseFormat。Anthropic API 支持 JSON mode，
//        但适配器未将 ResponseFormat 映射到 Anthropic 的 response_format 参数。
// Affected: Any→Anthropic with ResponseFormat="json_object"
func TestAudit_ResponseFormat_Dropped(t *testing.T) {
	p := NewProvider("test-key")

	config := &provider.ChatConfig{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      1024,
		ResponseFormat: "json_object",
	}

	params := p.buildParams("", nil, config)

	// Anthropic SDK BetaMessageNewParams 没有直接的 ResponseFormat 字段
	// 但 Anthropic API 支持通过 tools 强制 JSON 输出，或通过 system prompt 引导
	// 当前 buildParams 完全忽略 ResponseFormat —— 参数被静默丢弃
	t.Logf("P1 NOTE: ResponseFormat=%q is dropped by Anthropic adapter buildParams (no mapping exists)", config.ResponseFormat)
	_ = params
}

// TestAudit_ParallelToolCalls_Dropped 验证 ParallelToolCalls 在 Anthropic 适配器中被丢弃。
//
// Severity: P2
// File:Line: provider/anthropic/params.go (buildParams 缺失 ParallelToolCalls 处理)
// Issue: Anthropic API 不支持 parallel_tool_calls 参数，buildParams 不处理此字段。
// Affected: Any→Anthropic with ParallelToolCalls=true
func TestAudit_ParallelToolCalls_Dropped(t *testing.T) {
	p := NewProvider("test-key")

	config := &provider.ChatConfig{
		Model:             "claude-sonnet-4-20250514",
		MaxTokens:         1024,
		ParallelToolCalls: true,
	}

	params := p.buildParams("", nil, config)

	// Anthropic 没有 parallel_tool_calls 字段，参数被丢弃是预期行为
	t.Logf("P2 NOTE: ParallelToolCalls=%v is dropped by Anthropic adapter (not supported by Anthropic API)", config.ParallelToolCalls)
	_ = params
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

	if len(params.System) != 1 {
		t.Fatalf("System blocks = %d, want 1", len(params.System))
	}

	sysBlock := params.System[0]
	if sysBlock.CacheControl.Type != "ephemeral" {
		t.Errorf("CacheControl.Type = %q, want 'ephemeral'", sysBlock.CacheControl.Type)
	}
}

package completions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// newLegacyProvider 创建 Legacy 兼容模式的 Provider 实例。
func newLegacyProvider(t *testing.T) *CompletionsProvider {
	t.Helper()
	p := NewCompletionsProviderWithOptions(WithAPIKey("test"), WithLegacyCompat())
	return p
}

// newDefaultProvider 创建默认（非 Legacy）模式的 Provider 实例。
func newDefaultProvider(t *testing.T) *CompletionsProvider {
	t.Helper()
	return NewCompletionsProvider("test")
}

// TestBuildParams_LegacyMaxTokens 验证 Legacy 模式使用旧字段名 max_tokens。
func TestBuildParams_LegacyMaxTokens(t *testing.T) {
	p := newLegacyProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{MaxTokens: 4096})

	v, ok := params["max_tokens"]
	if !ok {
		t.Fatal("Legacy: max_tokens should be set")
	}
	if v.(int64) != 4096 {
		t.Errorf("Legacy: max_tokens = %v, want 4096", v)
	}
	if _, exists := params["max_completion_tokens"]; exists {
		t.Error("Legacy: max_completion_tokens should NOT be set")
	}
}

// TestBuildParams_DefaultMaxCompletionTokens 验证默认模式使用新字段名 max_completion_tokens。
func TestBuildParams_DefaultMaxCompletionTokens(t *testing.T) {
	p := newDefaultProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{MaxTokens: 4096})

	v, ok := params["max_completion_tokens"]
	if !ok {
		t.Fatal("Default: max_completion_tokens should be set")
	}
	if v.(int64) != 4096 {
		t.Errorf("Default: max_completion_tokens = %v, want 4096", v)
	}
	if _, exists := params["max_tokens"]; exists {
		t.Error("Default: max_tokens should NOT be set")
	}
}

// TestBuildParams_LegacyNoParallelToolCalls 验证 Legacy 无工具时不发送 ParallelToolCalls。
func TestBuildParams_LegacyNoParallelToolCalls(t *testing.T) {
	p := newLegacyProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{})

	if _, exists := params["parallel_tool_calls"]; exists {
		t.Error("Legacy (no tools): parallel_tool_calls should NOT be set")
	}
}

// TestBuildParams_LegacyParallelToolCallsWithTools 验证 Legacy 有工具时发送 ParallelToolCalls。
func TestBuildParams_LegacyParallelToolCallsWithTools(t *testing.T) {
	p := newLegacyProvider(t)
	config := &provider.ChatConfig{
		Tools: []provider.Tool{
			{Type: "function", Function: provider.FunctionDef{Name: "test_tool"}},
		},
		ParallelToolCalls: true,
	}
	params := p.buildParams("", nil, config)

	v, ok := params["parallel_tool_calls"]
	if !ok {
		t.Fatal("Legacy (with tools): parallel_tool_calls should be set")
	}
	if v != true {
		t.Errorf("Legacy (with tools): parallel_tool_calls = %v, want true", v)
	}
}

// TestBuildParams_LegacyParallelToolCallsOmittedWhenFalse 验证 ParallelToolCalls=false 时不发送。
func TestBuildParams_LegacyParallelToolCallsOmittedWhenFalse(t *testing.T) {
	p := newLegacyProvider(t)
	config := &provider.ChatConfig{
		Tools: []provider.Tool{
			{Type: "function", Function: provider.FunctionDef{Name: "test_tool"}},
		},
		ParallelToolCalls: false,
	}
	params := p.buildParams("", nil, config)

	if _, exists := params["parallel_tool_calls"]; exists {
		t.Error("Legacy (with tools, ParallelToolCalls=false): parallel_tool_calls should NOT be set")
	}
}

// TestBuildStreamOptions_LegacyOmitted 验证 Legacy 模式下 buildStreamOptions 返回 nil。
func TestBuildStreamOptions_LegacyOmitted(t *testing.T) {
	p := newLegacyProvider(t)
	opts := p.buildStreamOptions()

	if opts != nil {
		t.Errorf("Legacy: buildStreamOptions should return nil, got %v", opts)
	}
}

// TestBuildStreamOptions_DefaultSet 验证默认模式下 buildStreamOptions 设置 include_usage=true。
func TestBuildStreamOptions_DefaultSet(t *testing.T) {
	p := newDefaultProvider(t)
	opts := p.buildStreamOptions()

	if opts == nil {
		t.Fatal("Default: buildStreamOptions should not be nil")
	}
	v, ok := opts["include_usage"]
	if !ok {
		t.Fatal("Default: include_usage should be set")
	}
	if v != true {
		t.Errorf("Default: include_usage = %v, want true", v)
	}
}

// TestBuildParams_LegacyReasoningEffort 验证 Legacy 模式正常映射 reasoning_effort。
func TestBuildParams_LegacyReasoningEffort(t *testing.T) {
	p := newLegacyProvider(t)
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	v, ok := params["reasoning_effort"]
	if !ok {
		t.Fatal("Legacy: reasoning_effort should be set")
	}
	if v != "high" {
		t.Errorf("Legacy: reasoning_effort = %v, want %q", v, "high")
	}
}

// TestBuildParams_LegacyThinkingPassthrough 验证 Legacy 模式从 ProviderExtra 透传 thinking。
func TestBuildParams_LegacyThinkingPassthrough(t *testing.T) {
	p := newLegacyProvider(t)
	thinkingValue := map[string]any{"type": "enabled"}
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"thinking": thinkingValue},
	}
	params := p.buildParams("", nil, config)

	thinking, ok := params["thinking"]
	if !ok {
		t.Fatal("Legacy: expected 'thinking' in params")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("Legacy: thinking value type = %T, want map[string]any", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Errorf("Legacy: thinking.type = %v, want 'enabled'", thinkingMap["type"])
	}
}

// TestBuildParams_LegacyThinkingPassthroughAdaptive 验证 adaptive thinking 被归一化为 enabled。
func TestBuildParams_LegacyThinkingPassthroughAdaptive(t *testing.T) {
	p := newLegacyProvider(t)
	thinkingValue := map[string]any{"type": "adaptive"}
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"thinking": thinkingValue},
	}
	params := p.buildParams("", nil, config)

	thinking, ok := params["thinking"]
	if !ok {
		t.Fatal("Legacy: expected 'thinking' in params")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("Legacy: thinking type = %T, want map[string]any", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Errorf("Legacy: thinking.type = %v, want 'enabled' (normalized from adaptive)", thinkingMap["type"])
	}
}

// TestBuildParams_LegacyThinkingPassthroughAdaptiveJSONRawMessage 验证 json.RawMessage 输入也能正确归一化。
func TestBuildParams_LegacyThinkingPassthroughAdaptiveJSONRawMessage(t *testing.T) {
	p := newLegacyProvider(t)
	thinkingValue := json.RawMessage(`{"type":"adaptive"}`)
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"thinking": thinkingValue},
	}
	params := p.buildParams("", nil, config)

	thinking, ok := params["thinking"]
	if !ok {
		t.Fatal("Legacy: expected 'thinking' in params")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("Legacy: thinking type = %T, want map[string]any (json.RawMessage should be unwrapped)", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Errorf("Legacy: thinking.type = %v, want 'enabled'", thinkingMap["type"])
	}
}

// TestBuildParams_LegacyThinkingPassthroughDisabled 验证 disabled thinking 保持不变。
func TestBuildParams_LegacyThinkingPassthroughDisabled(t *testing.T) {
	p := newLegacyProvider(t)
	thinkingValue := map[string]any{"type": "disabled"}
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"thinking": thinkingValue},
	}
	params := p.buildParams("", nil, config)

	thinking, ok := params["thinking"]
	if !ok {
		t.Fatal("Legacy: expected 'thinking' in params")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("Legacy: thinking type = %T, want map[string]any", thinking)
	}
	if thinkingMap["type"] != "disabled" {
		t.Errorf("Legacy: thinking.type = %v, want 'disabled'", thinkingMap["type"])
	}
}

// TestBuildParams_LegacyThinkingPassthroughUnknownType 验证未知 type 值原样保留。
func TestBuildParams_LegacyThinkingPassthroughUnknownType(t *testing.T) {
	p := newLegacyProvider(t)
	thinkingValue := map[string]any{"type": "custom_type", "extra": "data"}
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"thinking": thinkingValue},
	}
	params := p.buildParams("", nil, config)

	thinking, ok := params["thinking"]
	if !ok {
		t.Fatal("Legacy: expected 'thinking' in params")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("Legacy: thinking type = %T, want map[string]any", thinking)
	}
	if thinkingMap["type"] != "custom_type" {
		t.Errorf("Legacy: thinking.type = %v, want 'custom_type'", thinkingMap["type"])
	}
	if thinkingMap["extra"] != "data" {
		t.Errorf("Legacy: thinking.extra = %v, want 'data'", thinkingMap["extra"])
	}
}

// TestBuildParams_LegacyThinkingFromEffort 验证从 Effort 合成 thinking 参数。
func TestBuildParams_LegacyThinkingFromEffort(t *testing.T) {
	tests := []struct {
		effort              string
		wantThinkingType    string
		wantReasoningEffort string
	}{
		{"none", "disabled", "none"},
		{"minimal", "enabled", "minimal"},
		{"low", "enabled", "low"},
		{"medium", "enabled", "medium"},
		{"high", "enabled", "high"},
		{"xhigh", "enabled", "xhigh"},
		{"max", "enabled", "max"},
	}

	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			p := newLegacyProvider(t)
			config := &provider.ChatConfig{
				ThinkingConfig: &provider.ThinkingConfig{Effort: tt.effort},
			}
			params := p.buildParams("", nil, config)

			// 验证 reasoning_effort 直接透传
			if v, ok := params["reasoning_effort"]; !ok || v != tt.wantReasoningEffort {
				t.Errorf("reasoning_effort = %v, want %q", v, tt.wantReasoningEffort)
			}

			// 验证 thinking 合成
			thinking, ok := params["thinking"]
			if !ok {
				t.Fatalf("expected 'thinking' in params for effort=%s", tt.effort)
			}
			thinkingMap, ok := thinking.(map[string]any)
			if !ok {
				t.Fatalf("thinking type = %T, want map[string]any", thinking)
			}
			if thinkingMap["type"] != tt.wantThinkingType {
				t.Errorf("thinking.type = %v, want %q", thinkingMap["type"], tt.wantThinkingType)
			}
		})
	}
}

// TestBuildParams_LegacyThinkingEffortOverriddenByRawThinking 验证原始 thinking JSON 优先于 Effort 合成。
func TestBuildParams_LegacyThinkingEffortOverriddenByRawThinking(t *testing.T) {
	p := newLegacyProvider(t)
	thinkingValue := map[string]any{"type": "enabled", "budget_tokens": int64(50000)}
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "low"},
		ProviderExtra:  map[string]any{"thinking": thinkingValue},
	}
	params := p.buildParams("", nil, config)

	thinking, ok := params["thinking"]
	if !ok {
		t.Fatal("Legacy: expected 'thinking' in params")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("Legacy: thinking type = %T, want map[string]any", thinking)
	}
	budget, hasBudget := thinkingMap["budget_tokens"].(int64)
	if !hasBudget {
		t.Error("Legacy: expected budget_tokens from raw thinking JSON to be preserved")
	} else if budget != 50000 {
		t.Errorf("Legacy: budget_tokens = %d, want 50000", budget)
	}
}

// TestBuildParams_LegacyThinkingPassthroughNil 验证无 thinking 时不设置 thinking 字段。
func TestBuildParams_LegacyThinkingPassthroughNil(t *testing.T) {
	p := newLegacyProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{})

	if _, ok := params["thinking"]; ok {
		t.Error("Legacy: 'thinking' should not be present when ProviderExtra is nil")
	}
}

// TestBuildParams_DefaultReasoningEffort 验证默认模式正常映射 reasoning_effort。
func TestBuildParams_DefaultReasoningEffort(t *testing.T) {
	p := newDefaultProvider(t)
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	v, ok := params["reasoning_effort"]
	if !ok {
		t.Fatal("Default: reasoning_effort should be set")
	}
	if v != "high" {
		t.Errorf("Default: reasoning_effort = %v, want %q", v, "high")
	}
}

// TestBuildParams_DefaultParallelToolCalls 验证默认模式仅在工具存在且显式 true 时发送 ParallelToolCalls。
func TestBuildParams_DefaultParallelToolCalls(t *testing.T) {
	p := newDefaultProvider(t)

	t.Run("no tools omitted", func(t *testing.T) {
		params := p.buildParams("", nil, &provider.ChatConfig{})
		if _, exists := params["parallel_tool_calls"]; exists {
			t.Error("Default (no tools): parallel_tool_calls should be omitted")
		}
	})

	t.Run("with tools false omitted", func(t *testing.T) {
		params := p.buildParams("", nil, &provider.ChatConfig{
			Tools: []provider.Tool{
				{Type: "function", Function: provider.FunctionDef{Name: "test_tool"}},
			},
			ParallelToolCalls: false,
		})
		if _, exists := params["parallel_tool_calls"]; exists {
			t.Error("Default (with tools, false): parallel_tool_calls should be omitted")
		}
	})

	t.Run("with tools true set", func(t *testing.T) {
		params := p.buildParams("", nil, &provider.ChatConfig{
			Tools: []provider.Tool{
				{Type: "function", Function: provider.FunctionDef{Name: "test_tool"}},
			},
			ParallelToolCalls: true,
		})
		v, ok := params["parallel_tool_calls"]
		if !ok {
			t.Fatal("Default (with tools, true): parallel_tool_calls should be set")
		}
		if v != true {
			t.Errorf("Default (with tools, true): parallel_tool_calls = %v, want true", v)
		}
	})
}

// TestMarshalJSON_LegacyFields 验证 JSON 序列化中 Legacy 使用 max_tokens 而非 max_completion_tokens。
func TestMarshalJSON_LegacyFields(t *testing.T) {
	p := newLegacyProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{MaxTokens: 4096})

	jsonData, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(jsonData)

	if !strings.Contains(jsonStr, "max_tokens") {
		t.Errorf("Legacy JSON: expected 'max_tokens' in output, got:\n%s", jsonStr)
	}
	if strings.Contains(jsonStr, "max_completion_tokens") {
		t.Errorf("Legacy JSON: did not expect 'max_completion_tokens' in output, got:\n%s", jsonStr)
	}
}

// TestWithLegacyCompat_Flag 验证 WithLegacyCompat Option 正确设置 legacyCompat 字段。
func TestWithLegacyCompat_Flag(t *testing.T) {
	legacy := NewCompletionsProviderWithOptions(WithAPIKey("test"), WithLegacyCompat())
	if !legacy.legacyCompat {
		t.Error("Provider with WithLegacyCompat(): legacyCompat should be true")
	}

	defaultP := NewCompletionsProvider("test")
	if defaultP.legacyCompat {
		t.Error("Provider without WithLegacyCompat(): legacyCompat should be false")
	}
}

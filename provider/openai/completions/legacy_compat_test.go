package completions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
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

// TestBuildParams_LegacyMaxTokens 验证 Legacy 模式使用 MaxTokens（旧字段名 max_tokens）。
func TestBuildParams_LegacyMaxTokens(t *testing.T) {
	p := newLegacyProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{MaxTokens: 4096})

	if param.IsOmitted(params.MaxTokens) {
		t.Error("Legacy: MaxTokens should be set")
	}
	if params.MaxTokens.Value != 4096 {
		t.Errorf("Legacy: MaxTokens.Value = %d, want 4096", params.MaxTokens.Value)
	}
	if !param.IsOmitted(params.MaxCompletionTokens) {
		t.Error("Legacy: MaxCompletionTokens should NOT be set")
	}
}

// TestBuildParams_DefaultMaxCompletionTokens 验证默认模式使用 MaxCompletionTokens（新字段名）。
func TestBuildParams_DefaultMaxCompletionTokens(t *testing.T) {
	p := newDefaultProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{MaxTokens: 4096})

	if param.IsOmitted(params.MaxCompletionTokens) {
		t.Error("Default: MaxCompletionTokens should be set")
	}
	if params.MaxCompletionTokens.Value != 4096 {
		t.Errorf("Default: MaxCompletionTokens.Value = %d, want 4096", params.MaxCompletionTokens.Value)
	}
	if !param.IsOmitted(params.MaxTokens) {
		t.Error("Default: MaxTokens should NOT be set")
	}
}

// TestBuildParams_LegacyNoParallelToolCalls 验证 Legacy 无工具时不发送 ParallelToolCalls。
func TestBuildParams_LegacyNoParallelToolCalls(t *testing.T) {
	p := newLegacyProvider(t)
	params := p.buildParams("", nil, &provider.ChatConfig{})

	if !param.IsOmitted(params.ParallelToolCalls) {
		t.Error("Legacy (no tools): ParallelToolCalls should NOT be set")
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

	if param.IsOmitted(params.ParallelToolCalls) {
		t.Error("Legacy (with tools): ParallelToolCalls should be set")
	}
	if !params.ParallelToolCalls.Value {
		t.Error("Legacy (with tools): ParallelToolCalls.Value should be true")
	}
}

// TestBuildParams_LegacyParallelToolCallsOmittedWhenFalse 验证 Legacy 模式下有工具但
// ParallelToolCalls 为 false（零值）时不发送 ParallelToolCalls 字段。
//
// 该测试复现并防护智谱 GLM 等 OpenAI 兼容端点的 400 code:1210 参数错误问题：
// 这些端点不支持 parallel_tool_calls 参数，即使发送 false 也会被拒绝。
func TestBuildParams_LegacyParallelToolCallsOmittedWhenFalse(t *testing.T) {
	p := newLegacyProvider(t)
	config := &provider.ChatConfig{
		Tools: []provider.Tool{
			{Type: "function", Function: provider.FunctionDef{Name: "test_tool"}},
		},
		ParallelToolCalls: false,
	}
	params := p.buildParams("", nil, config)

	if !param.IsOmitted(params.ParallelToolCalls) {
		t.Error("Legacy (with tools, ParallelToolCalls=false): ParallelToolCalls should NOT be set; " +
			"sending it to OpenAI-compatible endpoints (e.g. Zhipu GLM) causes 400 param error")
	}
}

// TestBuildStreamOptions_LegacyOmitted 验证 Legacy 模式下 buildStreamOptions 返回零值（序列化时省略）。
//
// 该测试复现并防护智谱 GLM 等 OpenAI 兼容端点的 400 code:1210 参数错误问题：
// 这些端点不支持 stream_options 参数，发送 include_usage 会导致请求被拒绝。
func TestBuildStreamOptions_LegacyOmitted(t *testing.T) {
	p := newLegacyProvider(t)
	opts := p.buildStreamOptions()

	if !param.IsOmitted(opts.IncludeUsage) {
		t.Error("Legacy: StreamOptions.IncludeUsage should be omitted; " +
			"sending stream_options to OpenAI-compatible endpoints (e.g. Zhipu GLM) causes 400 code:1210")
	}
}

// TestBuildStreamOptions_DefaultSet 验证默认模式（非 Legacy）下 buildStreamOptions 设置 IncludeUsage=true。
func TestBuildStreamOptions_DefaultSet(t *testing.T) {
	p := newDefaultProvider(t)
	opts := p.buildStreamOptions()

	if param.IsOmitted(opts.IncludeUsage) {
		t.Error("Default: StreamOptions.IncludeUsage should be set")
	}
	if !opts.IncludeUsage.Value {
		t.Error("Default: StreamOptions.IncludeUsage.Value should be true")
	}
}

// TestBuildParams_LegacyNoReasoningEffort 验证 Legacy 模式跳过 reasoning_effort 映射。
func TestBuildParams_LegacyNoReasoningEffort(t *testing.T) {
	p := newLegacyProvider(t)
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.ReasoningEffort != "" {
		t.Errorf("Legacy: ReasoningEffort should be empty, got %q", string(params.ReasoningEffort))
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

	extraFields := params.ExtraFields()
	if extraFields == nil {
		t.Fatal("Legacy: ExtraFields() should not be nil after SetExtraFields")
	}
	thinking, ok := extraFields["thinking"]
	if !ok {
		t.Error("Legacy: expected 'thinking' in ExtraFields")
	} else {
		thinkingMap, ok := thinking.(map[string]any)
		if !ok {
			t.Errorf("Legacy: thinking value type = %T, want map[string]any", thinking)
		} else if thinkingMap["type"] != "enabled" {
			t.Errorf("Legacy: thinking.type = %v, want 'enabled'", thinkingMap["type"])
		}
	}
}

// TestBuildParams_DefaultReasoningEffort 验证默认模式正常映射 reasoning_effort。
func TestBuildParams_DefaultReasoningEffort(t *testing.T) {
	p := newDefaultProvider(t)
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.ReasoningEffort != shared.ReasoningEffort("high") {
		t.Errorf("Default: ReasoningEffort = %q, want %q", string(params.ReasoningEffort), "high")
	}
}

// TestBuildParams_DefaultParallelToolCalls 验证默认模式无条件发送 ParallelToolCalls。
func TestBuildParams_DefaultParallelToolCalls(t *testing.T) {
	p := newDefaultProvider(t)
	// 无工具配置，默认模式仍应设置 ParallelToolCalls
	params := p.buildParams("", nil, &provider.ChatConfig{})

	if param.IsOmitted(params.ParallelToolCalls) {
		t.Error("Default: ParallelToolCalls should always be set")
	}
	// 默认 ChatConfig.ParallelToolCalls 为 false，验证值
	if params.ParallelToolCalls.Value {
		t.Error("Default: ParallelToolCalls.Value should be false (zero value from config)")
	}
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
	// 带 WithLegacyCompat 的 Provider
	legacy := NewCompletionsProviderWithOptions(WithAPIKey("test"), WithLegacyCompat())
	if !legacy.legacyCompat {
		t.Error("Provider with WithLegacyCompat(): legacyCompat should be true")
	}

	// 不带 WithLegacyCompat 的 Provider
	defaultP := NewCompletionsProvider("test")
	if defaultP.legacyCompat {
		t.Error("Provider without WithLegacyCompat(): legacyCompat should be false")
	}

	// 最简构造函数也不应开启 legacyCompat
	simpleP := NewCompletionsProvider("test")
	if simpleP.legacyCompat {
		t.Error("NewCompletionsProvider('test'): legacyCompat should be false")
	}
}

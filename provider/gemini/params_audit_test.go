package gemini

import (
	"math"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	"google.golang.org/genai"
)

// TestAudit_MaxTokens_Int64ToInt32_Overflow 验证 Gemini MaxTokens int64→int32 溢出风险。
//
// Severity: P0
// File:Line: provider/gemini/params.go:38
// Issue: config.MaxTokens 是 int64，但 genai.GenerateContentConfig.MaxOutputTokens 是 int32。
//        当 MaxTokens > math.MaxInt32 (2,147,483,647) 时，int32(config.MaxTokens) 静默溢出，
//        产生负数或截断值，导致 API 行为不可预测。
// Affected: Any→Gemini conversion with large MaxTokens
func TestAudit_MaxTokens_Int64ToInt32_Overflow(t *testing.T) {
	p := NewProvider("test-key")

	// 边界值：正好是 int32 最大值
	config := &provider.ChatConfig{
		Model:     "gemini-2.5-pro",
		MaxTokens: math.MaxInt32, // 2,147,483,647
	}
	gc := p.buildContentConfig("", config)
	if gc.MaxOutputTokens != math.MaxInt32 {
		t.Errorf("MaxOutputTokens = %d, want %d (int32 max)", gc.MaxOutputTokens, math.MaxInt32)
	}

	// 溢出值：int32 最大值 + 1 → 应被截断为 MaxInt32
	config.MaxTokens = math.MaxInt32 + 1
	gc = p.buildContentConfig("", config)
	if gc.MaxOutputTokens != math.MaxInt32 {
		t.Errorf("MaxTokens=%d overflow: MaxOutputTokens = %d, want %d (clamped to MaxInt32)",
			config.MaxTokens, gc.MaxOutputTokens, math.MaxInt32)
	}

	// 更大的溢出值 → 也应被截断为 MaxInt32
	config.MaxTokens = math.MaxInt64
	gc = p.buildContentConfig("", config)
	if gc.MaxOutputTokens != math.MaxInt32 {
		t.Errorf("MaxTokens=MaxInt64 overflow: MaxOutputTokens = %d, want %d (clamped to MaxInt32)",
			gc.MaxOutputTokens, math.MaxInt32)
	}
}

// TestAudit_MaxTokens_Int64ToInt32_NormalRange 验证正常范围内的 MaxTokens 值。
func TestAudit_MaxTokens_Int64ToInt32_NormalRange(t *testing.T) {
	p := NewProvider("test-key")

	tests := []struct {
		name      string
		maxTokens int64
		want      int32
	}{
		{"small", 100, 100},
		{"typical", 4096, 4096},
		{"large", 100000, 100000},
		{"max_safe", math.MaxInt32, math.MaxInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &provider.ChatConfig{
				Model:     "gemini-2.5-pro",
				MaxTokens: tt.maxTokens,
			}
			gc := p.buildContentConfig("", config)
			if gc.MaxOutputTokens != tt.want {
				t.Errorf("MaxOutputTokens = %d, want %d", gc.MaxOutputTokens, tt.want)
			}
		})
	}
}

// TestAudit_SafetySettings_TypeMismatch 验证 safety_settings 从 ProviderExtra 提取时的类型断言。
//
// Severity: P0
// File:Line: provider/gemini/params.go:64
// Issue: 当 safety_settings 通过 codec (geminiSafetySetting) → ProviderExtra 路径传入时，
//        存储类型是 []geminiSafetySetting（codec 本地结构体），
//        但 provider 期望 []*genai.SafetySetting（SDK 结构体指针切片），
//        类型断言 settings.([]*genai.SafetySetting) 永远返回 false，参数被静默丢弃。
// Affected: Gemini codec → relay → Gemini provider 的 safety_settings 透传路径
func TestAudit_SafetySettings_TypeMismatch(t *testing.T) {
	p := NewProvider("test-key")

	// 场景1：通过 WithSafetySettings Option 设置（正确类型）
	settings := []*genai.SafetySetting{
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockThresholdBlockNone,
		},
	}
	config := &provider.ChatConfig{
		Model: "gemini-2.5-pro",
		ProviderExtra: map[string]any{
			"safety_settings": settings,
		},
	}
	gc := p.buildContentConfig("", config)
	if len(gc.SafetySettings) != 1 {
		t.Errorf("With correct type: SafetySettings count = %d, want 1", len(gc.SafetySettings))
	}

	// 场景2：通过 codec 路径传入（[]geminiSafetySetting 类型 → 会被断言为 any slice）
	// 模拟 codec 的行为：存入 map[string]any 的 slice
	wrongTypeSettings := []map[string]string{
		{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
	}
	config.ProviderExtra["safety_settings"] = wrongTypeSettings
	gc = p.buildContentConfig("", config)
	if len(gc.SafetySettings) != 0 {
		t.Errorf("With wrong type: SafetySettings count = %d, want 0 (type assertion should fail)", len(gc.SafetySettings))
	}
	// 这个测试文档了已知问题：codec 存储的 []geminiSafetySetting 与 provider 期望的
	// []*genai.SafetySetting 类型不匹配，导致安全设置在 codec→relay→provider 路径上被静默丢弃
}

// TestAudit_UserID_Dropped 验证 UserID 在 Gemini 适配器中被丢弃。
//
// Severity: P1
// File:Line: provider/gemini/params.go:18 (buildContentConfig 缺失 UserID 映射)
// Issue: Gemini API 不直接支持 user 字段，但其他 OpenAI 兼容端点（如代理网关）可能需要。
//        当前 buildContentConfig 不处理 config.UserID，导致 UserID 被静默丢弃。
// Affected: Any→Gemini with UserID set
func TestAudit_UserID_Dropped(t *testing.T) {
	p := NewProvider("test-key")

	config := &provider.ChatConfig{
		Model:  "gemini-2.5-pro",
		UserID: "user-123",
	}
	gc := p.buildContentConfig("", config)

	// Gemini SDK GenerateContentConfig 没有 User 字段
	// 验证 UserID 确实没有被映射到任何字段（文档化已知限制）
	// Labels 是唯一可能的映射目标，但当前未使用
	if gc.Labels != nil {
		// 如果 Labels 被设置，检查是否包含 UserID
		if _, ok := gc.Labels["user_id"]; ok {
			return // UserID 被映射到了 Labels
		}
	}
	// 预期：UserID 被丢弃
	t.Logf("P1 NOTE: UserID=%q is dropped by Gemini adapter (no SDK field available)", config.UserID)
}

// TestAudit_ParallelToolCalls_Dropped 验证 ParallelToolCalls 在 Gemini 适配器中被丢弃。
//
// Severity: P2
// File:Line: provider/gemini/params.go (buildContentConfig 缺失 ParallelToolCalls 映射)
// Issue: Gemini API 不支持 parallel_tool_calls 参数，buildContentConfig 不处理此字段。
// Affected: Any→Gemini with ParallelToolCalls=true
func TestAudit_ParallelToolCalls_Dropped(t *testing.T) {
	p := NewProvider("test-key")

	config := &provider.ChatConfig{
		Model:             "gemini-2.5-pro",
		ParallelToolCalls: true,
		Tools: []provider.Tool{
			{
				Type: "function",
				Function: provider.FunctionDef{
					Name: "test_func",
				},
			},
		},
	}
	gc := p.buildContentConfig("", config)

	// Gemini SDK 没有 ParallelToolCalls 字段，参数被丢弃是预期行为
	// 但应该通过文档或日志告知用户
	_ = gc
	t.Logf("P2 NOTE: ParallelToolCalls=%v is dropped by Gemini adapter (not supported by Gemini API)", config.ParallelToolCalls)
}

// TestAudit_ResponseFormat_Mapping 验证 ResponseFormat 映射。
func TestAudit_ResponseFormat_Mapping(t *testing.T) {
	p := NewProvider("test-key")

	tests := []struct {
		name           string
		responseFormat string
		wantMIME       string
	}{
		{"json_object", "json_object", "application/json"},
		{"text", "text", ""},  // text 不映射
		{"empty", "", ""},     // 空值不设置
		{"other", "xml", ""},  // 未知值不设置
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &provider.ChatConfig{
				Model:          "gemini-2.5-pro",
				ResponseFormat: tt.responseFormat,
			}
			gc := p.buildContentConfig("", config)
			if gc.ResponseMIMEType != tt.wantMIME {
				t.Errorf("ResponseMIMEType = %q, want %q", gc.ResponseMIMEType, tt.wantMIME)
			}
		})
	}
}

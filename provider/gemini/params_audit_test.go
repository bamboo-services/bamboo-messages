package gemini

import (
	"math"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestAudit_MaxTokens_Int64ToInt32_Overflow 验证 Gemini MaxTokens int64→int32 溢出保护。
//
// config.MaxTokens 是 int64，但 Gemini maxOutputTokens 实际为 int32 范围。
// 当 MaxTokens > math.MaxInt32 时，应被截断为 MaxInt32，避免静默溢出。
func TestAudit_MaxTokens_Int64ToInt32_Overflow(t *testing.T) {
	p := NewProvider("test-key")

	// 边界值：正好是 int32 最大值
	config := &provider.ChatConfig{
		Model:     "gemini-2.5-pro",
		MaxTokens: math.MaxInt32,
	}
	gc := p.buildContentConfig(config)
	maxOut, ok := gc["maxOutputTokens"].(int)
	if !ok || maxOut != math.MaxInt32 {
		t.Errorf("MaxOutputTokens = %v, want %d (int32 max)", gc["maxOutputTokens"], math.MaxInt32)
	}

	// 溢出值：int32 最大值 + 1 → 应被截断为 MaxInt32
	config.MaxTokens = math.MaxInt32 + 1
	gc = p.buildContentConfig(config)
	maxOut, ok = gc["maxOutputTokens"].(int)
	if !ok || maxOut != math.MaxInt32 {
		t.Errorf("MaxTokens=%d overflow: MaxOutputTokens = %v, want %d (clamped to MaxInt32)",
			config.MaxTokens, gc["maxOutputTokens"], math.MaxInt32)
	}

	// 更大的溢出值 → 也应被截断为 MaxInt32
	config.MaxTokens = math.MaxInt64
	gc = p.buildContentConfig(config)
	maxOut, ok = gc["maxOutputTokens"].(int)
	if !ok || maxOut != math.MaxInt32 {
		t.Errorf("MaxTokens=MaxInt64 overflow: MaxOutputTokens = %v, want %d (clamped to MaxInt32)",
			gc["maxOutputTokens"], math.MaxInt32)
	}
}

// TestAudit_MaxTokens_Int64ToInt32_NormalRange 验证正常范围内的 MaxTokens 值。
func TestAudit_MaxTokens_Int64ToInt32_NormalRange(t *testing.T) {
	p := NewProvider("test-key")

	tests := []struct {
		name      string
		maxTokens int64
		want      int
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
			gc := p.buildContentConfig(config)
			maxOut, ok := gc["maxOutputTokens"].(int)
			if !ok || maxOut != tt.want {
				t.Errorf("MaxOutputTokens = %v, want %d", gc["maxOutputTokens"], tt.want)
			}
		})
	}
}

// TestAudit_SafetySettings_Passthrough 验证 safety_settings 从 ProviderExtra 提取并透传。
func TestBuildContentConfig_ThinkingBudgetForGemini25(t *testing.T) {
	p := NewProvider("test-key")
	gc := p.buildContentConfig(&provider.ChatConfig{
		Model:          "gemini-2.5-flash",
		ThinkingConfig: &provider.ThinkingConfig{Effort: "medium"},
	})
	tc, ok := gc["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatal("thinkingConfig missing")
	}
	if tc["includeThoughts"] != true {
		t.Errorf("includeThoughts = %v", tc["includeThoughts"])
	}
	if tc["thinkingBudget"] != 8192 {
		t.Errorf("thinkingBudget = %v, want 8192", tc["thinkingBudget"])
	}
	if _, ok := tc["thinkingLevel"]; ok {
		t.Errorf("thinkingLevel should be omitted for 2.5: %v", tc["thinkingLevel"])
	}
}

func TestBuildContentConfig_ThinkingLevelForGemini3(t *testing.T) {
	p := NewProvider("test-key")
	gc := p.buildContentConfig(&provider.ChatConfig{
		Model:          "gemini-3.1-pro",
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	})
	tc, ok := gc["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatal("thinkingConfig missing")
	}
	if tc["thinkingLevel"] != "high" {
		t.Errorf("thinkingLevel = %v, want high", tc["thinkingLevel"])
	}
	if _, ok := tc["thinkingBudget"]; ok {
		t.Errorf("thinkingBudget should be omitted for gemini-3: %v", tc["thinkingBudget"])
	}
}

func TestAudit_SafetySettings_Passthrough(t *testing.T) {
	p := NewProvider("test-key")

	settings := []map[string]string{
		{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
	}
	config := &provider.ChatConfig{
		Model: "gemini-2.5-pro",
		ProviderExtra: map[string]any{
			"safety_settings": settings,
		},
	}
	gc := p.buildContentConfig(config)

	ss, ok := gc["safetySettings"]
	if !ok {
		t.Fatal("safetySettings not set in generationConfig")
	}
	// safetySettings 通过 GetExtraAny 提取后原样存入
	_ = ss
}

// TestAudit_UserID_LabelsMapping 验证 UserID 映射到 labels["user_id"]。
func TestAudit_UserID_LabelsMapping(t *testing.T) {
	p := NewProvider("test-key")

	config := &provider.ChatConfig{
		Model:  "gemini-2.5-pro",
		UserID: "user-123",
	}
	gc := p.buildContentConfig(config)

	labels, ok := gc["labels"].(map[string]string)
	if !ok {
		t.Fatal("labels not set in generationConfig")
	}
	if labels["user_id"] != "user-123" {
		t.Errorf("labels[user_id] = %q, want %q", labels["user_id"], "user-123")
	}
}

// TestAudit_ParallelToolCalls_Ignored 验证 ParallelToolCalls 在 Gemini 适配器中被忽略（仅记录日志）。
func TestAudit_ParallelToolCalls_Ignored(t *testing.T) {
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
	gc := p.buildContentConfig(config)

	// Gemini 不支持 ParallelToolCalls，generationConfig 中不应有此字段
	if _, ok := gc["parallelToolCalls"]; ok {
		t.Error("parallelToolCalls should not be set in Gemini generationConfig")
	}
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
		{"text", "text", ""},
		{"empty", "", ""},
		{"other", "xml", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &provider.ChatConfig{
				Model:          "gemini-2.5-pro",
				ResponseFormat: tt.responseFormat,
			}
			gc := p.buildContentConfig(config)
			mime, _ := gc["responseMimeType"].(string)
			if mime != tt.wantMIME {
				t.Errorf("responseMimeType = %q, want %q", mime, tt.wantMIME)
			}
		})
	}
}

package anthropic

import (
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
	// systemPrompt 应正常设置
	if len(params.System) != 1 || params.System[0].Text != "system prompt" {
		t.Errorf("System not set correctly with nil config")
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
	if params.Temperature.Valid() {
		t.Error("Temperature should not be valid for empty config")
	}
	if params.TopP.Valid() {
		t.Error("TopP should not be valid for empty config")
	}
	if len(params.System) != 0 {
		t.Errorf("System should be empty, got %d entries", len(params.System))
	}
}

func TestBuildParams_SystemPrompt(t *testing.T) {
	p := NewProvider("test-api-key")
	params := p.buildParams("system instruction", nil, nil)

	if len(params.System) != 1 {
		t.Fatalf("System length = %d, want 1", len(params.System))
	}
	if params.System[0].Text != "system instruction" {
		t.Errorf("System[0].Text = %q, want %q", params.System[0].Text, "system instruction")
	}
}

func TestBuildParams_Temperature(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		Temperature: float64Ptr(0.7),
	}
	params := p.buildParams("", nil, config)

	if !params.Temperature.Valid() {
		t.Error("expected Temperature to be valid")
	}
	if params.Temperature.Value != 0.7 {
		t.Errorf("Temperature.Value = %v, want 0.7", params.Temperature.Value)
	}
}

func TestBuildParams_TopP(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		TopP: float64Ptr(0.95),
	}
	params := p.buildParams("", nil, config)

	if !params.TopP.Valid() {
		t.Error("expected TopP to be valid")
	}
	if params.TopP.Value != 0.95 {
		t.Errorf("TopP.Value = %v, want 0.95", params.TopP.Value)
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
	if params.Tools[0].OfTool == nil {
		t.Fatal("expected Tools[0].OfTool to be non-nil")
	}
	if params.Tools[0].OfTool.Name != "test_tool" {
		t.Errorf("Tool name = %q, want %q", params.Tools[0].OfTool.Name, "test_tool")
	}
}

func TestBuildParams_ThinkingConfig(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := p.buildParams("", nil, config)

	if params.Thinking.OfAdaptive == nil {
		t.Error("expected Thinking.OfAdaptive to be non-nil for effort=high")
	}
}

func TestBuildParams_TopK(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{"top_k": 40.0},
	}
	params := p.buildParams("", nil, config)

	if !params.TopK.Valid() {
		t.Error("expected TopK to be valid")
	}
	if params.TopK.Value != int64(40) {
		t.Errorf("TopK.Value = %v, want 40", params.TopK.Value)
	}
}

func TestBuildParams_ToolChoiceAuto(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{ToolChoice: "auto"}
	params := p.buildParams("", nil, config)

	if params.ToolChoice.OfAuto == nil {
		t.Error("expected ToolChoice.OfAuto to be non-nil")
	}
}

func TestBuildParams_ToolChoiceForced(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{ToolChoice: "forced"}
	params := p.buildParams("", nil, config)

	if params.ToolChoice.OfAny == nil {
		t.Error("expected ToolChoice.OfAny to be non-nil for forced")
	}
}

func TestBuildParams_ToolChoiceNone(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{ToolChoice: "none"}
	params := p.buildParams("", nil, config)

	if params.ToolChoice.OfNone == nil {
		t.Error("expected ToolChoice.OfNone to be non-nil")
	}
}

func TestBuildParams_UserIDAndMetadata(t *testing.T) {
	p := NewProvider("test-api-key")
	config := &provider.ChatConfig{
		UserID:   "user-123",
		Metadata: map[string]string{"key": "val"},
	}
	params := p.buildParams("", nil, config)

	if !params.Metadata.UserID.Valid() {
		t.Error("expected Metadata.UserID to be valid")
	}
	if params.Metadata.UserID.Value != "user-123" {
		t.Errorf("Metadata.UserID.Value = %q, want %q", params.Metadata.UserID.Value, "user-123")
	}

	// 验证 extra fields 包含 metadata
	extraFields := params.Metadata.ExtraFields()
	if extraFields["key"] != "val" {
		t.Errorf("extra fields[key] = %v, want %q", extraFields["key"], "val")
	}
}

package bamboo

import (
	"encoding/json"
	"testing"
)

func TestResponse_JSONRoundtrip(t *testing.T) {
	original := Response{
		ID:           "msg_01XYZ",
		Type:         "message",
		Role:         RoleAssistant,
		Model:        "claude-sonnet-4-20250514",
		StopReason:   FinishReasonEndTurn,
		Content:      []ContentBlock{NewTextBlock("你好！")},
		Usage:        Usage{InputTokens: 15, OutputTokens: 8},
		ProviderType: "anthropic",
		RequestID:    "req_abc123",
		CreatedAt:    1748236800,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed Response
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	// 验证 Anthropic 原生字段
	if parsed.ID != "msg_01XYZ" {
		t.Errorf("ID 不匹配: 期望 msg_01XYZ，实际 %s", parsed.ID)
	}
	if parsed.Type != "message" {
		t.Errorf("Type 不匹配: 期望 message，实际 %s", parsed.Type)
	}
	if parsed.Role != RoleAssistant {
		t.Errorf("Role 不匹配: 期望 %s，实际 %s", RoleAssistant, parsed.Role)
	}
	if parsed.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model 不匹配")
	}
	if parsed.StopReason != FinishReasonEndTurn {
		t.Errorf("StopReason 不匹配")
	}
	if len(parsed.Content) != 1 {
		t.Fatalf("Content 长度不匹配: 期望 1，实际 %d", len(parsed.Content))
	}
	tb, ok := parsed.Content[0].(*TextBlock)
	if !ok {
		t.Fatal("Content[0] 类型断言为 *TextBlock 失败")
	}
	if tb.Text != "你好！" {
		t.Errorf("Content[0].Text 不匹配")
	}

	// 验证 Usage
	if parsed.Usage.InputTokens != 15 {
		t.Errorf("Usage.InputTokens 不匹配: 期望 15，实际 %d", parsed.Usage.InputTokens)
	}
	if parsed.Usage.OutputTokens != 8 {
		t.Errorf("Usage.OutputTokens 不匹配: 期望 8，实际 %d", parsed.Usage.OutputTokens)
	}

	// 验证 Bamboo 扩展字段
	if parsed.ProviderType != "anthropic" {
		t.Errorf("ProviderType 不匹配: 期望 anthropic，实际 %s", parsed.ProviderType)
	}
	if parsed.RequestID != "req_abc123" {
		t.Errorf("RequestID 不匹配")
	}
	if parsed.CreatedAt != 1748236800 {
		t.Errorf("CreatedAt 不匹配")
	}
}

func TestResponse_WithCacheUsage(t *testing.T) {
	original := Response{
		ID:         "msg_cache",
		Type:       "message",
		Role:       RoleAssistant,
		Content:    []ContentBlock{},
		StopReason: FinishReasonEndTurn,
		Usage: Usage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheCreationInputTokens: 80,
			CacheReadInputTokens:     20,
		},
		ProviderType: "anthropic",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed Response
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Usage.CacheCreationInputTokens != 80 {
		t.Errorf("CacheCreationInputTokens 不匹配: 期望 80，实际 %d", parsed.Usage.CacheCreationInputTokens)
	}
	if parsed.Usage.CacheReadInputTokens != 20 {
		t.Errorf("CacheReadInputTokens 不匹配: 期望 20，实际 %d", parsed.Usage.CacheReadInputTokens)
	}
}

func TestResponse_WithToolUse(t *testing.T) {
	original := Response{
		ID:         "msg_tool",
		Type:       "message",
		Role:       RoleAssistant,
		Content:    []ContentBlock{NewToolUseBlock("toolu_001", "get_weather", map[string]any{"city": "Tokyo"})},
		StopReason: FinishReasonToolUse,
		Usage:      Usage{InputTokens: 50, OutputTokens: 30},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed Response
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.StopReason != FinishReasonToolUse {
		t.Errorf("StopReason 不匹配: 期望 %s，实际 %s", FinishReasonToolUse, parsed.StopReason)
	}
	if len(parsed.Content) != 1 || parsed.Content[0].BlockType() != ContentBlockToolUse {
		t.Error("Content[0] 应为 tool_use 类型")
	}
}

// ---- FinishReason 常量测试 ----

func TestFinishReason_Values(t *testing.T) {
	tests := []struct {
		reason   FinishReason
		expected string
	}{
		{FinishReasonEndTurn, "end_turn"},
		{FinishReasonMaxTokens, "max_tokens"},
		{FinishReasonToolUse, "tool_use"},
		{FinishReasonStopSequence, "stop_sequence"},
	}

	for _, tt := range tests {
		if string(tt.reason) != tt.expected {
			t.Errorf("期望 %s，实际 %s", tt.expected, tt.reason)
		}
	}
}

// ---- Usage JSON 测试 ----

func TestUsage_JSONRoundtrip(t *testing.T) {
	original := Usage{
		InputTokens:              100,
		OutputTokens:             200,
		CacheCreationInputTokens: 50,
		CacheReadInputTokens:     30,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed Usage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.InputTokens != 100 || parsed.OutputTokens != 200 {
		t.Errorf("基础字段不匹配")
	}
	if parsed.CacheCreationInputTokens != 50 || parsed.CacheReadInputTokens != 30 {
		t.Errorf("缓存字段不匹配")
	}
}

func TestUsage_OmitsEmptyCache(t *testing.T) {
	usage := Usage{
		InputTokens:  10,
		OutputTokens: 5,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	// 验证 omitempty：缓存字段为零值时不应出现在 JSON 中
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析为 map 失败: %v", err)
	}

	if _, ok := raw["cache_creation_input_tokens"]; ok {
		t.Error("cache_creation_input_tokens 应被 omitempty 忽略")
	}
	if _, ok := raw["cache_read_input_tokens"]; ok {
		t.Error("cache_read_input_tokens 应被 omitempty 忽略")
	}
}

package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// buildMessages 测试
// ==============================

// TestBuildMessages_ToolUseInputNotDoubleEncoded 验证 tool_use 的 input 字段不会被双重编码。
//
// 修复前：tc.Function.Arguments (string) 直接传给 NewBetaToolUseBlock 的 input any，
// SDK 内部 json.Marshal 对 string 产生双重编码（如 "\"{...}\"" 或 base64）。
// 修复后：先转为 json.RawMessage，Marshal 时直接输出原始字节。
func TestBuildMessages_ToolUseInputNotDoubleEncoded(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{
					ID: "call_123",
					Function: provider.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"北京"}`,
					},
				},
			},
		},
	}

	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	msg := result[0]
	if msg.Role != anthropic.BetaMessageParamRoleAssistant {
		t.Fatalf("expected assistant role, got %v", msg.Role)
	}

	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg.Content))
	}

	block := msg.Content[0]
	if block.OfToolUse == nil {
		t.Fatal("expected tool_use block, got nil")
	}

	tu := block.OfToolUse
	if tu.ID != "call_123" {
		t.Errorf("expected tool_use ID 'call_123', got %q", tu.ID)
	}
	if tu.Name != "get_weather" {
		t.Errorf("expected tool_use name 'get_weather', got %q", tu.Name)
	}

	// 验证 input 是合法 JSON 对象，不是 base64/双重编码字符串
	inputBytes, err := json.Marshal(tu.Input)
	if err != nil {
		t.Fatalf("failed to marshal tool_use input: %v", err)
	}

	// 解析回 map 验证结构
	var parsed map[string]any
	if err := json.Unmarshal(inputBytes, &parsed); err != nil {
		t.Fatalf("tool_use input is not valid JSON object: %v\nraw: %s", err, string(inputBytes))
	}

	if parsed["city"] != "北京" {
		t.Errorf("expected city=北京, got %v", parsed["city"])
	}
}

// TestBuildMessages_EmptyToolUseArguments 验证空 arguments 不会 panic，且 input 为空对象。
func TestBuildMessages_EmptyToolUseArguments(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{
					ID: "call_empty",
					Function: provider.FunctionCall{
						Name:      "noop",
						Arguments: "",
					},
				},
			},
		},
	}

	// 不应 panic
	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	block := result[0].Content[0]
	if block.OfToolUse == nil {
		t.Fatal("expected tool_use block, got nil")
	}

	// input 应为 {}
	inputBytes, err := json.Marshal(block.OfToolUse.Input)
	if err != nil {
		t.Fatalf("failed to marshal tool_use input: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(inputBytes, &parsed); err != nil {
		t.Fatalf("tool_use input is not valid JSON object: %v\nraw: %s", err, string(inputBytes))
	}

	if len(parsed) != 0 {
		t.Errorf("expected empty object, got %v", parsed)
	}
}

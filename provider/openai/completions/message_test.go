package completions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestBuildAssistantMessage_OmitsEmptyToolCalls 验证没有工具调用的 assistant 消息
// 在序列化后不会包含空 tool_calls 数组。
//
// 第三方 OpenAI 兼容端点（如 Kimi coding API）可能将 "tool_calls": [] 视为无效请求，
// 从而返回空响应（choices=0）。通过保持 ToolCalls 为 nil，可让 SDK 的 omitzero 标签在
// 序列化时省略该字段。
func TestBuildAssistantMessage_OmitsEmptyToolCalls(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	msg := provider.Message{
		Role:    provider.RoleAssistant,
		Content: "Hello, how can I help you?",
	}

	result := p.buildAssistantMessage(msg)
	if result.OfAssistant == nil {
		t.Fatal("expected assistant message")
	}

	raw, err := json.Marshal(result.OfAssistant)
	if err != nil {
		t.Fatalf("failed to marshal assistant message: %v", err)
	}

	if strings.Contains(string(raw), `"tool_calls"`) {
		t.Errorf("assistant message without tool calls should not contain tool_calls field, got: %s", raw)
	}
}

// TestBuildAssistantMessage_IncludesToolCallsWhenPresent 验证有工具调用的 assistant 消息
// 仍然正确包含 tool_calls 字段。
func TestBuildAssistantMessage_IncludesToolCallsWhenPresent(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	msg := provider.Message{
		Role:    provider.RoleAssistant,
		Content: "Let me check that.",
		ToolCalls: []provider.ToolCall{
			{
				ID:   "call-123",
				Type: "function",
				Function: provider.FunctionCall{
					Name:      "get_weather",
					Arguments: `{"location": "Tokyo"}`,
				},
			},
		},
	}

	result := p.buildAssistantMessage(msg)
	if result.OfAssistant == nil {
		t.Fatal("expected assistant message")
	}

	if len(result.OfAssistant.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(result.OfAssistant.ToolCalls))
	}

	raw, err := json.Marshal(result.OfAssistant)
	if err != nil {
		t.Fatalf("failed to marshal assistant message: %v", err)
	}

	if !strings.Contains(string(raw), `"tool_calls"`) {
		t.Errorf("assistant message with tool calls should contain tool_calls field, got: %s", raw)
	}
}

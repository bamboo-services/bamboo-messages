package completions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestBuildAssistantMessage_OmitsEmptyToolCalls 验证没有工具调用的 assistant 消息
// 在序列化后不会包含空 tool_calls 数组。
func TestBuildAssistantMessage_OmitsEmptyToolCalls(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	msg := provider.Message{
		Role:    provider.RoleAssistant,
		Content: "Hello, how can I help you?",
	}

	result := p.buildAssistantMessage(msg)
	if result["role"] != "assistant" {
		t.Fatal("expected assistant message")
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal assistant message: %v", err)
	}

	if strings.Contains(string(raw), `"tool_calls"`) {
		t.Errorf("assistant message without tool calls should not contain tool_calls field, got: %s", raw)
	}
}

// TestBuildAssistantMessage_IncludesToolCallsWhenPresent 验证有工具调用的 assistant 消息包含 tool_calls。
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
	if result["role"] != "assistant" {
		t.Fatal("expected assistant message")
	}

	tc, ok := result["tool_calls"].([]map[string]any)
	if !ok {
		t.Fatalf("expected tool_calls to be []map[string]any, got %T", result["tool_calls"])
	}
	if len(tc) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(tc))
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal assistant message: %v", err)
	}

	if !strings.Contains(string(raw), `"tool_calls"`) {
		t.Errorf("assistant message with tool calls should contain tool_calls field, got: %s", raw)
	}
}

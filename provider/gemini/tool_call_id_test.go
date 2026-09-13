package gemini

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

func TestGeminiToolCallIdentityReservedIDs(t *testing.T) {
	// Given: 固定命名空间使碰撞分支可重复触发，不替换随机源。
	history := []provider.Message{
		{ToolCalls: []provider.ToolCall{{ID: "gemini_call_fixture_1"}}},
		{ToolCallID: "gemini_call_fixture_2"},
	}
	ids := newToolCallIDs(history)
	ids.namespace = "fixture"
	// When
	explicit := ids.next("gemini_call_fixture_3")
	first, second := ids.next(""), ids.next("")
	// Then
	if explicit != "gemini_call_fixture_3" || first != "gemini_call_fixture_4" || second != "gemini_call_fixture_5" {
		t.Fatalf("IDs=%q/%q/%q; want explicit preserved and generated ordinals 4/5", explicit, first, second)
	}
	if history[0].ToolCalls[0].ID != "gemini_call_fixture_1" || history[1].ToolCallID != "gemini_call_fixture_2" {
		t.Fatal("input history mutated")
	}
}

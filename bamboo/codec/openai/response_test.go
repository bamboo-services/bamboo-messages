package openai

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

func TestSerializeResponse_TextOnly(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_001",
		Model:      "gpt-4o",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("Hello!"),
		},
		Usage: bamboo.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out openaiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.ID != "resp_001" {
		t.Errorf("ID = %q", out.ID)
	}
	if out.Object != "chat.completion" {
		t.Errorf("Object = %q, want %q", out.Object, "chat.completion")
	}
	if out.Model != "gpt-4o" {
		t.Errorf("Model = %q", out.Model)
	}
	if len(out.Choices) != 1 {
		t.Fatalf("Choices len = %d", len(out.Choices))
	}
	choice := out.Choices[0]
	if choice.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", choice.FinishReason, "stop")
	}
	if choice.Message.Content == nil || *choice.Message.Content != "Hello!" {
		t.Errorf("Content = %v", choice.Message.Content)
	}
	if choice.Message.Role != "assistant" {
		t.Errorf("Role = %q", choice.Message.Role)
	}
}

func TestSerializeResponse_ToolCalls(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_002",
		Model:      "gpt-4o",
		StopReason: bamboo.FinishReasonToolUse,
		Content: []bamboo.ContentBlock{
			bamboo.NewToolUseBlock("call_abc", "get_weather", map[string]any{"city": "SF"}),
		},
		Usage: bamboo.Usage{
			InputTokens:  15,
			OutputTokens: 10,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out openaiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	choice := out.Choices[0]
	if choice.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", choice.FinishReason, "tool_calls")
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(choice.Message.ToolCalls))
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("ID = %q", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("Type = %q", tc.Type)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("Name = %q", tc.Function.Name)
	}
	// arguments should be valid JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Errorf("Arguments not valid JSON: %v", err)
	}
}

func TestSerializeResponse_Thinking(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_003",
		Model:      "o1",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("The answer is 42."),
			bamboo.NewThinkingBlock("Let me think...", "sig_abc"),
		},
		Usage: bamboo.Usage{
			InputTokens:  20,
			OutputTokens: 30,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out openaiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	choice := out.Choices[0]
	if choice.Message.ReasoningContent != "Let me think..." {
		t.Errorf("ReasoningContent = %q, want %q", choice.Message.ReasoningContent, "Let me think...")
	}
	if choice.Message.ThinkingSignature != "sig_abc" {
		t.Errorf("ThinkingSignature = %q, want sig_abc", choice.Message.ThinkingSignature)
	}
	if choice.Message.Content == nil || *choice.Message.Content != "The answer is 42." {
		t.Errorf("Content = %v", choice.Message.Content)
	}
}

func TestSerializeResponse_Usage(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_004",
		Model:      "gpt-4o",
		StopReason: bamboo.FinishReasonEndTurn,
		Content:    []bamboo.ContentBlock{bamboo.NewTextBlock("ok")},
		Usage: bamboo.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out openaiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", out.Usage.PromptTokens)
	}
	if out.Usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", out.Usage.CompletionTokens)
	}
	if out.Usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", out.Usage.TotalTokens)
	}
}

func TestSerializeResponse_FinishReasonMapping(t *testing.T) {
	tests := []struct {
		reason bamboo.FinishReason
		want   string
	}{
		{bamboo.FinishReasonEndTurn, "stop"},
		{bamboo.FinishReasonMaxTokens, "length"},
		{bamboo.FinishReasonToolUse, "tool_calls"},
		{bamboo.FinishReasonStopSequence, "stop"},
	}

	for _, tt := range tests {
		got := mapFinishReason(tt.reason)
		if got != tt.want {
			t.Errorf("mapFinishReason(%q) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestSerializeResponse_EmptyContent(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_005",
		Model:      "gpt-4o",
		StopReason: bamboo.FinishReasonEndTurn,
		Content:    []bamboo.ContentBlock{},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out openaiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.Choices[0].Message.Content == nil {
		t.Error("Content should not be nil when no tool calls")
	}
}

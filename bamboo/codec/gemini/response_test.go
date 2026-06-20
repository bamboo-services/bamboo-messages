package gemini

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

func TestSerializeResponse_TextOnly(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp-123",
		Model:      "gemini-2.0-flash",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("Hello, world!"),
		},
		Usage: bamboo.Usage{
			InputTokens:  10,
			OutputTokens: 20,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse error = %v", err)
	}

	var out geminiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// candidates
	if len(out.Candidates) != 1 {
		t.Fatalf("Candidates len = %d, want 1", len(out.Candidates))
	}
	cand := out.Candidates[0]
	if cand.Index != 0 {
		t.Errorf("Index = %d", cand.Index)
	}
	if cand.FinishReason != "STOP" {
		t.Errorf("FinishReason = %q, want STOP", cand.FinishReason)
	}
	if cand.Content == nil {
		t.Fatal("Content is nil")
	}
	if cand.Content.Role != "model" {
		t.Errorf("Role = %q, want model", cand.Content.Role)
	}
	if len(cand.Content.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(cand.Content.Parts))
	}
	if cand.Content.Parts[0].Text != "Hello, world!" {
		t.Errorf("text = %q", cand.Content.Parts[0].Text)
	}

	// usageMetadata
	if out.UsageMetadata == nil {
		t.Fatal("UsageMetadata is nil")
	}
	if out.UsageMetadata.PromptTokenCount != 10 {
		t.Errorf("PromptTokenCount = %d, want 10", out.UsageMetadata.PromptTokenCount)
	}
	if out.UsageMetadata.CandidatesTokenCount != 20 {
		t.Errorf("CandidatesTokenCount = %d, want 20", out.UsageMetadata.CandidatesTokenCount)
	}
	if out.UsageMetadata.TotalTokenCount != 30 {
		t.Errorf("TotalTokenCount = %d, want 30", out.UsageMetadata.TotalTokenCount)
	}

	// modelVersion
	if out.ModelVersion != "gemini-2.0-flash" {
		t.Errorf("ModelVersion = %q", out.ModelVersion)
	}
}

func TestSerializeResponse_FunctionCall(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp-456",
		Model:      "gemini-2.0-flash",
		StopReason: bamboo.FinishReasonToolUse,
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("Let me check."),
			&bamboo.ToolUseBlock{
				Type:  bamboo.ContentBlockToolUse,
				ID:    "call-abc",
				Name:  "get_weather",
				Input: json.RawMessage(`{"city":"SF"}`),
			},
		},
		Usage: bamboo.Usage{
			InputTokens:  5,
			OutputTokens: 15,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse error = %v", err)
	}

	var out geminiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	parts := out.Candidates[0].Content.Parts
	if len(parts) != 2 {
		t.Fatalf("Parts len = %d, want 2", len(parts))
	}

	// text
	if parts[0].Text != "Let me check." {
		t.Errorf("parts[0].text = %q", parts[0].Text)
	}

	// functionCall
	if parts[1].FunctionCall == nil {
		t.Fatal("parts[1].functionCall is nil")
	}
	if parts[1].FunctionCall.Name != "get_weather" {
		t.Errorf("functionCall.name = %q", parts[1].FunctionCall.Name)
	}
	if parts[1].FunctionCall.ID != "call-abc" {
		t.Errorf("functionCall.id = %q", parts[1].FunctionCall.ID)
	}

	var args map[string]any
	if err := json.Unmarshal(parts[1].FunctionCall.Args, &args); err != nil {
		t.Fatalf("failed to unmarshal args: %v", err)
	}
	if args["city"] != "SF" {
		t.Errorf("args.city = %v", args["city"])
	}
}

func TestSerializeResponse_ThinkingBlock_Serialized(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp-789",
		Model:      "gemini-2.5-pro",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewThinkingBlock("thinking content", "sig"),
			bamboo.NewTextBlock("answer"),
		},
		Usage: bamboo.Usage{},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse error = %v", err)
	}

	var out geminiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	parts := out.Candidates[0].Content.Parts
	// Fix: thinking should now be serialized as {text, thought: true}
	if len(parts) != 2 {
		t.Fatalf("Parts len = %d, want 2 (thinking + text)", len(parts))
	}
	// thinking part
	if parts[0].Text != "thinking content" {
		t.Errorf("parts[0].text = %q, want 'thinking content'", parts[0].Text)
	}
	if !parts[0].Thought {
		t.Errorf("parts[0].thought = false, want true")
	}
	// text part
	if parts[1].Text != "answer" {
		t.Errorf("parts[1].text = %q, want 'answer'", parts[1].Text)
	}
	if parts[1].Thought {
		t.Errorf("parts[1].thought = true, want false")
	}
}

func TestSerializeResponse_FinishReasonMapping(t *testing.T) {
	tests := []struct {
		reason bamboo.FinishReason
		want   string
	}{
		{bamboo.FinishReasonEndTurn, "STOP"},
		{bamboo.FinishReasonMaxTokens, "MAX_TOKENS"},
		{bamboo.FinishReasonToolUse, "STOP"},
		{bamboo.FinishReasonStopSequence, "STOP"},
	}

	for _, tt := range tests {
		resp := &bamboo.Response{
			StopReason: tt.reason,
			Content:    []bamboo.ContentBlock{bamboo.NewTextBlock("x")},
		}
		data, err := serializeResponse(resp)
		if err != nil {
			t.Fatalf("serializeResponse(%v) error = %v", tt.reason, err)
		}
		var out geminiResponse
		json.Unmarshal(data, &out)
		if out.Candidates[0].FinishReason != tt.want {
			t.Errorf("reason %v → %q, want %q", tt.reason, out.Candidates[0].FinishReason, tt.want)
		}
	}
}

func TestSerializeResponse_UsageMetadata_TotalCalculation(t *testing.T) {
	resp := &bamboo.Response{
		StopReason: bamboo.FinishReasonEndTurn,
		Content:    []bamboo.ContentBlock{bamboo.NewTextBlock("x")},
		Usage: bamboo.Usage{
			InputTokens:  100,
			OutputTokens: 200,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse error = %v", err)
	}

	var out geminiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// totalTokenCount = prompt + candidates
	if out.UsageMetadata.TotalTokenCount != 300 {
		t.Errorf("TotalTokenCount = %d, want 300", out.UsageMetadata.TotalTokenCount)
	}
}

func TestSerializeResponse_ToolUseBlock_EmptyInput(t *testing.T) {
	resp := &bamboo.Response{
		StopReason: bamboo.FinishReasonToolUse,
		Content: []bamboo.ContentBlock{
			&bamboo.ToolUseBlock{
				Type:  bamboo.ContentBlockToolUse,
				ID:    "call-1",
				Name:  "no_args_tool",
				Input: nil, // 空 input
			},
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse error = %v", err)
	}

	var out geminiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	fc := out.Candidates[0].Content.Parts[0].FunctionCall
	if fc == nil {
		t.Fatal("functionCall is nil")
	}
	// 空 input 应该被设为 {}
	var args map[string]any
	if err := json.Unmarshal(fc.Args, &args); err != nil {
		t.Fatalf("empty args should be valid JSON {}: %v", err)
	}
}

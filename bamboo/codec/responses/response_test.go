package responses

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
		CreatedAt:  1700000000,
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

	var out responsesOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.ID != "resp_001" {
		t.Errorf("ID = %q", out.ID)
	}
	if out.Object != "response" {
		t.Errorf("Object = %q, want %q", out.Object, "response")
	}
	if out.CreatedAt != 1700000000 {
		t.Errorf("CreatedAt = %d", out.CreatedAt)
	}
	if out.Model != "gpt-4o" {
		t.Errorf("Model = %q", out.Model)
	}
	if out.Status != "completed" {
		t.Errorf("Status = %q, want %q", out.Status, "completed")
	}
	if len(out.Output) != 1 {
		t.Fatalf("Output len = %d, want 1", len(out.Output))
	}

	// 验证 message 项目
	msgItem := out.Output[0]
	if msgItem.Type != "message" {
		t.Errorf("Output[0].Type = %q, want %q", msgItem.Type, "message")
	}
	if msgItem.Role != "assistant" {
		t.Errorf("Role = %q", msgItem.Role)
	}
	if msgItem.Status != "completed" {
		t.Errorf("Item Status = %q", msgItem.Status)
	}
	if len(msgItem.Content) != 1 {
		t.Fatalf("Content len = %d", len(msgItem.Content))
	}
	if msgItem.Content[0].Type != "output_text" {
		t.Errorf("Content[0].Type = %q", msgItem.Content[0].Type)
	}
	if msgItem.Content[0].Text != "Hello!" {
		t.Errorf("Content[0].Text = %q", msgItem.Content[0].Text)
	}

	// 验证 usage
	if out.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 5 {
		t.Errorf("Usage.OutputTokens = %d", out.Usage.OutputTokens)
	}
}

func TestSerializeResponse_UsageDetailsIncludeZeroRequiredFields(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_usage_zero",
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

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw response error = %v", err)
	}
	usage, ok := raw["usage"].(map[string]any)
	if !ok {
		t.Fatalf("missing usage: %s", data)
	}
	inputDetails, ok := usage["input_tokens_details"].(map[string]any)
	if !ok {
		t.Fatalf("missing input_tokens_details: %v", usage)
	}
	if value, ok := inputDetails["cached_tokens"].(float64); !ok || value != 0 {
		t.Fatalf("cached_tokens = %v (%T), want explicit 0", inputDetails["cached_tokens"], inputDetails["cached_tokens"])
	}
	outputDetails, ok := usage["output_tokens_details"].(map[string]any)
	if !ok {
		t.Fatalf("missing output_tokens_details: %v", usage)
	}
	if value, ok := outputDetails["reasoning_tokens"].(float64); !ok || value != 0 {
		t.Fatalf("reasoning_tokens = %v (%T), want explicit 0", outputDetails["reasoning_tokens"], outputDetails["reasoning_tokens"])
	}
}

func TestSerializeResponse_FunctionCall(t *testing.T) {
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

	var out responsesOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.Status != "completed" {
		t.Errorf("Status = %q, want %q", out.Status, "completed")
	}

	// 查找 function_call 项目
	var fcItem *outputItem
	for i := range out.Output {
		if out.Output[i].Type == "function_call" {
			fcItem = &out.Output[i]
			break
		}
	}
	if fcItem == nil {
		t.Fatal("no function_call item in output")
	}
	if fcItem.CallID != "call_abc" {
		t.Errorf("CallID = %q", fcItem.CallID)
	}
	if fcItem.Name != "get_weather" {
		t.Errorf("Name = %q", fcItem.Name)
	}
	// arguments 应为有效 JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(fcItem.Arguments), &args); err != nil {
		t.Errorf("Arguments not valid JSON: %v", err)
	}
}

func TestSerializeResponse_Reasoning(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "resp_003",
		Model:      "o3",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewThinkingBlockWithProvider("Let me think...", "sig_abc", bamboo.SignatureProviderOpenAIResponses),
			bamboo.NewTextBlock("The answer is 42."),
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

	var out responsesOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	// reasoning 项目应在 output 中
	var reasoningItem *outputItem
	var msgItem *outputItem
	for i := range out.Output {
		switch out.Output[i].Type {
		case "reasoning":
			reasoningItem = &out.Output[i]
		case "message":
			msgItem = &out.Output[i]
		}
	}

	if reasoningItem == nil {
		t.Fatal("no reasoning item in output")
	}
	// content 承载原始思考全文（reasoning_text 轨道）
	if len(reasoningItem.Content) != 1 {
		t.Fatalf("reasoning Content len = %d, want 1", len(reasoningItem.Content))
	}
	if reasoningItem.Content[0].Type != "reasoning_text" {
		t.Errorf("reasoning Content Type = %q, want %q", reasoningItem.Content[0].Type, "reasoning_text")
	}
	if reasoningItem.Content[0].Text != "Let me think..." {
		t.Errorf("reasoning Content Text = %q, want %q", reasoningItem.Content[0].Text, "Let me think...")
	}
	// summary 承载启发式提取的摘要（短文本提取结果与原文一致）
	if len(reasoningItem.Summary) != 1 {
		t.Fatalf("reasoning Summary len = %d, want 1", len(reasoningItem.Summary))
	}
	if reasoningItem.Summary[0].Type != "summary_text" {
		t.Errorf("reasoning Summary Type = %q, want %q", reasoningItem.Summary[0].Type, "summary_text")
	}
	if reasoningItem.Summary[0].Text != "Let me think..." {
		t.Errorf("reasoning Summary Text = %q, want %q", reasoningItem.Summary[0].Text, "Let me think...")
	}
	// encrypted_content 透传 ThinkingBlock.Signature
	if reasoningItem.EncryptedContent != "sig_abc" {
		t.Errorf("reasoning EncryptedContent = %q, want %q", reasoningItem.EncryptedContent, "sig_abc")
	}

	if msgItem == nil {
		t.Fatal("no message item in output")
	}
	if msgItem.Content[0].Text != "The answer is 42." {
		t.Errorf("message Text = %q", msgItem.Content[0].Text)
	}
}

func TestSerializeResponse_MergedText(t *testing.T) {
	// 多个 TextBlock 应合并到一个 message 项目
	resp := &bamboo.Response{
		ID:         "resp_004",
		Model:      "gpt-4o",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("Hello "),
			bamboo.NewTextBlock("World!"),
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out responsesOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	// 应该只有一个 message 项目
	msgCount := 0
	for _, item := range out.Output {
		if item.Type == "message" {
			msgCount++
		}
	}
	if msgCount != 1 {
		t.Errorf("message items = %d, want 1", msgCount)
	}

	// 验证文本合并
	var msgItem *outputItem
	for i := range out.Output {
		if out.Output[i].Type == "message" {
			msgItem = &out.Output[i]
			break
		}
	}
	if msgItem.Content[0].Text != "Hello World!" {
		t.Errorf("merged Text = %q", msgItem.Content[0].Text)
	}
}

func TestSerializeResponse_StatusMapping(t *testing.T) {
	tests := []struct {
		reason bamboo.FinishReason
		want   string
	}{
		{bamboo.FinishReasonEndTurn, "completed"},
		{bamboo.FinishReasonMaxTokens, "incomplete"},
		{bamboo.FinishReasonToolUse, "completed"},
		{bamboo.FinishReasonStopSequence, "completed"},
	}

	for _, tt := range tests {
		got := mapStatus(tt.reason)
		if got != tt.want {
			t.Errorf("mapStatus(%q) = %q, want %q", tt.reason, got, tt.want)
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

	var out responsesOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	// 即使内容为空，也应有至少一个 message 项目
	if len(out.Output) == 0 {
		t.Error("Output should not be empty")
	}
}

func TestSerializeResponse_CombinedContent(t *testing.T) {
	// 同时有 text + thinking + tool_use
	resp := &bamboo.Response{
		ID:         "resp_006",
		Model:      "gpt-4o",
		StopReason: bamboo.FinishReasonToolUse,
		Content: []bamboo.ContentBlock{
			bamboo.NewThinkingBlock("Thinking...", ""),
			bamboo.NewTextBlock("Let me check."),
			bamboo.NewToolUseBlock("call_1", "search", map[string]any{"q": "test"}),
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out responsesOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	// output 应包含 reasoning + message + function_call
	types := make(map[string]int)
	for _, item := range out.Output {
		types[item.Type]++
	}
	if types["reasoning"] != 1 {
		t.Errorf("reasoning count = %d, want 1", types["reasoning"])
	}
	if types["message"] != 1 {
		t.Errorf("message count = %d, want 1", types["message"])
	}
	if types["function_call"] != 1 {
		t.Errorf("function_call count = %d, want 1", types["function_call"])
	}
}

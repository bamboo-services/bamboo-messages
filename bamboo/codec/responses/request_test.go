package responses

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

func TestParseRequest_InputString(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hello"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", req.Config.Model, "gpt-4o")
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != bamboo.RoleUser {
		t.Errorf("Role = %q, want %q", req.Messages[0].Role, bamboo.RoleUser)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", req.Messages[0].Content[0])
	}
	if tb.Text != "Hello" {
		t.Errorf("Text = %q, want %q", tb.Text, "Hello")
	}
}

func TestParseRequest_Instructions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"instructions": "You are helpful.",
		"input": "Hi"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.System != "You are helpful." {
		t.Errorf("System = %q", req.System)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra should preserve native instructions")
	}
	if req.Config.ProviderExtra["instructions"] != "You are helpful." {
		t.Errorf("ProviderExtra[instructions] = %v", req.Config.ProviderExtra["instructions"])
	}
}

func TestParseRequest_InputArrayUserMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello world"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d", len(req.Messages))
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleUser {
		t.Errorf("Role = %q", msg.Role)
	}
	tb, ok := msg.Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", msg.Content[0])
	}
	if tb.Text != "Hello world" {
		t.Errorf("Text = %q", tb.Text)
	}
}

func TestParseRequest_InputArrayAssistantMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Hi there"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleAssistant {
		t.Errorf("Role = %q", msg.Role)
	}
	tb, ok := msg.Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock")
	}
	if tb.Text != "Hi there" {
		t.Errorf("Text = %q", tb.Text)
	}
}

func TestParseRequest_InputArraySystemMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"instructions": "Base instructions.",
		"input": [
			{"type": "message", "role": "system", "content": [{"type": "input_text", "text": "Extra system"}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hi"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1 (system excluded)", len(req.Messages))
	}
	// system 应合并到 System 字段
	if req.System != "Base instructions.\n\nExtra system" {
		t.Errorf("System = %q", req.System)
	}
}

func TestParseRequest_FunctionCall(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call", "id": "fc_1", "call_id": "call_abc", "name": "get_weather", "arguments": "{\"city\":\"SF\"}"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleAssistant {
		t.Errorf("Role = %q", msg.Role)
	}
	toolUse, ok := msg.Content[0].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", msg.Content[0])
	}
	if toolUse.ID != "call_abc" {
		t.Errorf("ID = %q, want %q", toolUse.ID, "call_abc")
	}
	if toolUse.Name != "get_weather" {
		t.Errorf("Name = %q", toolUse.Name)
	}
}

func TestParseRequest_FunctionCallOutput(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call_output", "call_id": "call_abc", "output": "Sunny, 72F"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleUser {
		t.Errorf("Role = %q", msg.Role)
	}
	trBlock, ok := msg.Content[0].(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", msg.Content[0])
	}
	if trBlock.ToolUseID != "call_abc" {
		t.Errorf("ToolUseID = %q", trBlock.ToolUseID)
	}
	if trBlock.Content != "Sunny, 72F" {
		t.Errorf("Content = %q", trBlock.Content)
	}
}

func TestParseRequest_Reasoning(t *testing.T) {
	body := []byte(`{
		"model": "o3",
		"input": [
			{"type": "reasoning", "content": [{"type": "reasoning_text", "text": "Let me think..."}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "What is 2+2?"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	// 第一条消息是 assistant + ThinkingBlock
	if req.Messages[0].Role != bamboo.RoleAssistant {
		t.Errorf("Messages[0].Role = %q", req.Messages[0].Role)
	}
	thinking, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinking.Thinking != "Let me think..." {
		t.Errorf("Thinking = %q", thinking.Thinking)
	}
}

// TestParseRequest_ReasoningContentPreferred 验证 content（原始思考全文）
// 优先于 summary（有损摘要）被解析。
func TestParseRequest_ReasoningContentPreferred(t *testing.T) {
	body := []byte(`{
		"model": "o3",
		"input": [
			{"type": "reasoning", "summary": [{"type": "summary_text", "text": "摘要"}], "content": [{"type": "reasoning_text", "text": "完整原始思考"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	thinking, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinking.Thinking != "完整原始思考" {
		t.Errorf("Thinking = %q, want content text (preferred over summary)", thinking.Thinking)
	}
}

// TestParseRequest_ReasoningEncryptedContent 验证 encrypted_content 透传为
// ThinkingBlock.Signature，且无明文文本时仍生成消息（保留加密推理链）。
func TestParseRequest_ReasoningEncryptedContent(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5",
		"input": [
			{"type": "reasoning", "summary": [], "encrypted_content": "gAAAAAB_encrypted"},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "continue"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	thinking, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinking.Thinking != "" {
		t.Errorf("Thinking = %q, want empty", thinking.Thinking)
	}
	if thinking.Signature != "gAAAAAB_encrypted" {
		t.Errorf("Signature = %q, want encrypted_content passthrough", thinking.Signature)
	}
}

func TestParseRequest_Tools(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Weather?",
		"tools": [{
			"type": "function",
			"name": "get_weather",
			"description": "Get weather",
			"parameters": {
				"type": "object",
				"properties": {
					"city": {"type": "string", "description": "City name"}
				},
				"required": ["city"]
			}
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Config.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(req.Config.Tools))
	}
	tool := req.Config.Tools[0]
	if tool.Name != "get_weather" {
		t.Errorf("Name = %q", tool.Name)
	}
	if tool.Description != "Get weather" {
		t.Errorf("Description = %q", tool.Description)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("解析 InputSchema 失败: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("Schema Type = %v", schema["type"])
	}
}

func TestParseRequest_ReasoningConfig(t *testing.T) {
	body := []byte(`{
		"model": "o3",
		"input": "Think",
		"reasoning": {"effort": "high"}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ThinkingConfig == nil {
		t.Fatal("ThinkingConfig is nil")
	}
	if req.Config.ThinkingConfig.Effort != "high" {
		t.Errorf("Effort = %q, want %q", req.Config.ThinkingConfig.Effort, "high")
	}
}

func TestParseRequest_MaxOutputTokens(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"max_output_tokens": 4096
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", req.Config.MaxTokens)
	}
}

func TestParseRequest_TextFormat(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"text": {"format": {"type": "json_object"}}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ResponseFormat != "json_object" {
		t.Errorf("ResponseFormat = %q", req.Config.ResponseFormat)
	}
}

func TestParseRequest_ProviderExtra(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"previous_response_id": "resp_prev",
		"store": true,
		"truncation": "auto"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra is nil")
	}
	if v, ok := req.Config.ProviderExtra["previous_response_id"].(string); !ok || v != "resp_prev" {
		t.Errorf("previous_response_id = %v", req.Config.ProviderExtra["previous_response_id"])
	}
	if v, ok := req.Config.ProviderExtra["store"].(bool); !ok || !v {
		t.Errorf("store = %v", req.Config.ProviderExtra["store"])
	}
	if v, ok := req.Config.ProviderExtra["truncation"].(string); !ok || v != "auto" {
		t.Errorf("truncation = %v", req.Config.ProviderExtra["truncation"])
	}
}

func TestParseRequest_ToolChoiceString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"auto"`, "auto"},
		{`"none"`, "none"},
		{`"required"`, "required"},
		{`{"type":"function","name":"my_tool"}`, "forced"},
	}

	for _, tt := range tests {
		choice, err := parseToolChoice(json.RawMessage(tt.input))
		if err != nil {
			t.Errorf("parseToolChoice(%s) error = %v", tt.input, err)
		}
		if choice != tt.expected {
			t.Errorf("parseToolChoice(%s) = %q, want %q", tt.input, choice, tt.expected)
		}
	}
}

func TestParseRequest_Stream(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"stream": true
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if !req.IsStream {
		t.Errorf("IsStream = false, want true")
	}
}

func TestParseRequest_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)

	_, err := parseRequest(body)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	var bambooErr *pkgErrors.BambooError
	if !errors.As(err, &bambooErr) {
		t.Errorf("expected *pkgErrors.BambooError, got %T", err)
	}
	if bambooErr == nil {
		t.Fatalf("expected BambooError to be non-nil")
	}
	if bambooErr.Category != "下游" {
		t.Errorf("Category = %q, want %q", bambooErr.Category, "下游")
	}
	if bambooErr.Message == "" {
		t.Errorf("Message should not be empty")
	}
	if bambooErr.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0", bambooErr.StatusCode)
	}
}

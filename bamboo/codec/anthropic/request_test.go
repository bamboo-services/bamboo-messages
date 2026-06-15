package anthropic

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

func TestParseRequest_BasicWithMaxTokens(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"max_tokens": 2048,
		"temperature": 0.7
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q", req.Config.Model)
	}
	if req.Config.MaxTokens != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", req.Config.MaxTokens)
	}
	if req.Config.Temperature == nil || *req.Config.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", req.Config.Temperature)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != bamboo.RoleUser {
		t.Errorf("Role = %q", req.Messages[0].Role)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", req.Messages[0].Content[0])
	}
	if tb.Text != "Hello" {
		t.Errorf("Text = %q, want %q", tb.Text, "Hello")
	}
}

func TestParseRequest_MaxTokensDefault(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want default 4096", req.Config.MaxTokens)
	}
}

func TestParseRequest_SystemString(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": "You are a helpful assistant.",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.System != "You are a helpful assistant." {
		t.Errorf("System = %q", req.System)
	}
}

func TestParseRequest_SystemArray(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": [
			{"type": "text", "text": "You are helpful."},
			{"type": "text", "text": "Be concise."}
		],
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	want := "You are helpful.\n\nBe concise."
	if req.System != want {
		t.Errorf("System = %q, want %q", req.System, want)
	}
}

func TestParseRequest_ToolUseContentBlock(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "assistant",
			"content": [
				{"type": "text", "text": "Let me check."},
				{"type": "tool_use", "id": "call_abc", "name": "get_weather", "input": {"city": "SF"}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleAssistant {
		t.Errorf("Role = %q", msg.Role)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("Content len = %d, want 2", len(msg.Content))
	}
	toolUse, ok := msg.Content[1].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", msg.Content[1])
	}
	if toolUse.ID != "call_abc" {
		t.Errorf("ID = %q", toolUse.ID)
	}
	if toolUse.Name != "get_weather" {
		t.Errorf("Name = %q", toolUse.Name)
	}
	// input should be valid JSON
	var input map[string]any
	if err := json.Unmarshal(toolUse.Input, &input); err != nil {
		t.Errorf("Input not valid JSON: %v", err)
	}
	if input["city"] != "SF" {
		t.Errorf("Input.city = %v", input["city"])
	}
}

func TestParseRequest_ToolResultContentBlock(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "tool_result", "tool_use_id": "call_abc", "content": "Sunny, 72F"}
			]
		}]
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

func TestParseRequest_ToolResultWithError(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "tool_result", "tool_use_id": "call_err", "content": "Tool failed", "is_error": true}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	trBlock, ok := req.Messages[0].Content[0].(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock")
	}
	if !trBlock.IsError {
		t.Errorf("IsError = false, want true")
	}
}

func TestParseRequest_ThinkingContentBlock(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "assistant",
			"content": [
				{"type": "thinking", "thinking": "Let me think...", "signature": "sig_abc"}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	thinkBlock, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinkBlock.Thinking != "Let me think..." {
		t.Errorf("Thinking = %q", thinkBlock.Thinking)
	}
	if thinkBlock.Signature != "sig_abc" {
		t.Errorf("Signature = %q", thinkBlock.Signature)
	}
}

func TestParseRequest_ImageBase64(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "What's this?"},
				{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo="}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	imgBlock, ok := req.Messages[0].Content[1].(*bamboo.ImageBlock)
	if !ok {
		t.Fatalf("expected *ImageBlock, got %T", req.Messages[0].Content[1])
	}
	if imgBlock.Source.Type != "base64" {
		t.Errorf("Source.Type = %q, want %q", imgBlock.Source.Type, "base64")
	}
	if imgBlock.Source.MediaType != "image/png" {
		t.Errorf("MediaType = %q", imgBlock.Source.MediaType)
	}
	if imgBlock.Source.Data != "iVBORw0KGgo=" {
		t.Errorf("Data = %q", imgBlock.Source.Data)
	}
}

func TestParseRequest_ImageURL(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "image", "source": {"type": "url", "url": "https://example.com/img.png"}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	imgBlock, ok := req.Messages[0].Content[0].(*bamboo.ImageBlock)
	if !ok {
		t.Fatalf("expected *ImageBlock")
	}
	if imgBlock.Source.Type != "url" {
		t.Errorf("Source.Type = %q, want %q", imgBlock.Source.Type, "url")
	}
	if imgBlock.Source.URL != "https://example.com/img.png" {
		t.Errorf("URL = %q", imgBlock.Source.URL)
	}
}

func TestParseRequest_ToolChoiceMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"type":"auto"}`, "auto"},
		{`{"type":"any"}`, "required"},
		{`{"type":"none"}`, "none"},
		{`{"type":"tool","name":"my_tool"}`, "forced"},
	}

	for _, tt := range tests {
		got := parseToolChoice(json.RawMessage(tt.input))
		if got != tt.expected {
			t.Errorf("parseToolChoice(%s) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseRequest_ThinkingAdaptive(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"thinking": {"type": "adaptive"},
		"messages": [{"role": "user", "content": "Think"}]
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

func TestParseRequest_ThinkingEnabled(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"thinking": {"type": "enabled", "budget_tokens": 10000},
		"messages": [{"role": "user", "content": "Think"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ThinkingConfig == nil {
		t.Fatal("ThinkingConfig is nil")
	}
	if req.Config.ThinkingConfig.Effort != "medium" {
		t.Errorf("Effort = %q, want %q", req.Config.ThinkingConfig.Effort, "medium")
	}
}

func TestParseRequest_MetadataUserID(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"metadata": {"user_id": "user-123"},
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", req.Config.UserID, "user-123")
	}
}

func TestParseRequest_TopKProviderExtra(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"top_k": 40,
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra is nil")
	}
	topK, ok := req.Config.ProviderExtra["top_k"].(int64)
	if !ok || topK != 40 {
		t.Errorf("top_k = %v, want 40", req.Config.ProviderExtra["top_k"])
	}
}

func TestParseRequest_Stream(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"stream": true,
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if !req.IsStream {
		t.Errorf("IsStream = false, want true")
	}
}

func TestParseRequest_Tools(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Weather?"}],
		"tools": [{
			"name": "get_weather",
			"description": "Get weather",
			"input_schema": {
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
	if tool.InputSchema.Type != "object" {
		t.Errorf("InputSchema.Type = %q", tool.InputSchema.Type)
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "city" {
		t.Errorf("Required = %v", tool.InputSchema.Required)
	}
}

func TestParseRequest_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)

	_, err := parseRequest(body)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	var codecErr *codec.CodecError
	if !errors.As(err, &codecErr) {
		t.Errorf("expected *codec.CodecError, got %T", err)
	}
	if codecErr.Type != codec.ErrInvalidRequest {
		t.Errorf("Type = %q, want %q", codecErr.Type, codec.ErrInvalidRequest)
	}
}

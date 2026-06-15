package openai

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

func TestParseRequest_Minimal(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Hello"}
		]
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
	if req.IsStream {
		t.Errorf("IsStream = true, want false")
	}
}

func TestParseRequest_SystemExtraction(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "system", "content": "Be concise."},
			{"role": "user", "content": "Hi"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.System != "You are helpful.\n\nBe concise." {
		t.Errorf("System = %q", req.System)
	}
	if len(req.Messages) != 1 {
		t.Errorf("Messages len = %d, want 1 (system excluded)", len(req.Messages))
	}
}

func TestParseRequest_ImageDataURL(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "What's this?"},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgo="}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if len(msg.Content) != 2 {
		t.Fatalf("Content blocks len = %d, want 2", len(msg.Content))
	}

	imgBlock, ok := msg.Content[1].(*bamboo.ImageBlock)
	if !ok {
		t.Fatalf("expected *ImageBlock, got %T", msg.Content[1])
	}
	if imgBlock.Source.Type != "base64" {
		t.Errorf("Source.Type = %q, want %q", imgBlock.Source.Type, "base64")
	}
	if imgBlock.Source.MediaType != "image/png" {
		t.Errorf("MediaType = %q, want %q", imgBlock.Source.MediaType, "image/png")
	}
	if imgBlock.Source.Data != "iVBORw0KGgo=" {
		t.Errorf("Data = %q", imgBlock.Source.Data)
	}
}

func TestParseRequest_ImageURL(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "image_url", "image_url": {"url": "https://example.com/img.png"}}
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
}

func TestParseRequest_AssistantToolCalls(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{
			"role": "assistant",
			"content": null,
			"tool_calls": [{
				"id": "call_abc",
				"type": "function",
				"function": {"name": "get_weather", "arguments": "{\"city\":\"SF\"}"}
			}]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleAssistant {
		t.Errorf("Role = %q, want %q", msg.Role, bamboo.RoleAssistant)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(msg.Content))
	}
	toolUse, ok := msg.Content[0].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", msg.Content[0])
	}
	if toolUse.ID != "call_abc" {
		t.Errorf("ID = %q", toolUse.ID)
	}
	if toolUse.Name != "get_weather" {
		t.Errorf("Name = %q", toolUse.Name)
	}
}

func TestParseRequest_ToolMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{
			"role": "tool",
			"content": "Sunny, 72F",
			"tool_call_id": "call_abc"
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleUser {
		t.Errorf("Role = %q, want %q", msg.Role, bamboo.RoleUser)
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

func TestParseRequest_ToolChoiceMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"auto"`, "auto"},
		{`"none"`, "none"},
		{`"required"`, "required"},
		{`{"type":"function","function":{"name":"my_tool"}}`, "forced"},
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

func TestParseRequest_MaxCompletionTokens(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"max_completion_tokens": 500,
		"max_tokens": 1000
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.MaxTokens != 500 {
		t.Errorf("MaxTokens = %d, want 500 (max_completion_tokens takes priority)", req.Config.MaxTokens)
	}
}

func TestParseRequest_ReasoningEffort(t *testing.T) {
	body := []byte(`{
		"model": "o1",
		"messages": [{"role": "user", "content": "Think"}],
		"reasoning_effort": "high"
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

func TestParseRequest_ProviderExtra(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"frequency_penalty": 0.5,
		"presence_penalty": 0.3,
		"seed": 42
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra is nil")
	}
	if v, ok := req.Config.ProviderExtra["frequency_penalty"].(float64); !ok || v != 0.5 {
		t.Errorf("frequency_penalty = %v, want 0.5", req.Config.ProviderExtra["frequency_penalty"])
	}
	if v, ok := req.Config.ProviderExtra["seed"].(int64); !ok || v != 42 {
		t.Errorf("seed = %v, want 42", req.Config.ProviderExtra["seed"])
	}
}

func TestParseRequest_Stream(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
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
	var codecErr *codec.CodecError
	if !errors.As(err, &codecErr) {
		t.Errorf("expected *codec.CodecError, got %T", err)
	}
	if codecErr.Type != codec.ErrInvalidRequest {
		t.Errorf("Type = %q, want %q", codecErr.Type, codec.ErrInvalidRequest)
	}
}

func TestParseRequest_Stop(t *testing.T) {
	// stop as string
	stops, err := parseStop(json.RawMessage(`"STOP"`))
	if err != nil || len(stops) != 1 || stops[0] != "STOP" {
		t.Errorf("parseStop(string) = %v, err = %v", stops, err)
	}

	// stop as array
	stops, err = parseStop(json.RawMessage(`["A","B"]`))
	if err != nil || len(stops) != 2 {
		t.Errorf("parseStop(array) = %v, err = %v", stops, err)
	}
}

func TestParseRequest_Tools(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Weather?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get weather",
				"parameters": {
					"type": "object",
					"properties": {
						"city": {"type": "string", "description": "City name"}
					},
					"required": ["city"]
				}
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
}

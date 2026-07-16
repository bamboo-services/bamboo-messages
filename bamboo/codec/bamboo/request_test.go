package bamboo

import (
	"errors"
	"testing"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

func TestParseRequest_FullEnvelope(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Hello"}]}
		],
		"system": "You are a helpful assistant.",
		"config": {
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 2048,
			"temperature": 0.7
		},
		"stream": true
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.System != "You are a helpful assistant." {
		t.Errorf("System = %q", req.System)
	}
	if !req.IsStream {
		t.Errorf("IsStream = false, want true")
	}
	if req.Config == nil {
		t.Fatal("Config is nil")
	}
	if req.Config.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Config.Model = %q", req.Config.Model)
	}
	if req.Config.MaxTokens != 2048 {
		t.Errorf("Config.MaxTokens = %d, want 2048", req.Config.MaxTokens)
	}
	if req.Config.Temperature == nil || *req.Config.Temperature != 0.7 {
		t.Errorf("Config.Temperature = %v, want 0.7", req.Config.Temperature)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != bmbamboo.RoleUser {
		t.Errorf("Role = %q", req.Messages[0].Role)
	}
	tb, ok := req.Messages[0].Content[0].(*bmbamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", req.Messages[0].Content[0])
	}
	if tb.Text != "Hello" {
		t.Errorf("Text = %q, want %q", tb.Text, "Hello")
	}
}

func TestParseRequest_ConfigMissingDefaultsToZero(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Hi"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config == nil {
		t.Fatal("Config should be non-nil even when missing in body")
	}
	if req.Config.Model != "" {
		t.Errorf("Config.Model = %q, want empty", req.Config.Model)
	}
	if req.Config.MaxTokens != 0 {
		t.Errorf("Config.MaxTokens = %d, want 0", req.Config.MaxTokens)
	}
}

func TestParseRequest_ToolUseContentBlock(t *testing.T) {
	body := []byte(`{
		"messages": [{
			"role": "assistant",
			"content": [
				{"type": "tool_use", "id": "call_abc", "name": "get_weather", "input": {"city": "SF"}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	tub, ok := req.Messages[0].Content[0].(*bmbamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", req.Messages[0].Content[0])
	}
	if tub.ID != "call_abc" {
		t.Errorf("ID = %q", tub.ID)
	}
	if tub.Name != "get_weather" {
		t.Errorf("Name = %q", tub.Name)
	}
}

func TestParseRequest_ThinkingContentBlock(t *testing.T) {
	body := []byte(`{
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
	tb, ok := req.Messages[0].Content[0].(*bmbamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if tb.Thinking != "Let me think..." {
		t.Errorf("Thinking = %q", tb.Thinking)
	}
	if tb.Signature != "sig_abc" {
		t.Errorf("Signature = %q", tb.Signature)
	}
}

func TestParseRequest_ImageAndDocumentBlocks(t *testing.T) {
	body := []byte(`{
		"messages": [{
			"role": "user",
			"content": [
				{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo="}},
				{"type": "document", "source": {"type": "url", "url": "https://example.com/doc.pdf"}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	img, ok := req.Messages[0].Content[0].(*bmbamboo.ImageBlock)
	if !ok {
		t.Fatalf("expected *ImageBlock, got %T", req.Messages[0].Content[0])
	}
	if img.Source.Type != "base64" || img.Source.MediaType != "image/png" || img.Source.Data != "iVBORw0KGgo=" {
		t.Errorf("ImageBlock.Source = %+v", img.Source)
	}
	doc, ok := req.Messages[0].Content[1].(*bmbamboo.DocumentBlock)
	if !ok {
		t.Fatalf("expected *DocumentBlock, got %T", req.Messages[0].Content[1])
	}
	if doc.Source.Type != "url" || doc.Source.URL != "https://example.com/doc.pdf" {
		t.Errorf("DocumentBlock.Source = %+v", doc.Source)
	}
}

func TestParseRequest_ToolResultBlock(t *testing.T) {
	body := []byte(`{
		"messages": [{
			"role": "user",
			"content": [
				{"type": "tool_result", "tool_use_id": "call_abc", "content": "Sunny, 72F", "is_error": false}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	tr, ok := req.Messages[0].Content[0].(*bmbamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", req.Messages[0].Content[0])
	}
	if tr.ToolUseID != "call_abc" {
		t.Errorf("ToolUseID = %q", tr.ToolUseID)
	}
	if tr.Content != "Sunny, 72F" {
		t.Errorf("Content = %q", tr.Content)
	}
}

func TestParseRequest_RedactedThinkingBlock(t *testing.T) {
	body := []byte(`{
		"messages": [{
			"role": "assistant",
			"content": [
				{"type": "redacted_thinking", "data": "encrypted-blob-xyz"}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	rt, ok := req.Messages[0].Content[0].(*bmbamboo.RedactedThinkingBlock)
	if !ok {
		t.Fatalf("expected *RedactedThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if rt.Data != "encrypted-blob-xyz" {
		t.Errorf("Data = %q", rt.Data)
	}
}

func TestParseRequest_IsStreamPropagation(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		stream bool
	}{
		{
			name:   "stream true",
			body:   `{"messages":[],"stream":true}`,
			stream: true,
		},
		{
			name:   "stream false explicit",
			body:   `{"messages":[],"stream":false}`,
			stream: false,
		},
		{
			name:   "stream missing defaults false",
			body:   `{"messages":[]}`,
			stream: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := parseRequest([]byte(tt.body))
			if err != nil {
				t.Fatalf("parseRequest() error = %v", err)
			}
			if req.IsStream != tt.stream {
				t.Errorf("IsStream = %v, want %v", req.IsStream, tt.stream)
			}
		})
	}
}

func TestParseRequest_ReasoningIDPropagation(t *testing.T) {
	body := []byte(`{
		"messages": [{
			"role": "assistant",
			"reasoning_id": "rs_abc123",
			"content": [{"type": "text", "text": "ok"}]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Messages[0].ReasoningID != "rs_abc123" {
		t.Errorf("ReasoningID = %q, want %q", req.Messages[0].ReasoningID, "rs_abc123")
	}
}

func TestParseRequest_EmptyBodyError(t *testing.T) {
	// 空对象 {} 是合法 JSON，应解析成功（messages 为空切片）
	req, err := parseRequest([]byte(`{}`))
	if err != nil {
		t.Fatalf("parseRequest({}) error = %v", err)
	}
	if len(req.Messages) != 0 {
		t.Errorf("Messages len = %d, want 0", len(req.Messages))
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
		t.Fatalf("expected *pkgErrors.BambooError, got %T (%v)", err, err)
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

func TestParseRequest_ProviderExtraPassthrough(t *testing.T) {
	body := []byte(`{
		"messages": [],
		"config": {
			"model": "test-model",
			"provider_extra": {"custom_key": "custom_value", "top_k": 40}
		}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra is nil")
	}
	if req.Config.ProviderExtra["custom_key"] != "custom_value" {
		t.Errorf("custom_key = %v", req.Config.ProviderExtra["custom_key"])
	}
}

// TestCodec_ParseRequest_Delegation 验证 Codec.ParseRequest 委托到 parseRequest。
func TestCodec_ParseRequest_Delegation(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	req, err := Codec.ParseRequest(body)
	if err != nil {
		t.Fatalf("Codec.ParseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Errorf("Messages len = %d, want 1", len(req.Messages))
	}
}

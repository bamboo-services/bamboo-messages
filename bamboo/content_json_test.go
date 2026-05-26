package bamboo

import (
	"encoding/json"
	"testing"
)

// ---- ContentBlock JSON 往返测试 ----

func TestContentBlockText_JSONRoundtrip(t *testing.T) {
	original := NewTextBlock("Hello, 世界！")
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentBlock
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Type != ContentBlockText {
		t.Errorf("往返后 Type 不匹配: 期望 %s，实际 %s", ContentBlockText, parsed.Type)
	}
	if parsed.Text != original.Text {
		t.Errorf("往返后 Text 不匹配: 期望 %s，实际 %s", original.Text, parsed.Text)
	}
}

func TestContentBlockThinking_JSONRoundtrip(t *testing.T) {
	original := NewThinkingBlock("推理过程", "sig_xyz")
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentBlock
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Thinking != "推理过程" {
		t.Errorf("往返后 Thinking 不匹配")
	}
	if parsed.Signature != "sig_xyz" {
		t.Errorf("往返后 Signature 不匹配")
	}
}

func TestContentBlockToolUse_JSONRoundtrip(t *testing.T) {
	input := map[string]any{"query": "test"}
	original := NewToolUseBlock("toolu_100", "search", input)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentBlock
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.ID != "toolu_100" {
		t.Errorf("往返后 ID 不匹配")
	}
	if parsed.Name != "search" {
		t.Errorf("往返后 Name 不匹配")
	}
	if parsed.Input == nil {
		t.Fatal("往返后 Input 不应为 nil")
	}
}

func TestContentBlockToolResult_JSONRoundtrip(t *testing.T) {
	original := NewToolResultBlock("toolu_100", "搜索结果", false)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentBlock
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.ToolUseID != "toolu_100" {
		t.Errorf("往返后 ToolUseID 不匹配")
	}
	if parsed.ResultContent != "搜索结果" {
		t.Errorf("往返后 ResultContent 不匹配")
	}
}

func TestContentBlockImage_JSONRoundtrip(t *testing.T) {
	original := NewImageBlock(ContentSource{
		Type:      "base64",
		MediaType: "image/jpeg",
		Data:      "/9j/4AAQSkZJRg==",
	})

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentBlock
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Type != ContentBlockImage {
		t.Errorf("往返后 Type 不匹配")
	}
	if parsed.Source == nil {
		t.Fatal("往返后 Source 不应为 nil")
	}
	if parsed.Source.MediaType != "image/jpeg" {
		t.Errorf("往返后 Source.MediaType 不匹配")
	}
}

func TestContentBlockDocument_JSONRoundtrip(t *testing.T) {
	original := NewDocumentBlock(ContentSource{
		Type:    "text",
		Content: "这是一份文档内容",
	})

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentBlock
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Source.Content != "这是一份文档内容" {
		t.Errorf("往返后 Source.Content 不匹配")
	}
}

// ---- ContentSource 独立测试 ----

func TestContentSource_Base64_JSONRoundtrip(t *testing.T) {
	original := ContentSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      "base64data",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentSource
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Type != "base64" || parsed.MediaType != "image/png" || parsed.Data != "base64data" {
		t.Errorf("往返后字段不匹配: %+v", parsed)
	}
}

func TestContentSource_URL_JSONRoundtrip(t *testing.T) {
	original := ContentSource{
		Type: "url",
		URL:  "https://example.com/image.png",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed ContentSource
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.URL != "https://example.com/image.png" {
		t.Errorf("往返后 URL 不匹配")
	}
}

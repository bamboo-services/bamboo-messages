package bamboo

import (
	"encoding/json"
	"testing"
)

// ---- JSON 字节兼容性测试 ----

func TestTextBlock_ByteCompatibility(t *testing.T) {
	// 1. 创建 TextBlock
	block := NewTextBlock("hello")

	// 2. 序列化为 JSON
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	// 3. 验证字节精确匹配
	expected := `{"type":"text","text":"hello"}`
	if string(data) != expected {
		t.Errorf("JSON 字节不兼容\n期望: %s\n实际: %s", expected, string(data))
	}

	// 4. 通过 ContentBlocks 反序列化回来
	wrapper := `[` + string(data) + `]`
	var parsed ContentBlocks
	if err := json.Unmarshal([]byte(wrapper), &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("期望 1 个 block，实际 %d 个", len(parsed))
	}

	// 5. 验证 BlockType 和 Text 内容
	if parsed[0].BlockType() != ContentBlockText {
		t.Errorf("BlockType 不匹配: 期望 %s，实际 %s", ContentBlockText, parsed[0].BlockType())
	}
	tb, ok := parsed[0].(*TextBlock)
	if !ok {
		t.Fatal("类型断言为 *TextBlock 失败")
	}
	if tb.Text != "hello" {
		t.Errorf("Text 不匹配: 期望 hello，实际 %s", tb.Text)
	}
}

// ---- ContentBlock JSON 往返测试 ----

func TestContentBlockText_JSONRoundtrip(t *testing.T) {
	original := NewTextBlock("Hello, 世界！")
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	// 使用 ContentBlocks 包装类型反序列化
	wrapper := `[` + string(data) + `]`
	var parsed ContentBlocks
	if err := json.Unmarshal([]byte(wrapper), &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("期望 1 个 block，实际 %d 个", len(parsed))
	}

	if parsed[0].BlockType() != ContentBlockText {
		t.Errorf("往返后 Type 不匹配: 期望 %s，实际 %s", ContentBlockText, parsed[0].BlockType())
	}
	tb, ok := parsed[0].(*TextBlock)
	if !ok {
		t.Fatal("类型断言为 *TextBlock 失败")
	}
	ob, _ := original.(*TextBlock)
	if tb.Text != ob.Text {
		t.Errorf("往返后 Text 不匹配: 期望 %s，实际 %s", ob.Text, tb.Text)
	}
}

func TestContentBlockThinking_JSONRoundtrip(t *testing.T) {
	original := NewThinkingBlock("推理过程", "sig_xyz")
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	wrapper := `[` + string(data) + `]`
	var parsed ContentBlocks
	if err := json.Unmarshal([]byte(wrapper), &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("期望 1 个 block，实际 %d 个", len(parsed))
	}

	tb, ok := parsed[0].(*ThinkingBlock)
	if !ok {
		t.Fatal("类型断言为 *ThinkingBlock 失败")
	}
	if tb.Thinking != "推理过程" {
		t.Errorf("往返后 Thinking 不匹配")
	}
	if tb.Signature != "sig_xyz" {
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

	wrapper := `[` + string(data) + `]`
	var parsed ContentBlocks
	if err := json.Unmarshal([]byte(wrapper), &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("期望 1 个 block，实际 %d 个", len(parsed))
	}

	tb, ok := parsed[0].(*ToolUseBlock)
	if !ok {
		t.Fatal("类型断言为 *ToolUseBlock 失败")
	}
	if tb.ID != "toolu_100" {
		t.Errorf("往返后 ID 不匹配")
	}
	if tb.Name != "search" {
		t.Errorf("往返后 Name 不匹配")
	}
	if tb.Input == nil {
		t.Fatal("往返后 Input 不应为 nil")
	}
}

func TestContentBlockToolResult_JSONRoundtrip(t *testing.T) {
	original := NewToolResultBlock("toolu_100", "搜索结果", false)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	wrapper := `[` + string(data) + `]`
	var parsed ContentBlocks
	if err := json.Unmarshal([]byte(wrapper), &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("期望 1 个 block，实际 %d 个", len(parsed))
	}

	tb, ok := parsed[0].(*ToolResultBlock)
	if !ok {
		t.Fatal("类型断言为 *ToolResultBlock 失败")
	}
	if tb.ToolUseID != "toolu_100" {
		t.Errorf("往返后 ToolUseID 不匹配")
	}
	if tb.Content != "搜索结果" {
		t.Errorf("往返后 Content 不匹配")
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

	wrapper := `[` + string(data) + `]`
	var parsed ContentBlocks
	if err := json.Unmarshal([]byte(wrapper), &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("期望 1 个 block，实际 %d 个", len(parsed))
	}

	if parsed[0].BlockType() != ContentBlockImage {
		t.Errorf("往返后 Type 不匹配")
	}
	ib, ok := parsed[0].(*ImageBlock)
	if !ok {
		t.Fatal("类型断言为 *ImageBlock 失败")
	}
	if ib.Source == nil {
		t.Fatal("往返后 Source 不应为 nil")
	}
	if ib.Source.MediaType != "image/jpeg" {
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

	wrapper := `[` + string(data) + `]`
	var parsed ContentBlocks
	if err := json.Unmarshal([]byte(wrapper), &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("期望 1 个 block，实际 %d 个", len(parsed))
	}

	db, ok := parsed[0].(*DocumentBlock)
	if !ok {
		t.Fatal("类型断言为 *DocumentBlock 失败")
	}
	if db.Source.Content != "这是一份文档内容" {
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

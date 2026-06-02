package bamboo

import (
	"encoding/json"
	"testing"
)

// ---- ContentBlock 构造函数测试 ----

func TestNewTextBlock(t *testing.T) {
	block := NewTextBlock("你好世界")
	if block.BlockType() != ContentBlockText {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockText, block.BlockType())
	}
	tb, ok := block.(*TextBlock)
	if !ok {
		t.Fatal("类型断言为 *TextBlock 失败")
	}
	if tb.Text != "你好世界" {
		t.Errorf("期望 Text=你好世界，实际 Text=%s", tb.Text)
	}
}

func TestNewThinkingBlock(t *testing.T) {
	block := NewThinkingBlock("让我想想...", "sig_abc123")
	if block.BlockType() != ContentBlockThinking {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockThinking, block.BlockType())
	}
	tb, ok := block.(*ThinkingBlock)
	if !ok {
		t.Fatal("类型断言为 *ThinkingBlock 失败")
	}
	if tb.Thinking != "让我想想..." {
		t.Errorf("期望 Thinking=让我想想...，实际 Thinking=%s", tb.Thinking)
	}
	if tb.Signature != "sig_abc123" {
		t.Errorf("期望 Signature=sig_abc123，实际 Signature=%s", tb.Signature)
	}
}

func TestNewToolUseBlock(t *testing.T) {
	input := map[string]any{"city": "Tokyo", "unit": "celsius"}
	block := NewToolUseBlock("toolu_001", "get_weather", input)

	if block.BlockType() != ContentBlockToolUse {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockToolUse, block.BlockType())
	}
	tb, ok := block.(*ToolUseBlock)
	if !ok {
		t.Fatal("类型断言为 *ToolUseBlock 失败")
	}
	if tb.ID != "toolu_001" {
		t.Errorf("期望 ID=toolu_001，实际 ID=%s", tb.ID)
	}
	if tb.Name != "get_weather" {
		t.Errorf("期望 Name=get_weather，实际 Name=%s", tb.Name)
	}
	var parsed map[string]any
	if err := json.Unmarshal(tb.Input, &parsed); err != nil {
		t.Fatalf("Input JSON 解析失败: %v", err)
	}
	if parsed["city"] != "Tokyo" {
		t.Errorf("期望 city=Tokyo，实际 city=%v", parsed["city"])
	}
}

func TestNewToolUseBlock_NilInput(t *testing.T) {
	block := NewToolUseBlock("toolu_002", "empty_tool", nil)
	tb, ok := block.(*ToolUseBlock)
	if !ok {
		t.Fatal("类型断言为 *ToolUseBlock 失败")
	}
	if string(tb.Input) != `{}` {
		t.Errorf("期望 nil input 序列化为 {}，实际为 %s", string(tb.Input))
	}
}

func TestNewToolResultBlock(t *testing.T) {
	block := NewToolResultBlock("toolu_001", "东京当前温度 25°C", false)

	if block.BlockType() != ContentBlockToolResult {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockToolResult, block.BlockType())
	}
	tb, ok := block.(*ToolResultBlock)
	if !ok {
		t.Fatal("类型断言为 *ToolResultBlock 失败")
	}
	if tb.ToolUseID != "toolu_001" {
		t.Errorf("期望 ToolUseID=toolu_001，实际 ToolUseID=%s", tb.ToolUseID)
	}
	if tb.Content != "东京当前温度 25°C" {
		t.Errorf("期望 Content=东京当前温度 25°C，实际 Content=%s", tb.Content)
	}
	if tb.IsError {
		t.Error("期望 IsError=false")
	}
}

func TestNewToolResultBlock_Error(t *testing.T) {
	block := NewToolResultBlock("toolu_003", "工具调用超时", true)
	tb, ok := block.(*ToolResultBlock)
	if !ok {
		t.Fatal("类型断言为 *ToolResultBlock 失败")
	}
	if !tb.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestNewImageBlock(t *testing.T) {
	source := ContentSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      "iVBORw0KGgo=",
	}
	block := NewImageBlock(source)

	if block.BlockType() != ContentBlockImage {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockImage, block.BlockType())
	}
	ib, ok := block.(*ImageBlock)
	if !ok {
		t.Fatal("类型断言为 *ImageBlock 失败")
	}
	if ib.Source == nil {
		t.Fatal("期望 Source 不为 nil")
	}
	if ib.Source.Type != "base64" {
		t.Errorf("期望 Source.Type=base64，实际 Source.Type=%s", ib.Source.Type)
	}
}

func TestNewDocumentBlock(t *testing.T) {
	source := ContentSource{
		Type: "url",
		URL:  "https://example.com/doc.pdf",
	}
	block := NewDocumentBlock(source)

	if block.BlockType() != ContentBlockDocument {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockDocument, block.BlockType())
	}
	db, ok := block.(*DocumentBlock)
	if !ok {
		t.Fatal("类型断言为 *DocumentBlock 失败")
	}
	if db.Source == nil {
		t.Fatal("期望 Source 不为 nil")
	}
	if db.Source.URL != "https://example.com/doc.pdf" {
		t.Errorf("期望 Source.URL 正确设置")
	}
}

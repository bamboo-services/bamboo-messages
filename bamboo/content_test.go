package bamboo

import (
	"encoding/json"
	"testing"
)

// ---- ContentBlock 构造函数测试 ----

func TestNewTextBlock(t *testing.T) {
	block := NewTextBlock("你好世界")
	if block.Type != ContentBlockText {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockText, block.Type)
	}
	if block.Text != "你好世界" {
		t.Errorf("期望 Text=你好世界，实际 Text=%s", block.Text)
	}
}

func TestNewThinkingBlock(t *testing.T) {
	block := NewThinkingBlock("让我想想...", "sig_abc123")
	if block.Type != ContentBlockThinking {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockThinking, block.Type)
	}
	if block.Thinking != "让我想想..." {
		t.Errorf("期望 Thinking=让我想想...，实际 Thinking=%s", block.Thinking)
	}
	if block.Signature != "sig_abc123" {
		t.Errorf("期望 Signature=sig_abc123，实际 Signature=%s", block.Signature)
	}
}

func TestNewToolUseBlock(t *testing.T) {
	input := map[string]any{"city": "Tokyo", "unit": "celsius"}
	block := NewToolUseBlock("toolu_001", "get_weather", input)

	if block.Type != ContentBlockToolUse {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockToolUse, block.Type)
	}
	if block.ID != "toolu_001" {
		t.Errorf("期望 ID=toolu_001，实际 ID=%s", block.ID)
	}
	if block.Name != "get_weather" {
		t.Errorf("期望 Name=get_weather，实际 Name=%s", block.Name)
	}
	var parsed map[string]any
	if err := json.Unmarshal(block.Input, &parsed); err != nil {
		t.Fatalf("Input JSON 解析失败: %v", err)
	}
	if parsed["city"] != "Tokyo" {
		t.Errorf("期望 city=Tokyo，实际 city=%v", parsed["city"])
	}
}

func TestNewToolUseBlock_NilInput(t *testing.T) {
	block := NewToolUseBlock("toolu_002", "empty_tool", nil)
	if string(block.Input) != `{}` {
		t.Errorf("期望 nil input 序列化为 {}，实际为 %s", string(block.Input))
	}
}

func TestNewToolResultBlock(t *testing.T) {
	block := NewToolResultBlock("toolu_001", "东京当前温度 25°C", false)

	if block.Type != ContentBlockToolResult {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockToolResult, block.Type)
	}
	if block.ToolUseID != "toolu_001" {
		t.Errorf("期望 ToolUseID=toolu_001，实际 ToolUseID=%s", block.ToolUseID)
	}
	if block.ResultContent != "东京当前温度 25°C" {
		t.Errorf("期望 ResultContent=东京当前温度 25°C，实际 ResultContent=%s", block.ResultContent)
	}
	if block.IsError {
		t.Error("期望 IsError=false")
	}
}

func TestNewToolResultBlock_Error(t *testing.T) {
	block := NewToolResultBlock("toolu_003", "工具调用超时", true)
	if !block.IsError {
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

	if block.Type != ContentBlockImage {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockImage, block.Type)
	}
	if block.Source == nil {
		t.Fatal("期望 Source 不为 nil")
	}
	if block.Source.Type != "base64" {
		t.Errorf("期望 Source.Type=base64，实际 Source.Type=%s", block.Source.Type)
	}
}

func TestNewDocumentBlock(t *testing.T) {
	source := ContentSource{
		Type: "url",
		URL:  "https://example.com/doc.pdf",
	}
	block := NewDocumentBlock(source)

	if block.Type != ContentBlockDocument {
		t.Errorf("期望 Type=%s，实际 Type=%s", ContentBlockDocument, block.Type)
	}
	if block.Source == nil {
		t.Fatal("期望 Source 不为 nil")
	}
	if block.Source.URL != "https://example.com/doc.pdf" {
		t.Errorf("期望 Source.URL 正确设置")
	}
}

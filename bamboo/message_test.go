package bamboo

import (
	"testing"
)

// ---- BambooMessage 构造函数测试 ----

func TestNewUserMessage(t *testing.T) {
	msg := NewUserMessage("你好")
	if msg.Role != RoleUser {
		t.Errorf("期望 Role=%s，实际 Role=%s", RoleUser, msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("期望 1 个 ContentBlock，实际 %d 个", len(msg.Content))
	}
	tb, ok := msg.Content[0].(*TextBlock)
	if !ok {
		t.Fatal("Content[0] 类型断言为 *TextBlock 失败")
	}
	if tb.Text != "你好" {
		t.Errorf("期望 Content[0].Text=你好，实际 %s", tb.Text)
	}
}

func TestNewAssistantMessage(t *testing.T) {
	msg := NewAssistantMessage("你好！有什么可以帮助你的吗？")
	if msg.Role != RoleAssistant {
		t.Errorf("期望 Role=%s", RoleAssistant)
	}
	tb, ok := msg.Content[0].(*TextBlock)
	if !ok {
		t.Fatal("Content[0] 类型断言为 *TextBlock 失败")
	}
	if tb.Text != "你好！有什么可以帮助你的吗？" {
		t.Errorf("Content[0].Text 不匹配")
	}
}

func TestNewUserMessageBlocks(t *testing.T) {
	blocks := []ContentBlock{
		NewTextBlock("看这张图片"),
		NewImageBlock(ContentSource{Type: "url", URL: "https://example.com/img.png"}),
	}
	msg := NewUserMessageBlocks(blocks...)
	if msg.Role != RoleUser {
		t.Errorf("期望 Role=%s", RoleUser)
	}
	if len(msg.Content) != 2 {
		t.Errorf("期望 2 个 ContentBlock，实际 %d 个", len(msg.Content))
	}
}

func TestNewAssistantMessageBlocks(t *testing.T) {
	blocks := []ContentBlock{
		NewTextBlock("让我查一下"),
		NewToolUseBlock("toolu_001", "search", map[string]any{"q": "test"}),
	}
	msg := NewAssistantMessageBlocks(blocks...)
	if msg.Role != RoleAssistant {
		t.Errorf("期望 Role=%s", RoleAssistant)
	}
	if len(msg.Content) != 2 {
		t.Errorf("期望 2 个 ContentBlock，实际 %d 个", len(msg.Content))
	}
}

// ---- MessageRole 常量测试 ----

func TestMessageRole_Values(t *testing.T) {
	if RoleUser != "user" {
		t.Errorf("RoleUser 期望 'user'，实际 '%s'", RoleUser)
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant 期望 'assistant'，实际 '%s'", RoleAssistant)
	}
}

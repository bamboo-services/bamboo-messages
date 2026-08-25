package responses

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestBuildAssistantItem_ReasoningID_NoRsPrefix 验证 buildAssistantItem 使用 msg.ReasoningID 而非 "rs_" + ThinkingSignature 拼接。
//
// 修复前：ID 被硬编码为 "rs_" + msg.ThinkingSignature
// 修复后：ID 使用 msg.ReasoningID
func TestBuildAssistantItem_ReasoningID_NoRsPrefix(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	msg := provider.Message{
		Role:              provider.RoleAssistant,
		Content:           "response text",
		ThinkingContent:   "thinking text",
		ThinkingSignature: "gAAAAABp_encrypted_token",
		ReasoningID:       "rs_real_id_123",
	}

	items := p.buildAssistantItem(msg)
	if len(items) == 0 {
		t.Fatal("expected at least 1 item")
	}

	// 第一个 item 应该是 reasoning
	reasoningItem := items[0]
	if reasoningItem["type"] != "reasoning" {
		t.Fatalf("expected first item type 'reasoning', got %v", reasoningItem["type"])
	}
	id, _ := reasoningItem["id"].(string)
	if id != "rs_real_id_123" {
		t.Errorf("reasoning ID = %q, want rs_real_id_123", id)
	}
	if id == "rs_gAAAAABp_encrypted_token" {
		t.Error("reasoning ID should NOT be 'rs_' + ThinkingSignature (old bug)")
	}
}

// TestBuildAssistantItem_EmptyReasoningID 验证 ReasoningID 为空时不 panic
func TestBuildAssistantItem_EmptyReasoningID(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	msg := provider.Message{
		Role:              provider.RoleAssistant,
		Content:           "response",
		ThinkingContent:   "thinking",
		ThinkingSignature: "encrypted",
		ReasoningID:       "", // 空
	}

	items := p.buildAssistantItem(msg) // 不应 panic
	if len(items) == 0 {
		t.Fatal("expected items even with empty ReasoningID")
	}

	reasoningItem := items[0]
	if reasoningItem["type"] != "reasoning" {
		t.Fatalf("expected reasoning item, got type %v", reasoningItem["type"])
	}
	id, _ := reasoningItem["id"].(string)
	if id != "" {
		t.Errorf("expected empty ID, got %q", id)
	}
}

// TestBuildAssistantItem_EncryptedContent 验证 ThinkingSignature 正确映射到 encrypted_content
func TestBuildAssistantItem_EncryptedContent(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	msg := provider.Message{
		Role:              provider.RoleAssistant,
		Content:           "text",
		ThinkingContent:   "thinking",
		ThinkingSignature: "gAAAAABp_test_encrypted",
		ReasoningID:       "rs_123",
	}

	items := p.buildAssistantItem(msg)
	reasoningItem := items[0]
	if reasoningItem["type"] != "reasoning" {
		t.Fatalf("expected reasoning item, got type %v", reasoningItem["type"])
	}
	enc, _ := reasoningItem["encrypted_content"].(string)
	if enc != "gAAAAABp_test_encrypted" {
		t.Errorf("encrypted_content = %q, want gAAAAABp_test_encrypted", enc)
	}
	summary, _ := reasoningItem["summary"].([]map[string]any)
	if len(summary) != 0 {
		t.Errorf("summary = %v, want empty array", summary)
	}
	if _, ok := reasoningItem["content"]; ok {
		t.Errorf("content = %v, want omitted by default (OpenAI content.maxItems=0)", reasoningItem["content"])
	}
}

func TestBuildAssistantItem_DefaultOmitsPlaintextReasoning(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	msg := provider.Message{
		Role:            provider.RoleAssistant,
		Content:         "response text",
		ThinkingContent: "plaintext thinking from grok",
	}

	items := p.buildAssistantItem(msg)
	if len(items) != 1 {
		t.Fatalf("expected only assistant message item, got %d items: %v", len(items), items)
	}
	if items[0]["role"] != "assistant" {
		t.Fatalf("expected assistant message, got %v", items[0])
	}
}

func TestBuildAssistantItem_IncludeReasoningContent(t *testing.T) {
	p := NewResponsesProviderWithOptions(
		WithAPIKey("test-api-key"),
		WithIncludeReasoningContent(true),
	)

	msg := provider.Message{
		Role:            provider.RoleAssistant,
		Content:         "response text",
		ThinkingContent: "plaintext thinking",
		ReasoningID:     "rs_123",
	}

	items := p.buildAssistantItem(msg)
	if len(items) < 1 {
		t.Fatal("expected reasoning item")
	}
	reasoningItem := items[0]
	if reasoningItem["type"] != "reasoning" {
		t.Fatalf("expected first item type 'reasoning', got %v", reasoningItem["type"])
	}
	content, _ := reasoningItem["content"].([]map[string]any)
	if len(content) != 1 || content[0]["type"] != "reasoning_text" || content[0]["text"] != "plaintext thinking" {
		t.Errorf("content = %v, want reasoning_text plaintext thinking", content)
	}
}

func TestBuildAssistantItem_IgnoreEncryptedContent(t *testing.T) {
	p := NewResponsesProviderWithOptions(
		WithAPIKey("test-api-key"),
		WithIgnoreEncryptedContent(true),
	)

	msg := provider.Message{
		Role:              provider.RoleAssistant,
		Content:           "response text",
		ThinkingSignature: "gAAAAABp_encrypted_token",
		ReasoningID:       "rs_123",
	}

	items := p.buildAssistantItem(msg)
	if len(items) < 1 {
		t.Fatal("expected reasoning item")
	}
	reasoningItem := items[0]
	if reasoningItem["type"] != "reasoning" {
		t.Fatalf("expected first item type 'reasoning', got %v", reasoningItem["type"])
	}
	if _, ok := reasoningItem["encrypted_content"]; ok {
		t.Errorf("encrypted_content = %v, want omitted when ignore is enabled", reasoningItem["encrypted_content"])
	}
	id, _ := reasoningItem["id"].(string)
	if id != "rs_123" {
		t.Errorf("id = %q, want rs_123", id)
	}
}

func TestBuildAssistantItem_IgnoreEncryptedDropsEmptyReasoning(t *testing.T) {
	p := NewResponsesProviderWithOptions(
		WithAPIKey("test-api-key"),
		WithIgnoreEncryptedContent(true),
	)

	msg := provider.Message{
		Role:              provider.RoleAssistant,
		Content:           "response text",
		ThinkingSignature: "gAAAAABp_encrypted_token",
	}

	items := p.buildAssistantItem(msg)
	if len(items) != 1 {
		t.Fatalf("expected only assistant message item after dropping ciphertext-only reasoning, got %d items: %v", len(items), items)
	}
	if items[0]["role"] != "assistant" {
		t.Fatalf("expected assistant message, got %v", items[0])
	}
}

func TestBuildAssistantItem_DualSwitchIndependent(t *testing.T) {
	p := NewResponsesProviderWithOptions(
		WithAPIKey("test-api-key"),
		WithIncludeReasoningContent(true),
		WithIgnoreEncryptedContent(true),
	)

	msg := provider.Message{
		Role:              provider.RoleAssistant,
		Content:           "response text",
		ThinkingContent:   "plaintext thinking",
		ThinkingSignature: "gAAAAABp_encrypted_token",
		ReasoningID:       "rs_123",
	}

	items := p.buildAssistantItem(msg)
	if len(items) < 1 {
		t.Fatal("expected reasoning item")
	}
	reasoningItem := items[0]
	if reasoningItem["type"] != "reasoning" {
		t.Fatalf("expected first item type 'reasoning', got %v", reasoningItem["type"])
	}
	content, _ := reasoningItem["content"].([]map[string]any)
	if len(content) != 1 || content[0]["text"] != "plaintext thinking" {
		t.Errorf("content = %v, want plaintext thinking", content)
	}
	if _, ok := reasoningItem["encrypted_content"]; ok {
		t.Errorf("encrypted_content = %v, want omitted", reasoningItem["encrypted_content"])
	}
}

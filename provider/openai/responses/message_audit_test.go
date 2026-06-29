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
	reasoningItem := items[0].OfReasoning
	if reasoningItem == nil {
		t.Fatal("expected first item to be reasoning")
	}
	if reasoningItem.ID != "rs_real_id_123" {
		t.Errorf("reasoning ID = %q, want rs_real_id_123", reasoningItem.ID)
	}
	if reasoningItem.ID == "rs_gAAAAABp_encrypted_token" {
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

	reasoningItem := items[0].OfReasoning
	if reasoningItem == nil {
		t.Fatal("expected reasoning item")
	}
	if reasoningItem.ID != "" {
		t.Errorf("expected empty ID, got %q", reasoningItem.ID)
	}
}

// TestBuildAssistantItem_EncryptedContent 验证 ThinkingSignature 正确映射到 EncryptedContent
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
	reasoningItem := items[0].OfReasoning
	if reasoningItem == nil {
		t.Fatal("expected reasoning item")
	}
	if !reasoningItem.EncryptedContent.Valid() {
		t.Fatal("EncryptedContent should be set")
	}
	if reasoningItem.EncryptedContent.Value != "gAAAAABp_test_encrypted" {
		t.Errorf("EncryptedContent = %q, want gAAAAABp_test_encrypted", reasoningItem.EncryptedContent.Value)
	}
}

package gemini

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	"google.golang.org/genai"
)

// TestHandlePart_ThoughtSignatureExtraction 验证 Gemini thinking part 的 ThoughtSignature 被正确提取为 SignatureDelta。
//
// Gemini 2.5 thinking 功能通过 ThoughtSignature ([]byte) 传递加密推理签名，
// 用于多轮对话中保留推理上下文。修复前该字段被完全忽略。
func TestHandlePart_ThoughtSignatureExtraction(t *testing.T) {
	p := NewProvider("test-key")

	// 构造一个带 ThoughtSignature 的 thinking part
	part := &genai.Part{
		Text:             "Let me analyze this...",
		Thought:          true,
		ThoughtSignature: []byte("EvEFCu4Fencrypted_signature"),
	}

	textStarted := false
	thinkingStarted := false
	events := p.handlePart(part, &textStarted, &thinkingStarted)

	// 应该有 3 个事件: BlockStart("thinking") + ThinkingDelta + SignatureDelta
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (BlockStart + ThinkingDelta + SignatureDelta), got %d", len(events))
	}

	// 查找 SignatureDelta 事件
	var foundSignature bool
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeSignature {
			foundSignature = true
			sigData, ok := e.Delta.Data.(provider.SignatureData)
			if !ok {
				t.Fatalf("signature delta data type = %T, want provider.SignatureData", e.Delta.Data)
			}
			if string(sigData) != "EvEFCu4Fencrypted_signature" {
				t.Errorf("signature = %q, want %q", string(sigData), "EvEFCu4Fencrypted_signature")
			}
		}
	}

	if !foundSignature {
		t.Fatal("no SignatureDelta event found — ThoughtSignature is silently dropped")
	}

	// thinkingStarted 应该被设置为 true
	if !thinkingStarted {
		t.Error("thinkingBlockStarted should be true after thinking part")
	}
}

// TestHandlePart_ThoughtSignatureOnly 验证只有 ThoughtSignature 没有 Text 的边缘情况。
func TestHandlePart_ThoughtSignatureOnly(t *testing.T) {
	p := NewProvider("test-key")

	part := &genai.Part{
		ThoughtSignature: []byte("sig_only"),
	}

	textStarted := false
	thinkingStarted := false
	events := p.handlePart(part, &textStarted, &thinkingStarted)

	// 即使没有 Text，ThoughtSignature 存在时也应发出 SignatureDelta
	var foundSignature bool
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeSignature {
			foundSignature = true
		}
	}

	if !foundSignature {
		t.Fatal("expected SignatureDelta event for thought-signature-only part, got none")
	}
}

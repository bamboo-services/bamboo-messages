package anthropic

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestContentBlockDelta_SignatureDelta 验证 signature_delta 事件被正确转换为 NewSignatureDelta。
//
// Anthropic extended thinking 在 thinking 块结束前发送 signature_delta 携带验证签名，
// 用于多轮对话中保留推理上下文。修复前该事件被 default 分支静默丢弃。
func TestContentBlockDelta_SignatureDelta(t *testing.T) {
	p := NewProvider("test-key")

	// 构造 signature_delta 事件
	// RawContentBlockDeltaUnion 的 Type 为 "content_block_delta"，
	// 内部 Delta.Type 为 "signature_delta"
	event := anthropic.BetaRawMessageStreamEventUnion{
		Type: "content_block_delta",
	}
	// 使用 JSON 注入方式设置 delta 字段
	eventJSON := `{
		"type": "content_block_delta",
		"index": 0,
		"delta": {
			"type": "signature_delta",
			"signature": "EvEFCu4F..."
		}
	}`
	if err := event.UnmarshalJSON([]byte(eventJSON)); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	var finishReason provider.FinishReason
	events := p.handleStreamEvent(event, &finishReason)

	if len(events) == 0 {
		t.Fatal("expected 1 event for signature_delta, got 0 — signature is silently dropped")
	}

	if events[0].Delta.Type != provider.StreamDeltaTypeSignature {
		t.Errorf("delta type = %q, want %q", events[0].Delta.Type, provider.StreamDeltaTypeSignature)
	}

	sigData, ok := events[0].Delta.Data.(provider.SignatureData)
	if !ok {
		t.Fatalf("delta data type = %T, want provider.SignatureData", events[0].Delta.Data)
	}

	if string(sigData) != "EvEFCu4F..." {
		t.Errorf("signature = %q, want %q", string(sigData), "EvEFCu4F...")
	}
}

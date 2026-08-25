package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

func TestParseRequest_ChatThinkingHubRoundtrip(t *testing.T) {
	body := []byte(`{
		"model":"gpt-4o",
		"messages":[{
			"role":"assistant",
			"content":"answer",
			"reasoning_content":"let me think",
			"thinking_signature":"sig_gemini",
			"thinking_provider":"gemini",
			"reasoning_id":"rs_1"
		}]
	}`)
	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Messages[0].ReasoningID != "rs_1" {
		t.Errorf("ReasoningID = %q, want rs_1", req.Messages[0].ReasoningID)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("block[0] = %T, want *ThinkingBlock", req.Messages[0].Content[0])
	}
	if tb.Thinking != "let me think" || tb.Signature != "sig_gemini" || tb.SignatureProvider != bamboo.SignatureProviderGemini {
		t.Errorf("ThinkingBlock = %+v", tb)
	}
}

func TestSerializeResponse_ChatThinkingHubFields(t *testing.T) {
	resp := &bamboo.Response{
		ID:          "resp_hub",
		Model:       "gpt-4o",
		StopReason:  bamboo.FinishReasonEndTurn,
		ReasoningID: "rs_hub",
		Content: []bamboo.ContentBlock{
			bamboo.NewThinkingBlockWithProvider("plan", "enc", bamboo.SignatureProviderOpenAIResponses),
			bamboo.NewTextBlock("done"),
		},
	}
	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}
	var out openaiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msg := out.Choices[0].Message
	if msg.ReasoningContent != "plan" {
		t.Errorf("ReasoningContent = %q", msg.ReasoningContent)
	}
	if msg.ThinkingSignature != "enc" {
		t.Errorf("ThinkingSignature = %q", msg.ThinkingSignature)
	}
	if msg.ThinkingProvider != bamboo.SignatureProviderOpenAIResponses {
		t.Errorf("ThinkingProvider = %q", msg.ThinkingProvider)
	}
	if msg.ReasoningID != "rs_hub" {
		t.Errorf("ReasoningID = %q", msg.ReasoningID)
	}
}

func TestStreamSerializer_SignatureDeltaEmitsChatHubFields(t *testing.T) {
	s := newStreamSerializer("gpt-4o")
	s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{
			Type:              bamboo.DeltaSignature,
			Signature:         "sig_1",
			SignatureProvider: bamboo.SignatureProviderGemini,
		},
	})
	if err != nil {
		t.Fatalf("Serialize error = %v", err)
	}
	if data == nil {
		t.Fatal("signature_delta should emit a Chat Completions chunk")
	}
	payload := strings.TrimPrefix(string(data), "data: ")
	payload = strings.TrimSpace(payload)
	var chunk openaiChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		t.Fatalf("unmarshal chunk: %v\n%s", err, payload)
	}
	delta := chunk.Choices[0].Delta
	if delta.ThinkingSignature != "sig_1" {
		t.Errorf("ThinkingSignature = %q", delta.ThinkingSignature)
	}
	if delta.ThinkingProvider != bamboo.SignatureProviderGemini {
		t.Errorf("ThinkingProvider = %q", delta.ThinkingProvider)
	}
}

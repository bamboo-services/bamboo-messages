package bamboo

import (
	"encoding/json"
	"testing"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
)

func TestSerializeResponse_TextBlock(t *testing.T) {
	resp := &bmbamboo.Response{
		ID:         "msg_001",
		Type:       "message",
		Role:       bmbamboo.RoleAssistant,
		Model:      "claude-sonnet-4-20250514",
		StopReason: bmbamboo.FinishReasonEndTurn,
		Content: []bmbamboo.ContentBlock{
			bmbamboo.NewTextBlock("Hello!"),
		},
		Usage: bmbamboo.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
		ProviderType: "anthropic",
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out bmbamboo.Response
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if out.ID != "msg_001" {
		t.Errorf("ID = %q", out.ID)
	}
	if out.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q", out.Model)
	}
	if out.StopReason != bmbamboo.FinishReasonEndTurn {
		t.Errorf("StopReason = %q", out.StopReason)
	}
	if out.ProviderType != "anthropic" {
		t.Errorf("ProviderType = %q", out.ProviderType)
	}
	if out.Usage.InputTokens != 10 || out.Usage.OutputTokens != 5 {
		t.Errorf("Usage = %+v", out.Usage)
	}
	tb, ok := out.Content[0].(*bmbamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", out.Content[0])
	}
	if tb.Text != "Hello!" {
		t.Errorf("Text = %q", tb.Text)
	}
}

func TestSerializeResponse_AllSevenBlockTypes(t *testing.T) {
	resp := &bmbamboo.Response{
		ID:         "msg_all",
		Type:       "message",
		Role:       bmbamboo.RoleAssistant,
		Model:      "test-model",
		StopReason: bmbamboo.FinishReasonEndTurn,
		Content: []bmbamboo.ContentBlock{
			bmbamboo.NewTextBlock("text content"),
			bmbamboo.NewThinkingBlock("thinking content", "sig_abc"),
			bmbamboo.NewToolUseBlock("call_1", "get_weather", map[string]any{"city": "SF"}),
			bmbamboo.NewToolResultBlock("call_1", "Sunny", false),
			bmbamboo.NewImageBlock(bmbamboo.ContentSource{
				Type:      "base64",
				MediaType: "image/png",
				Data:      "iVBORw0KGgo=",
			}),
			bmbamboo.NewDocumentBlock(bmbamboo.ContentSource{
				Type: "url",
				URL:  "https://example.com/doc.pdf",
			}),
			bmbamboo.NewRedactedThinkingBlock("encrypted-blob"),
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	// Round-trip: 验证所有 7 种 block 类型都能正确序列化和反序列化
	var out bmbamboo.Response
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if len(out.Content) != 7 {
		t.Fatalf("Content len = %d, want 7", len(out.Content))
	}

	// 1. TextBlock
	if tb, ok := out.Content[0].(*bmbamboo.TextBlock); !ok {
		t.Errorf("block[0] type = %T, want *TextBlock", out.Content[0])
	} else if tb.Text != "text content" {
		t.Errorf("TextBlock.Text = %q", tb.Text)
	}

	// 2. ThinkingBlock
	if tb, ok := out.Content[1].(*bmbamboo.ThinkingBlock); !ok {
		t.Errorf("block[1] type = %T, want *ThinkingBlock", out.Content[1])
	} else if tb.Thinking != "thinking content" || tb.Signature != "sig_abc" {
		t.Errorf("ThinkingBlock = %+v", tb)
	}

	// 3. ToolUseBlock
	if tub, ok := out.Content[2].(*bmbamboo.ToolUseBlock); !ok {
		t.Errorf("block[2] type = %T, want *ToolUseBlock", out.Content[2])
	} else {
		if tub.ID != "call_1" || tub.Name != "get_weather" {
			t.Errorf("ToolUseBlock = %+v", tub)
		}
		var input map[string]any
		if err := json.Unmarshal(tub.Input, &input); err != nil {
			t.Errorf("ToolUseBlock.Input not valid JSON: %v", err)
		}
		if input["city"] != "SF" {
			t.Errorf("ToolUseBlock.Input.city = %v", input["city"])
		}
	}

	// 4. ToolResultBlock
	if trb, ok := out.Content[3].(*bmbamboo.ToolResultBlock); !ok {
		t.Errorf("block[3] type = %T, want *ToolResultBlock", out.Content[3])
	} else if trb.ToolUseID != "call_1" || trb.Content != "Sunny" {
		t.Errorf("ToolResultBlock = %+v", trb)
	}

	// 5. ImageBlock
	if ib, ok := out.Content[4].(*bmbamboo.ImageBlock); !ok {
		t.Errorf("block[4] type = %T, want *ImageBlock", out.Content[4])
	} else if ib.Source == nil || ib.Source.Type != "base64" || ib.Source.Data != "iVBORw0KGgo=" {
		t.Errorf("ImageBlock.Source = %+v", ib.Source)
	}

	// 6. DocumentBlock
	if db, ok := out.Content[5].(*bmbamboo.DocumentBlock); !ok {
		t.Errorf("block[5] type = %T, want *DocumentBlock", out.Content[5])
	} else if db.Source == nil || db.Source.URL != "https://example.com/doc.pdf" {
		t.Errorf("DocumentBlock.Source = %+v", db.Source)
	}

	// 7. RedactedThinkingBlock
	if rtb, ok := out.Content[6].(*bmbamboo.RedactedThinkingBlock); !ok {
		t.Errorf("block[6] type = %T, want *RedactedThinkingBlock", out.Content[6])
	} else if rtb.Data != "encrypted-blob" {
		t.Errorf("RedactedThinkingBlock.Data = %q", rtb.Data)
	}
}

func TestSerializeResponse_RoundTrip(t *testing.T) {
	original := &bmbamboo.Response{
		ID:           "msg_rt",
		Type:         "message",
		Role:         bmbamboo.RoleAssistant,
		Model:        "test-model",
		StopReason:   bmbamboo.FinishReasonToolUse,
		StopSequence: "STOP",
		Content: []bmbamboo.ContentBlock{
			bmbamboo.NewTextBlock("round trip"),
			bmbamboo.NewToolUseBlock("call_rt", "search", map[string]any{"q": "hello"}),
		},
		Usage: bmbamboo.Usage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheCreationInputTokens: 10,
			CacheReadInputTokens:     20,
		},
		ProviderType: "openai-completions",
		RequestID:    "req_rt_001",
		ResponseID:   "resp_rt_001",
		CreatedAt:    1700000000,
	}

	data, err := serializeResponse(original)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var decoded bmbamboo.Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	// 完整字段比对
	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Model != original.Model {
		t.Errorf("Model = %q", decoded.Model)
	}
	if decoded.StopReason != original.StopReason {
		t.Errorf("StopReason = %q", decoded.StopReason)
	}
	if decoded.StopSequence != original.StopSequence {
		t.Errorf("StopSequence = %q", decoded.StopSequence)
	}
	if decoded.ProviderType != original.ProviderType {
		t.Errorf("ProviderType = %q", decoded.ProviderType)
	}
	if decoded.RequestID != original.RequestID {
		t.Errorf("RequestID = %q", decoded.RequestID)
	}
	if decoded.ResponseID != original.ResponseID {
		t.Errorf("ResponseID = %q", decoded.ResponseID)
	}
	if decoded.CreatedAt != original.CreatedAt {
		t.Errorf("CreatedAt = %d", decoded.CreatedAt)
	}
	if decoded.Usage != original.Usage {
		t.Errorf("Usage = %+v, want %+v", decoded.Usage, original.Usage)
	}
}

func TestSerializeResponse_EmptyContent(t *testing.T) {
	resp := &bmbamboo.Response{
		ID:         "msg_empty",
		Type:       "message",
		Role:       bmbamboo.RoleAssistant,
		Model:      "test-model",
		StopReason: bmbamboo.FinishReasonEndTurn,
		Content:    []bmbamboo.ContentBlock{},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out bmbamboo.Response
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if len(out.Content) != 0 {
		t.Errorf("Content len = %d, want 0", len(out.Content))
	}
}

func TestSerializeResponse_CacheUsageTokens(t *testing.T) {
	resp := &bmbamboo.Response{
		ID:         "msg_cache",
		Type:       "message",
		Role:       bmbamboo.RoleAssistant,
		Model:      "test-model",
		StopReason: bmbamboo.FinishReasonEndTurn,
		Content:    []bmbamboo.ContentBlock{bmbamboo.NewTextBlock("hi")},
		Usage: bmbamboo.Usage{
			InputTokens:              636,
			OutputTokens:             10,
			CacheCreationInputTokens: 0,
			CacheReadInputTokens:     98880,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out bmbamboo.Response
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	// identity codec 不做 input_tokens 语义转换，直接透传
	if out.Usage.InputTokens != 636 {
		t.Errorf("InputTokens = %d, want 636 (identity passthrough)", out.Usage.InputTokens)
	}
	if out.Usage.CacheReadInputTokens != 98880 {
		t.Errorf("CacheReadInputTokens = %d", out.Usage.CacheReadInputTokens)
	}
}

// TestCodec_SerializeResponse_Delegation 验证 Codec.SerializeResponse 委托到 serializeResponse。
func TestCodec_SerializeResponse_Delegation(t *testing.T) {
	resp := &bmbamboo.Response{
		ID:         "msg_codec",
		Type:       "message",
		Role:       bmbamboo.RoleAssistant,
		Model:      "test-model",
		StopReason: bmbamboo.FinishReasonEndTurn,
		Content:    []bmbamboo.ContentBlock{bmbamboo.NewTextBlock("via codec")},
	}

	data, err := Codec.SerializeResponse(resp)
	if err != nil {
		t.Fatalf("Codec.SerializeResponse() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Codec.SerializeResponse() returned empty data")
	}
}

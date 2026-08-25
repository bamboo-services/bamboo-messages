package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

func TestSerializeResponse_TextOnly(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "msg_001",
		Model:      "claude-sonnet-4-20250514",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("Hello!"),
		},
		Usage: bamboo.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out anthropicResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.ID != "msg_001" {
		t.Errorf("ID = %q", out.ID)
	}
	if out.Type != "message" {
		t.Errorf("Type = %q, want %q", out.Type, "message")
	}
	if out.Role != "assistant" {
		t.Errorf("Role = %q", out.Role)
	}
	if out.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q", out.Model)
	}
	if out.StopReason != bamboo.FinishReasonEndTurn {
		t.Errorf("StopReason = %q, want %q", out.StopReason, bamboo.FinishReasonEndTurn)
	}
	if len(out.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(out.Content))
	}
	// 验证 content block 内容
	var block map[string]any
	if err := json.Unmarshal(out.Content[0], &block); err != nil {
		t.Fatalf("failed to unmarshal content block: %v", err)
	}
	if block["type"] != "text" {
		t.Errorf("content type = %v, want %q", block["type"], "text")
	}
	if block["text"] != "Hello!" {
		t.Errorf("text = %v, want %q", block["text"], "Hello!")
	}
	if out.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d", out.Usage.OutputTokens)
	}
}

func TestSerializeResponse_ToolUse(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "msg_002",
		Model:      "claude-sonnet-4-20250514",
		StopReason: bamboo.FinishReasonToolUse,
		Content: []bamboo.ContentBlock{
			bamboo.NewToolUseBlock("call_abc", "get_weather", map[string]any{"city": "SF"}),
		},
		Usage: bamboo.Usage{
			InputTokens:  15,
			OutputTokens: 10,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out anthropicResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.StopReason != bamboo.FinishReasonToolUse {
		t.Errorf("StopReason = %q, want %q", out.StopReason, bamboo.FinishReasonToolUse)
	}
	if len(out.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(out.Content))
	}

	var block map[string]any
	if err := json.Unmarshal(out.Content[0], &block); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if block["type"] != "tool_use" {
		t.Errorf("type = %v, want %q", block["type"], "tool_use")
	}
	if block["id"] != "call_abc" {
		t.Errorf("id = %v", block["id"])
	}
	if block["name"] != "get_weather" {
		t.Errorf("name = %v", block["name"])
	}
	// input should be a JSON object
	input, ok := block["input"].(map[string]any)
	if !ok {
		t.Fatalf("input should be object, got %T", block["input"])
	}
	if input["city"] != "SF" {
		t.Errorf("input.city = %v", input["city"])
	}
}

func TestSerializeResponse_Thinking(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "msg_003",
		Model:      "claude-sonnet-4-20250514",
		StopReason: bamboo.FinishReasonEndTurn,
		Content: []bamboo.ContentBlock{
			bamboo.NewThinkingBlockWithProvider("Let me think...", "sig_abc", bamboo.SignatureProviderAnthropic),
			bamboo.NewTextBlock("The answer is 42."),
		},
		Usage: bamboo.Usage{
			InputTokens:  20,
			OutputTokens: 30,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out anthropicResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if len(out.Content) != 2 {
		t.Fatalf("Content len = %d, want 2", len(out.Content))
	}

	// first block: thinking
	var thinkBlock map[string]any
	if err := json.Unmarshal(out.Content[0], &thinkBlock); err != nil {
		t.Fatalf("unmarshal thinking block: %v", err)
	}
	if thinkBlock["type"] != "thinking" {
		t.Errorf("type = %v, want %q", thinkBlock["type"], "thinking")
	}
	if thinkBlock["thinking"] != "Let me think..." {
		t.Errorf("thinking = %v", thinkBlock["thinking"])
	}
	if thinkBlock["signature"] != "sig_abc" {
		t.Errorf("signature = %v", thinkBlock["signature"])
	}

	// second block: text
	var textBlock map[string]any
	if err := json.Unmarshal(out.Content[1], &textBlock); err != nil {
		t.Fatalf("unmarshal text block: %v", err)
	}
	if textBlock["type"] != "text" {
		t.Errorf("type = %v, want %q", textBlock["type"], "text")
	}
	if textBlock["text"] != "The answer is 42." {
		t.Errorf("text = %v", textBlock["text"])
	}
}

func TestSerializeResponse_StopSequence(t *testing.T) {
	resp := &bamboo.Response{
		ID:           "msg_004",
		Model:        "claude-sonnet-4-20250514",
		StopReason:   bamboo.FinishReasonStopSequence,
		StopSequence: "STOP",
		Content: []bamboo.ContentBlock{
			bamboo.NewTextBlock("partial"),
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out anthropicResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if out.StopReason != bamboo.FinishReasonStopSequence {
		t.Errorf("StopReason = %q", out.StopReason)
	}
	if out.StopSequence == nil || *out.StopSequence != "STOP" {
		t.Errorf("StopSequence = %v, want %q", out.StopSequence, "STOP")
	}
}

func TestSerializeResponse_EmptyContent(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "msg_005",
		Model:      "claude-sonnet-4-20250514",
		StopReason: bamboo.FinishReasonEndTurn,
		Content:    []bamboo.ContentBlock{},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out anthropicResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	if len(out.Content) != 0 {
		t.Errorf("Content len = %d, want 0", len(out.Content))
	}
	if out.StopReason != bamboo.FinishReasonEndTurn {
		t.Errorf("StopReason = %q", out.StopReason)
	}
}

// TestSerializeResponse_CacheTokenSemantics 验证 Anthropic 协议的 input_tokens
// 仅包含非缓存输入 token（bamboo.Usage.InputTokens - cache_creation - cache_read）。
//
// Anthropic 协议语义：
//   - input_tokens = 非缓存输入 token
//   - cache_creation_input_tokens = 缓存创建 token
//   - cache_read_input_tokens = 缓存读取 token
//   - 总输入 = input_tokens + cache_creation + cache_read
//
// bamboo.Usage.InputTokens 是总输入（含缓存），需要减去缓存部分。
func TestSerializeResponse_CacheTokenSemantics(t *testing.T) {
	resp := &bamboo.Response{
		ID:         "msg_cache",
		Model:      "claude-sonnet-4-20250514",
		StopReason: bamboo.FinishReasonEndTurn,
		Content:    []bamboo.ContentBlock{bamboo.NewTextBlock("hi")},
		Usage: bamboo.Usage{
			InputTokens:              99516,
			OutputTokens:             139,
			CacheCreationInputTokens: 0,
			CacheReadInputTokens:     98880,
		},
	}

	data, err := serializeResponse(resp)
	if err != nil {
		t.Fatalf("serializeResponse() error = %v", err)
	}

	var out anthropicResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	expectedNonCached := int64(99516 - 0 - 98880) // = 636
	if out.Usage.InputTokens != expectedNonCached {
		t.Errorf("input_tokens = %d, want %d (non-cached portion)", out.Usage.InputTokens, expectedNonCached)
	}
	if out.Usage.CacheReadInputTokens != 98880 {
		t.Errorf("cache_read_input_tokens = %d, want 98880", out.Usage.CacheReadInputTokens)
	}

	// 验证总输入 = input_tokens + cache_read = 636 + 98880 = 99516
	totalReconstructed := out.Usage.InputTokens + out.Usage.CacheCreationInputTokens + out.Usage.CacheReadInputTokens
	if totalReconstructed != 99516 {
		t.Errorf("reconstructed total = %d, want 99516 (original InputTokens)", totalReconstructed)
	}
}

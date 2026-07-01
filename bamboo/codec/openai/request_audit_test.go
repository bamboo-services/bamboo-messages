package openai

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── OpenAI Completions Codec Audit Tests ──
// Tests for issues found during N-to-N conversion safety audit.

// TestAudit_OpenAI_ToolChoiceFunctionLoss verifies that tool_choice function name
// is parsed but name itself is not preserved in the unified format.
//
// Severity: P2
// Issue: parseToolChoice maps {type:"function", function:{name:"xxx"}} to "forced",
//
//	losing the specific function name.
//
// Affected: OpenAI→Any conversion with specific function tool_choice.
func TestAudit_OpenAI_ToolChoiceFunctionLoss(t *testing.T) {
	choice, err := parseToolChoice(json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`))
	if err != nil {
		t.Fatalf("parseToolChoice() error = %v", err)
	}
	if choice != "forced" {
		t.Errorf("parseToolChoice() = %q, want %q", choice, "forced")
	}
	// Documenting: the function name "get_weather" is lost. A unified ToolChoice
	// format of just "forced" cannot represent which specific tool to force.
}

// TestAudit_OpenAI_UserMessageContentParts verifies user content part parsing.
func TestAudit_OpenAI_UserMessageContentParts(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Describe this:"},
				{"type": "image_url", "image_url": {"url": "https://example.com/img.png"}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	msg := req.Messages[0]
	if len(msg.Content) != 2 {
		t.Fatalf("Content len = %d, want 2", len(msg.Content))
	}

	if _, ok := msg.Content[0].(*bamboo.TextBlock); !ok {
		t.Errorf("Content[0] expected *TextBlock, got %T", msg.Content[0])
	}
	if _, ok := msg.Content[1].(*bamboo.ImageBlock); !ok {
		t.Errorf("Content[1] expected *ImageBlock, got %T", msg.Content[1])
	}
}

// TestAudit_OpenAI_AssistantContentAndToolCalls verifies assistant message parsing.
func TestAudit_OpenAI_AssistantContentAndToolCalls(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{
			"role": "assistant",
			"content": "Let me check.",
			"tool_calls": [{
				"id": "call_1",
				"type": "function",
				"function": {"name": "search", "arguments": "{\"q\":\"test\"}"}
			}, {
				"id": "call_2",
				"type": "function",
				"function": {"name": "lookup", "arguments": "{}"}
			}]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	msg := req.Messages[0]
	// text + 2 tool calls = 3 blocks
	if len(msg.Content) != 3 {
		t.Fatalf("Content len = %d, want 3", len(msg.Content))
	}

	// Verify text
	tb, ok := msg.Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("Content[0] expected *TextBlock, got %T", msg.Content[0])
	}
	if tb.Text != "Let me check." {
		t.Errorf("Text = %q", tb.Text)
	}

	// Verify tool call IDs preserved
	tu1, ok := msg.Content[1].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("Content[1] expected *ToolUseBlock, got %T", msg.Content[1])
	}
	if tu1.ID != "call_1" || tu1.Name != "search" {
		t.Errorf("ToolUse[1]: ID=%q Name=%q", tu1.ID, tu1.Name)
	}

	tu2, ok := msg.Content[2].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("Content[2] expected *ToolUseBlock, got %T", msg.Content[2])
	}
	if tu2.ID != "call_2" || tu2.Name != "lookup" {
		t.Errorf("ToolUse[2]: ID=%q Name=%q", tu2.ID, tu2.Name)
	}
}

// TestAudit_OpenAI_ToolResultWithEmptyContent verifies tool result with empty content.
func TestAudit_OpenAI_ToolResultWithEmptyContent(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{
			"role": "tool",
			"content": "",
			"tool_call_id": "call_abc"
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	trBlock, ok := req.Messages[0].Content[0].(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", req.Messages[0].Content[0])
	}
	if trBlock.ToolUseID != "call_abc" {
		t.Errorf("ToolUseID = %q", trBlock.ToolUseID)
	}
	if trBlock.Content != "" {
		t.Errorf("Content = %q, want empty", trBlock.Content)
	}
}

// TestAudit_OpenAI_PromptCacheKey verifies prompt_cache_key parsing.
func TestAudit_OpenAI_PromptCacheKey(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"prompt_cache_key": "my-cache-key"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	if req.Config.PromptCacheKey != "my-cache-key" {
		t.Errorf("PromptCacheKey = %q, want %q", req.Config.PromptCacheKey, "my-cache-key")
	}
}

// TestAudit_OpenAI_ParallelToolCalls verifies parallel_tool_calls parsing.
func TestAudit_OpenAI_ParallelToolCalls(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"parallel_tool_calls": true
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	if !req.Config.ParallelToolCalls {
		t.Errorf("ParallelToolCalls = false, want true")
	}
}

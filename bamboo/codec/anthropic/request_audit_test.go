package anthropic

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── Anthropic Codec Audit Tests ──
// Tests for issues found during N-to-N conversion safety audit.

// TestAudit_Anthropic_DocumentBlockNotParsed verifies that Anthropic "document" content blocks
// are silently dropped during parsing.
//
// Severity: P1
// Issue: convertContentBlock does not handle "document" type.
// Affected: Anthropic→Any conversion with PDF/document content.
func TestAudit_Anthropic_DocumentBlockNotParsed(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Summarize this document"},
				{"type": "document", "source": {"type": "base64", "media_type": "application/pdf", "data": "JVBERi0xLjQK"}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	msg := req.Messages[0]
	// Currently only 1 block is parsed (text); the document block is silently dropped.
	// After fix, there should be 2 blocks.
	if len(msg.Content) == 1 {
		t.Errorf("Document block was silently dropped: got %d content blocks, want 2 (text + document)", len(msg.Content))
	}
	// Verify the second block would be a DocumentBlock
	if len(msg.Content) >= 2 {
		_, ok := msg.Content[1].(*bamboo.DocumentBlock)
		if !ok {
			t.Errorf("Expected second block to be *DocumentBlock, got %T", msg.Content[1])
		}
	}
}

// TestAudit_Anthropic_ToolChoiceForced_NameLoss verifies that tool_choice {type:"tool", name:"xxx"}
// loses the tool name during parsing — maps to "forced" but the adapter treats it as OfAny.
//
// Severity: P2
// Issue: parseToolChoice maps {type:"tool"} to "forced" but discards the name field.
//
//	The Anthropic adapter maps "forced" → OfAny (force any tool), not OfTool (force specific tool).
//
// Affected: Anthropic→Anthropic (and any Anthropic→Other) with specific tool forcing.
func TestAudit_Anthropic_ToolChoiceForced_NameLoss(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"tool_choice": {"type": "tool", "name": "get_weather"},
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// The tool_choice should be "forced" but the name "get_weather" is lost
	if req.Config.ToolChoice != "forced" {
		t.Errorf("ToolChoice = %q, want %q", req.Config.ToolChoice, "forced")
	}

	// Documenting: the specific tool name "get_weather" cannot be recovered from config.
	// When the adapter maps "forced" → OfAny, it forces any tool, not the specific one.
	// A correct mapping should use ProviderExtra or a new field to preserve the tool name.
}

// TestAudit_Anthropic_ToolCacheControl verifies tool-level cache_control parsing.
func TestAudit_Anthropic_ToolCacheControl(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hi"}],
		"tools": [{
			"name": "get_weather",
			"description": "Get weather",
			"input_schema": {"type": "object"},
			"cache_control": {"type": "ephemeral"}
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Config.Tools) != 1 {
		t.Fatalf("Tools len = %d", len(req.Config.Tools))
	}
	if req.Config.Tools[0].CacheControl == nil {
		t.Fatal("Tool CacheControl should not be nil")
	}
	if req.Config.Tools[0].CacheControl.Type != "ephemeral" {
		t.Errorf("CacheControl.Type = %q, want %q", req.Config.Tools[0].CacheControl.Type, "ephemeral")
	}
}

// TestAudit_Anthropic_ContentBlockCacheControl verifies block-level cache_control parsing.
func TestAudit_Anthropic_ContentBlockCacheControl(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Cached text", "cache_control": {"type": "ephemeral"}}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", req.Messages[0].Content[0])
	}
	if tb.CacheControl == nil {
		t.Fatal("CacheControl should not be nil")
	}
	if tb.CacheControl.Type != "ephemeral" {
		t.Errorf("CacheControl.Type = %q", tb.CacheControl.Type)
	}
}

// TestAudit_Anthropic_UnknownContentBlockDropped verifies unknown content block types are silently dropped.
func TestAudit_Anthropic_UnknownContentBlockDropped(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Hello"},
				{"type": "future_type", "data": "something"}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	// Unknown block type is silently dropped (returns nil from convertContentBlock)
	if len(req.Messages[0].Content) != 1 {
		t.Errorf("Content len = %d, want 1 (unknown block should be dropped)", len(req.Messages[0].Content))
	}
}

// TestAudit_Anthropic_ToolResultContentArray verifies tool_result with array content.
func TestAudit_Anthropic_ToolResultContentArray(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [{
				"type": "tool_result",
				"tool_use_id": "call_123",
				"content": [{"type": "text", "text": "part1"}, {"type": "text", "text": "part2"}]
			}]
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
	if trBlock.Content != "part1\npart2" {
		t.Errorf("Content = %q, want %q", trBlock.Content, "part1\npart2")
	}
}

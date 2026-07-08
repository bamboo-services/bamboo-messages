package responses

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── Responses Codec Audit Tests ──
// Tests for issues found during N-to-N conversion safety audit.

// TestAudit_Responses_MetadataToProviderExtra verifies that the "metadata" field
// is parsed into ProviderExtra instead of config.Metadata.
//
// Severity: P1
// Issue: Responses codec puts metadata into ProviderExtra["metadata"] (as map[string]any),
//
//	but config.Metadata (map[string]string) is the standard field.
//	This means metadata is invisible to code that reads config.Metadata.
//
// Affected: Responses→Any conversion where metadata matters.
func TestAudit_Responses_MetadataToProviderExtra(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"metadata": {"key1": "value1", "key2": "value2"}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// Fix: 当所有 metadata 值都是 string 时，应存入 config.Metadata
	if req.Config.Metadata == nil {
		t.Fatal("config.Metadata is nil — all-string metadata should be stored in config.Metadata")
	}
	if req.Config.Metadata["key1"] != "value1" {
		t.Errorf("config.Metadata[key1] = %q, want %q", req.Config.Metadata["key1"], "value1")
	}
	if req.Config.Metadata["key2"] != "value2" {
		t.Errorf("config.Metadata[key2] = %q, want %q", req.Config.Metadata["key2"], "value2")
	}
	// 全 string 时不应存入 ProviderExtra
	if req.Config.ProviderExtra != nil {
		if _, ok := req.Config.ProviderExtra["metadata"]; ok {
			t.Errorf("all-string metadata should not be in ProviderExtra")
		}
	}
}

// TestAudit_Responses_ImageContentNotParsed verifies that "input_image" content parts
// in input messages are silently dropped.
//
// Severity: P1
// Issue: parseInputMessage only handles text parts (input_text/output_text), ignoring input_image.
// Affected: Responses→Any conversion with image input.
func TestAudit_Responses_ImageContentNotParsed(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [{
			"type": "message",
			"role": "user",
			"content": [
				{"type": "input_text", "text": "What's in this image?"},
				{"type": "input_image", "image_url": "https://example.com/img.png"}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	msg := req.Messages[0]
	// Currently only text blocks are parsed; input_image is silently dropped.
	if len(msg.Content) == 1 {
		t.Errorf("Image content was silently dropped: got %d blocks, want 2 (text + image)", len(msg.Content))
	}
	if len(msg.Content) >= 2 {
		_, ok := msg.Content[1].(*bamboo.ImageBlock)
		if !ok {
			t.Errorf("Expected second block to be *ImageBlock, got %T", msg.Content[1])
		}
	}
}

// TestAudit_Responses_FileContentNotParsed verifies that "input_file" content parts
// in input messages are silently dropped.
//
// Severity: P1
// Issue: parseInputMessage only handles text parts, ignoring input_file.
// Affected: Responses→Any conversion with file/document input.
func TestAudit_Responses_FileContentNotParsed(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [{
			"type": "message",
			"role": "user",
			"content": [
				{"type": "input_text", "text": "Summarize this"},
				{"type": "input_file", "file_id": "file-abc123"}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	msg := req.Messages[0]
	if len(msg.Content) == 1 {
		t.Errorf("File content was silently dropped: got %d blocks, want 2 (text + file)", len(msg.Content))
	}
}

// TestAudit_Responses_MetadataStringOnly verifies that all-string metadata still goes to ProviderExtra.
func TestAudit_Responses_MetadataStringOnly(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"metadata": {"user_id": "u123", "session": "s456"}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// Fix: 全 string metadata 应存入 config.Metadata
	if req.Config.Metadata == nil {
		t.Fatal("config.Metadata is nil — all-string metadata should be stored in config.Metadata")
	}
	if req.Config.Metadata["user_id"] != "u123" {
		t.Errorf("config.Metadata[user_id] = %q, want %q", req.Config.Metadata["user_id"], "u123")
	}
}

// TestAudit_Responses_ImageOnlyContentDropped verifies that when all content parts are
// non-text types, the message is dropped entirely.
//
// Severity: P1
// Issue: Only input_text/output_text parts are parsed; other types cause empty content,
//
//	which then causes the message to be filtered out entirely.
func TestAudit_Responses_ImageOnlyContentDropped(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [{
			"type": "message",
			"role": "user",
			"content": [
				{"type": "input_image", "image_url": "https://example.com/photo.jpg"}
			]
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// When only non-text parts exist, the message is dropped entirely because
	// parseInputMessage returns empty blocks, and the caller skips empty messages.
	if len(req.Messages) > 0 {
		msg := req.Messages[0]
		if len(msg.Content) == 0 {
			t.Errorf("Message exists but has empty content — input_image was silently dropped")
		}
	} else {
		// Document the behavior: no messages because all content was non-text
		t.Logf("No messages parsed — entire message dropped because only non-text content parts were present")
	}
}

// TestAudit_Metadata_StoredInProviderExtra 验证 Responses codec 将 metadata 存储在 ProviderExtra 中。
//
// Severity: P1
// File:Line: bamboo/codec/responses/request.go:159-164
// Issue: codec 将 metadata 存储在 ProviderExtra["metadata"] 中，
//
//	但 Responses provider adapter 从 config.Metadata 读取。
//	当通过 codec→relay→provider 路径时，metadata 被静默丢弃。
//
// Affected: Any→Responses conversion with metadata
func TestAudit_Metadata_StoredInProviderExtra(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],
		"metadata": {"key1": "value1", "key2": "value2"}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// Fix: 全 string metadata 应存入 config.Metadata
	if req.Config.Metadata == nil {
		t.Fatal("config.Metadata is nil — all-string metadata should be stored in config.Metadata")
	}

	if req.Config.Metadata["key1"] != "value1" {
		t.Errorf("config.Metadata[key1] = %q, want %q", req.Config.Metadata["key1"], "value1")
	}
	if req.Config.Metadata["key2"] != "value2" {
		t.Errorf("config.Metadata[key2] = %q, want %q", req.Config.Metadata["key2"], "value2")
	}

	// 全 string 时不应存入 ProviderExtra
	if req.Config.ProviderExtra != nil {
		if _, ok := req.Config.ProviderExtra["metadata"]; ok {
			t.Errorf("all-string metadata should not be in ProviderExtra")
		}
	}

	t.Logf("FIXED: metadata now correctly stored in config.Metadata for all-string values")
}

func TestAudit_Responses_PromptCacheKey(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"input": "hello",
		"prompt_cache_key": "session-abc"
	}`

	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest failed: %v", err)
	}
	if req.Config.PromptCacheKey != "session-abc" {
		t.Errorf("PromptCacheKey = %q, want %q", req.Config.PromptCacheKey, "session-abc")
	}
}

func TestAudit_Responses_PromptCacheKey_Empty(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"input": "hello"
	}`

	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest failed: %v", err)
	}
	if req.Config.PromptCacheKey != "" {
		t.Errorf("PromptCacheKey = %q, want empty", req.Config.PromptCacheKey)
	}
}

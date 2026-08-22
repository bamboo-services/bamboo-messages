package gemini

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── Gemini Codec Audit Tests ──
// Tests for issues found during N-to-N conversion safety audit.

// TestAudit_Gemini_IsStreamHardcodedFalse verifies that IsStream is always false.
//
// Fix: IsStream is hardcoded to false in parseRequest. Gemini uses URL param ?alt=sse
// to indicate streaming, which is not present in the request body.
// The relay layer must override IsStream based on URL context.
func TestAudit_Gemini_IsStreamHardcodedFalse(t *testing.T) {
	body := []byte(`{
		"contents": [{"role":"user","parts":[{"text":"Hi"}]}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// IsStream is always false by design — Gemini stream intent is in URL `?alt=sse`,
	// not in the request body. The relay layer is responsible for overriding this.
	if req.IsStream {
		t.Errorf("IsStream = true, want false (always hardcoded, relay overrides from URL)")
	}
}

// TestAudit_Gemini_ModelNotParsed verifies that the model field is not in the request body.
//
// Severity: P1
// Issue: Gemini puts the model in the URL path (e.g., /v1beta/models/gemini-2.5-pro:generateContent),
//
//	not in the request body. config.Model will be empty string.
//
// Affected: Gemini→Any conversion; model information is lost.
func TestAudit_Gemini_ModelNotParsed(t *testing.T) {
	body := []byte(`{
		"contents": [{"role":"user","parts":[{"text":"Hi"}]}],
		"generationConfig": {"temperature": 0.7}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// Model is always empty because Gemini doesn't include it in the body
	if req.Config.Model != "" {
		t.Errorf("Model = %q, want empty (Gemini model is in URL path)", req.Config.Model)
	}
}

// TestAudit_Gemini_ThinkingConfigParsed verifies that generationConfig.thinkingConfig
// is correctly parsed into bamboo ThinkingConfig.
//
// Fix: Gemini 2.5 models support thinkingConfig in generationConfig.
// The codec now parses thinkingBudget and maps it to effort levels.
func TestAudit_Gemini_ThinkingConfigParsed(t *testing.T) {
	tests := []struct {
		name       string
		budget     string
		wantEffort string
	}{
		{"low budget", "1024", "low"},
		{"medium budget", "4096", "medium"},
		{"high budget", "16384", "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"contents":[{"role":"user","parts":[{"text":"Think hard"}]}],"generationConfig":{"thinkingConfig":{"thinkingBudget":` + tt.budget + `}}}`)
			req, err := parseRequest(body)
			if err != nil {
				t.Fatalf("parseRequest() error = %v", err)
			}
			if req.Config.ThinkingConfig == nil {
				t.Fatal("ThinkingConfig is nil, want non-nil")
			}
			if req.Config.ThinkingConfig.Effort != tt.wantEffort {
				t.Errorf("Effort = %q, want %q", req.Config.ThinkingConfig.Effort, tt.wantEffort)
			}
		})
	}
}

// TestAudit_Gemini_FunctionResponseIDFallback verifies function response ID fallback to name.
func TestAudit_Gemini_FunctionResponseIDFallback(t *testing.T) {
	body := []byte(`{
		"contents": [{
			"role": "function",
			"parts": [{
				"functionResponse": {
					"name": "get_weather",
					"response": {"output": "sunny"}
				}
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
	// When ID is empty, falls back to name
	if trBlock.ToolUseID != "get_weather" {
		t.Errorf("ToolUseID = %q, want %q (fallback to name)", trBlock.ToolUseID, "get_weather")
	}
}

// TestAudit_Gemini_FunctionResponseWithID verifies function response with explicit ID.
func TestAudit_Gemini_FunctionResponseWithID(t *testing.T) {
	body := []byte(`{
		"contents": [{
			"role": "function",
			"parts": [{
				"functionResponse": {
					"id": "resp-123",
					"name": "get_weather",
					"response": {"output": "sunny"}
				}
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
	if trBlock.ToolUseID != "resp-123" {
		t.Errorf("ToolUseID = %q, want %q", trBlock.ToolUseID, "resp-123")
	}
}

// TestAudit_Gemini_EmptyModel verifies model is empty (documents the gap).
func TestAudit_Gemini_ThoughtPartBecomesThinkingBlock(t *testing.T) {
	body := []byte(`{
		"contents":[{"role":"model","parts":[
			{"text":"let me think","thought":true,"thoughtSignature":"sig_1"},
			{"text":"answer"}
		]}]
	}`)
	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
		t.Fatalf("messages/content = %+v", req.Messages)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("block[0] = %T, want *ThinkingBlock", req.Messages[0].Content[0])
	}
	if tb.Thinking != "let me think" || tb.Signature != "sig_1" {
		t.Errorf("ThinkingBlock = %+v", tb)
	}
	text, ok := req.Messages[0].Content[1].(*bamboo.TextBlock)
	if !ok || text.Text != "answer" {
		t.Errorf("block[1] = %+v", req.Messages[0].Content[1])
	}
}

func TestAudit_Gemini_FunctionCallThoughtSignatureSplitsBlocks(t *testing.T) {
	body := []byte(`{
		"contents":[{"role":"model","parts":[
			{"functionCall":{"id":"call-1","name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"sig_fc"}
		]}]
	}`)
	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
		t.Fatalf("messages/content = %+v", req.Messages)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("block[0] = %T, want *ThinkingBlock", req.Messages[0].Content[0])
	}
	if tb.Signature != "sig_fc" {
		t.Errorf("ThinkingBlock.Signature = %q, want sig_fc", tb.Signature)
	}
	tu, ok := req.Messages[0].Content[1].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("block[1] = %T, want *ToolUseBlock", req.Messages[0].Content[1])
	}
	if tu.ID != "call-1" || tu.Name != "get_weather" {
		t.Errorf("ToolUseBlock = %+v", tu)
	}
}

func TestAudit_Gemini_ThinkingLevelParsed(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"Hi"}]}],"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH","includeThoughts":true}}}`)
	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ThinkingConfig == nil || req.Config.ThinkingConfig.Effort != "high" {
		t.Fatalf("ThinkingConfig = %+v", req.Config.ThinkingConfig)
	}
}

func TestAudit_Gemini_EmptyModel(t *testing.T) {
	body := []byte(`{
		"contents": [{"role":"user","parts":[{"text":"test"}]}],
		"generationConfig": {"maxOutputTokens": 1024}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	if req.Config.Model != "" {
		t.Errorf("Model should be empty for Gemini (model is in URL path), got %q", req.Config.Model)
	}
}

// TestAudit_Gemini_NoCacheControl verifies that CacheControl is not used in Gemini codec.
func TestAudit_Gemini_NoCacheControl(t *testing.T) {
	body := []byte(`{
		"contents": [{"role":"user","parts":[{"text":"test"}]}],
		"cachedContent": "projects/123/locations/us/cachedContents/abc"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// cachedContent goes to ProviderExtra (not CacheControl)
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra is nil")
	}
	cc, ok := req.Config.ProviderExtra["cached_content"].(string)
	if !ok || cc != "projects/123/locations/us/cachedContents/abc" {
		t.Errorf("cached_content = %v", req.Config.ProviderExtra["cached_content"])
	}
}

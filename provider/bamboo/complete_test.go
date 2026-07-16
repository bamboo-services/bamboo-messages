package bamboo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// Complete 非流式测试
// ==============================

// TestComplete_TextResponse 验证纯文本响应解析。
func TestComplete_TextResponse(t *testing.T) {
	respBody := `{
		"id":"msg_001","type":"message","role":"assistant","model":"test-model",
		"stop_reason":"end_turn",
		"content":[{"type":"text","text":"Hello, world!"}],
		"usage":{"input_tokens":10,"output_tokens":5},
		"response_id":"resp_001"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "Hello, world!" {
		t.Errorf("Content = %q, want 'Hello, world!'", result.Content)
	}
	if result.FinishReason != provider.FinishReasonStop {
		t.Errorf("FinishReason = %v, want %v", result.FinishReason, provider.FinishReasonStop)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", result.Usage.OutputTokens)
	}
	if result.ResponseID != "resp_001" {
		t.Errorf("ResponseID = %q, want 'resp_001'", result.ResponseID)
	}
}

// TestComplete_FullContentBlocks 验证完整内容块（text + tool_use + thinking + redacted_thinking）。
func TestComplete_FullContentBlocks(t *testing.T) {
	respBody := `{
		"id":"msg_002","type":"message","role":"assistant","model":"test-model",
		"stop_reason":"end_turn",
		"content":[
			{"type":"thinking","thinking":"Let me reason...","signature":"sig_abc"},
			{"type":"redacted_thinking","data":"encrypted_data_1"},
			{"type":"text","text":"The answer is 42."},
			{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"Tokyo"}}
		],
		"usage":{"input_tokens":20,"output_tokens":15,"cache_creation_input_tokens":5,"cache_read_input_tokens":3}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Text
	if result.Content != "The answer is 42." {
		t.Errorf("Content = %q, want 'The answer is 42.'", result.Content)
	}

	// Thinking
	if result.Thinking != "Let me reason..." {
		t.Errorf("Thinking = %q, want 'Let me reason...'", result.Thinking)
	}
	if result.ThinkingSignature != "sig_abc" {
		t.Errorf("ThinkingSignature = %q, want 'sig_abc'", result.ThinkingSignature)
	}

	// RedactedThinking
	if len(result.RedactedThinking) != 1 || result.RedactedThinking[0] != "encrypted_data_1" {
		t.Errorf("RedactedThinking = %v, want ['encrypted_data_1']", result.RedactedThinking)
	}

	// ToolCalls
	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ID != "toolu_01" {
		t.Errorf("ToolCall ID = %q, want 'toolu_01'", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("ToolCall Type = %q, want 'function'", tc.Type)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("ToolCall Name = %q, want 'get_weather'", tc.Function.Name)
	}
	if !strings.Contains(tc.Function.Arguments, "Tokyo") {
		t.Errorf("ToolCall Arguments = %q, want contains 'Tokyo'", tc.Function.Arguments)
	}

	// Usage with cache
	if result.Usage.CacheCreationInputTokens != 5 {
		t.Errorf("CacheCreationInputTokens = %d, want 5", result.Usage.CacheCreationInputTokens)
	}
	if result.Usage.CacheReadInputTokens != 3 {
		t.Errorf("CacheReadInputTokens = %d, want 3", result.Usage.CacheReadInputTokens)
	}
}

// TestComplete_HTTPError 验证 HTTP 500 错误包装为 BambooError。
func TestComplete_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"category":"internal","message":"Internal server error","status_code":500}}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var bambooErr *pkgErrors.BambooError
	if !errors.As(err, &bambooErr) {
		t.Fatalf("expected *BambooError, got %T: %v", err, err)
	}
	if bambooErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", bambooErr.StatusCode)
	}
	if !strings.Contains(bambooErr.Message, "Internal server error") {
		t.Errorf("Message = %q, want contains 'Internal server error'", bambooErr.Message)
	}
}

// TestComplete_FinishReasonMappings 参数化测试所有 7 种 FinishReason 映射。
func TestComplete_FinishReasonMappings(t *testing.T) {
	tests := []struct {
		wireReason string
		want       provider.FinishReason
	}{
		{"end_turn", provider.FinishReasonStop},
		{"max_tokens", provider.FinishReasonLength},
		{"tool_use", provider.FinishReasonToolCalls},
		{"stop_sequence", provider.FinishReasonStop},
		{"pause_turn", provider.FinishReasonPauseTurn},
		{"refusal", provider.FinishReasonRefusal},
		{"server_tool_use", provider.FinishReasonServerToolUse},
	}

	for _, tt := range tests {
		t.Run(tt.wireReason, func(t *testing.T) {
			respBody := `{"id":"msg_x","type":"message","role":"assistant","model":"m","stop_reason":"` + tt.wireReason + `","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(respBody))
			}))
			defer server.Close()

			p := newMockProvider(t, server)
			ctx := context.Background()
			config := &provider.ChatConfig{Model: "m", MaxTokens: 100}
			messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

			result, err := p.Complete(ctx, messages, config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.FinishReason != tt.want {
				t.Errorf("stop_reason %q → FinishReason = %v, want %v", tt.wireReason, result.FinishReason, tt.want)
			}
		})
	}
}

// TestComplete_RequestFormat 验证请求路径和流式标记。
func TestComplete_RequestFormat(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedBody, _ = readBody(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"m","stop_reason":"end_turn","content":[],"usage":{"input_tokens":0,"output_tokens":0}}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "m", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	_, _ = p.Complete(ctx, messages, config)

	if capturedMethod != http.MethodPost {
		t.Errorf("HTTP method = %q, want %q", capturedMethod, http.MethodPost)
	}
	if capturedPath != "/v1/bamboo" {
		t.Errorf("URL path = %q, want /v1/bamboo", capturedPath)
	}
	// 非流式不应包含 stream:true
	if strings.Contains(string(capturedBody), `"stream":true`) {
		t.Errorf("non-stream body should not contain stream:true: %s", string(capturedBody))
	}
}

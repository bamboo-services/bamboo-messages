package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// Complete 非流式测试
// ==============================

// TestComplete_SingleThinkingBlock 验证单个 thinking block 的 signature 保留。
func TestComplete_SingleThinkingBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_001",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "thinking", "thinking": "Step 1", "signature": "sig-abc"}
			],
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if result.Thinking != "Step 1" {
		t.Errorf("Thinking = %q, want 'Step 1'", result.Thinking)
	}
	if result.ThinkingSignature != "sig-abc" {
		t.Errorf("ThinkingSignature = %q, want 'sig-abc'", result.ThinkingSignature)
	}
}

// TestComplete_MultipleThinkingBlocks 验证多个 thinking block 的 signature 拼接保留。
func TestComplete_MultipleThinkingBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_002",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "thinking", "thinking": "Step 1", "signature": "sig-aaa"},
				{"type": "thinking", "thinking": "Step 2", "signature": "sig-bbb"}
			],
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 10}
		}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if result.Thinking != "Step 1Step 2" {
		t.Errorf("Thinking = %q, want 'Step 1Step 2'", result.Thinking)
	}

	expectedSig := "sig-aaa\n---\nsig-bbb"
	if result.ThinkingSignature != expectedSig {
		t.Errorf("ThinkingSignature = %q, want %q", result.ThinkingSignature, expectedSig)
	}

	// 验证包含两个 signature
	if !strings.Contains(result.ThinkingSignature, "sig-aaa") {
		t.Error("ThinkingSignature missing 'sig-aaa'")
	}
	if !strings.Contains(result.ThinkingSignature, "sig-bbb") {
		t.Error("ThinkingSignature missing 'sig-bbb'")
	}
}

// TestComplete_MixedBlocksWithThinking 验证 text + thinking + tool_use 混合块。
func TestComplete_MixedBlocksWithThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_003",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "thinking", "thinking": "Let me think...", "signature": "sig-first"},
				{"type": "text", "text": "The answer is "},
				{"type": "thinking", "thinking": "Double check...", "signature": "sig-second"},
				{"type": "text", "text": "42."},
				{"type": "tool_use", "id": "toolu_01", "name": "get_answer", "input": {"value": 42}}
			],
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 20, "output_tokens": 15}
		}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if result.Content != "The answer is 42." {
		t.Errorf("Content = %q, want 'The answer is 42.'", result.Content)
	}
	if result.Thinking != "Let me think...Double check..." {
		t.Errorf("Thinking = %q, want 'Let me think...Double check...'", result.Thinking)
	}

	expectedSig := "sig-first\n---\nsig-second"
	if result.ThinkingSignature != expectedSig {
		t.Errorf("ThinkingSignature = %q, want %q", result.ThinkingSignature, expectedSig)
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ID != "toolu_01" {
		t.Errorf("ToolCalls[0].ID = %q, want 'toolu_01'", result.ToolCalls[0].ID)
	}
	if result.FinishReason != provider.FinishReasonToolCalls {
		t.Errorf("FinishReason = %v, want %v", result.FinishReason, provider.FinishReasonToolCalls)
	}
}

// TestComplete_ThinkingBlockWithoutSignature 验证无 signature 的 thinking block 不产生影响。
func TestComplete_ThinkingBlockWithoutSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_004",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "thinking", "thinking": "No sig here", "signature": ""},
				{"type": "text", "text": "OK"}
			],
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 5, "output_tokens": 2}
		}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if result.Thinking != "No sig here" {
		t.Errorf("Thinking = %q, want 'No sig here'", result.Thinking)
	}
	if result.ThinkingSignature != "" {
		t.Errorf("ThinkingSignature = %q, want empty", result.ThinkingSignature)
	}
}

// TestComplete_EmptyContent 验证空 content 数组不 panic。
func TestComplete_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_005",
			"type": "message",
			"role": "assistant",
			"content": [],
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 5, "output_tokens": 0}
		}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if result.Content != "" {
		t.Errorf("Content = %q, want empty", result.Content)
	}
	if result.Thinking != "" {
		t.Errorf("Thinking = %q, want empty", result.Thinking)
	}
	if result.ThinkingSignature != "" {
		t.Errorf("ThinkingSignature = %q, want empty", result.ThinkingSignature)
	}
	if result.ResponseID != "msg_005" {
		t.Errorf("ResponseID = %q, want 'msg_005'", result.ResponseID)
	}
}

// TestComplete_ErrorResponse 验证 Complete 的 HTTP 错误处理。
func TestComplete_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err == nil {
		t.Fatal("Complete() expected error, got nil")
	}
	if result != nil {
		t.Error("Complete() expected nil result on error")
	}
	if !strings.Contains(err.Error(), "Rate limit exceeded") {
		t.Errorf("error message = %q, should contain 'Rate limit exceeded'", err.Error())
	}
}

// TestComplete_RedactedThinkingBlock 验证非流式响应中 redacted_thinking block 的 data 被提取到 RedactedThinking。
func TestComplete_RedactedThinkingBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_rt_001",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "redacted_thinking", "data": "encrypted-data-aaa"},
				{"type": "text", "text": "Here is my answer."}
			],
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if len(result.RedactedThinking) != 1 {
		t.Fatalf("RedactedThinking len = %d, want 1", len(result.RedactedThinking))
	}
	if result.RedactedThinking[0] != "encrypted-data-aaa" {
		t.Errorf("RedactedThinking[0] = %q, want 'encrypted-data-aaa'", result.RedactedThinking[0])
	}
	if result.Content != "Here is my answer." {
		t.Errorf("Content = %q, want 'Here is my answer.'", result.Content)
	}
}

// TestComplete_MultipleRedactedThinkingBlocks 验证多个 redacted_thinking block 全部提取。
func TestComplete_MultipleRedactedThinkingBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_rt_002",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "redacted_thinking", "data": "rt-data-1"},
				{"type": "redacted_thinking", "data": "rt-data-2"},
				{"type": "text", "text": "Done."}
			],
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 3}
		}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, config)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if len(result.RedactedThinking) != 2 {
		t.Fatalf("RedactedThinking len = %d, want 2", len(result.RedactedThinking))
	}
	if result.RedactedThinking[0] != "rt-data-1" {
		t.Errorf("RedactedThinking[0] = %q, want 'rt-data-1'", result.RedactedThinking[0])
	}
	if result.RedactedThinking[1] != "rt-data-2" {
		t.Errorf("RedactedThinking[1] = %q, want 'rt-data-2'", result.RedactedThinking[1])
	}
}

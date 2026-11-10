package completions

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestChat_NilConfigDoesNotPanic 验证 config 为 nil 时不会 panic。
//
// 回归场景：Provider 契约规定「config 传 nil 时使用默认配置」，
// 但适配器在拼装 endpoint 时直接读取 config.Model，nil 配置会触发空指针 panic。
func TestChat_NilConfigDoesNotPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseFrame(`{"id":"1","choices":[{"index":0,"delta":{"content":"Hi"}}]}`))
		_, _ = fmt.Fprint(w, sseFrame(`[DONE]`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, nil)

	events := collectEvents(ch)
	if len(events) == 0 {
		t.Fatal("expected events")
	}
}

// TestComplete_NilConfigDoesNotPanic 验证非流式路径同样容忍 nil config。
func TestComplete_NilConfigDoesNotPanic(t *testing.T) {
	body := `{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"Hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := p.Complete(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", result.Usage.InputTokens)
	}
}

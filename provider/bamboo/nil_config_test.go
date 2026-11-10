package bamboo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestChat_NilConfigDoesNotPanic 验证 config 为 nil 时不会 panic。
//
// 回归场景：Provider 契约规定「config 传 nil 时使用默认配置」，
// 但适配器在拼装 endpoint 时直接读取 config.Model，nil 配置会触发空指针 panic。
func TestChat_NilConfigDoesNotPanic(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, nil)
	events := drainEvents(ch)
	if len(events) == 0 {
		t.Fatal("expected events")
	}
}

// TestComplete_NilConfigDoesNotPanic 验证非流式路径同样容忍 nil config。
func TestComplete_NilConfigDoesNotPanic(t *testing.T) {
	body := `{"id":"resp_001","type":"message","role":"assistant","content":[{"type":"text","text":"Hi"}],"model":"test-model","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	result, err := p.Complete(ctx, messages, nil)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", result.Usage.InputTokens)
	}
}

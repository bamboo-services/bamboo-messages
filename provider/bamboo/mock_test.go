package bamboo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// httptest 辅助工具
// ==============================

// newMockProvider 创建指向 mock server 的 Provider 实例。
func newMockProvider(t *testing.T, server *httptest.Server) *Provider {
	t.Helper()
	p := NewProviderWithOptions(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)
	return p
}

// sseFixture 构建符合 SSE 格式的事件序列。
//
// 每个事件由 event: 行和 data: 行组成，事件间用空行分隔。
func sseFixture(events ...[2]string) string {
	var sb strings.Builder
	for _, ev := range events {
		sb.WriteString("event: ")
		sb.WriteString(ev[0])
		sb.WriteString("\n")
		sb.WriteString("data: ")
		sb.WriteString(ev[1])
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// drainEvents 从 channel 收集所有事件直到关闭，带超时保护。
func drainEvents(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-time.After(5 * time.Second):
			return events
		}
	}
}

// findEventByType 在事件列表中查找指定类型的事件。
func findEventByType(events []provider.StreamEvent, t provider.StreamType) (provider.StreamEvent, bool) {
	for _, ev := range events {
		if ev.Type == t {
			return ev, true
		}
	}
	return provider.StreamEvent{}, false
}

// findDeltaByType 在事件列表中查找指定 delta 类型的 Delta 事件。
func findDeltaByType(events []provider.StreamEvent, dt provider.StreamDeltaType) (provider.StreamEvent, bool) {
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == dt {
			return ev, true
		}
	}
	return provider.StreamEvent{}, false
}

// readBody 读取 http.Request 的 body 并返回字节。
func readBody(r *http.Request) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

// TestChat_TimingAnchor 验证流式事件的物理时间锚点：
// Start 事件携带 TimingAnchor（RequestSentAt/ResponseHeaderAt），
// 服务端注入延迟后 TTFT 必须覆盖该延迟，且 RequestSentAt ≤ ResponseHeaderAt ≤ 首帧时刻。
func TestChat_TimingAnchor(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","role":"assistant"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		time.Sleep(150 * time.Millisecond) // 模拟上游 Prefill 延迟
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "bamboo-default", MaxTokens: 100}
	ch := p.Chat(ctx, []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}, config)
	events := drainEvents(ch)

	startEv, ok := findEventByType(events, provider.StreamTypeStart)
	if !ok {
		t.Fatal("expected StreamTypeStart event, not found")
	}
	if startEv.Timing == nil {
		t.Fatal("Start event should carry TimingAnchor, got nil")
	}
	if startEv.Timing.RequestSentAt.IsZero() || startEv.Timing.ResponseHeaderAt.IsZero() {
		t.Error("Timing anchor timestamps should be non-zero")
	}
	if startEv.ReceivedAt.IsZero() {
		t.Error("Start event should carry ReceivedAt")
	}
	if startEv.Timing.RequestSentAt.After(startEv.Timing.ResponseHeaderAt) {
		t.Error("RequestSentAt should be <= ResponseHeaderAt")
	}
	if startEv.Timing.ResponseHeaderAt.After(startEv.ReceivedAt) {
		t.Error("ResponseHeaderAt should be <= first frame ReceivedAt")
	}

	// 文本增量必须携带非零 ReceivedAt
	textEv, ok := findDeltaByType(events, provider.StreamDeltaTypeTextOutput)
	if !ok {
		t.Fatal("expected text delta event, not found")
	}
	if textEv.ReceivedAt.IsZero() {
		t.Error("text delta should carry ReceivedAt")
	}

	// TimingCollector 消费锚点后 TTFT 必须覆盖 150ms 服务端延迟
	tc := provider.NewTimingCollector()
	for _, ev := range events {
		tc.Observe(ev)
	}
	stats := tc.Stats()
	if stats.FirstByteDuration < 100*time.Millisecond {
		t.Errorf("TTFT = %v, want >= 100ms (must cover server prefill delay)", stats.FirstByteDuration)
	}
	if stats.FirstByteDuration > stats.TotalDuration {
		t.Errorf("invariant violated: TTFT (%v) > TotalDuration (%v)", stats.FirstByteDuration, stats.TotalDuration)
	}
}

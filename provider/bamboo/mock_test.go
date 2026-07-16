package bamboo

import (
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

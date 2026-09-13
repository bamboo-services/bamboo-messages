package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

func TestGeminiTask2CancellationHeldOpen(t *testing.T) {
	// Given
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(released)
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ready\"}]}}]}\n\n"); err != nil {
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch := newTestProvider(server).Chat(ctx, nil, &provider.ChatConfig{Model: "fixture"})
	select {
	case event := <-ch:
		if event.Type != provider.StreamTypeStart {
			t.Fatalf("first event=%v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not start")
	}
	// When
	cancel()
	events := collectTask2Events(t, ch)
	// Then: 取消路径不约束已有缓冲事件，只要求终止且不重复终态。
	counts := make(map[provider.StreamType]int)
	for _, event := range events {
		counts[event.Type]++
	}
	if counts[provider.StreamTypeStop] > 1 || counts[provider.StreamTypeDone] > 1 || counts[provider.StreamTypeError] > 1 {
		t.Fatalf("duplicate terminal events: %v", counts)
	}
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not release server")
	}
	t.Logf("cancelled channel_closed=true server_released=true terminal_counts=%v", counts)
}

func TestGeminiTask2DegradedEOF(t *testing.T) {
	// Given
	server := mockGeminiServer(t, 200, []string{`{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`})
	defer server.Close()
	p := NewProviderWithOptions(WithBaseURL(server.URL), WithAPIKey("fixture"), WithDegradedReason(provider.DegradedReasonToolUse))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// When
	events := collectTask2Events(t, p.Chat(ctx, nil, &provider.ChatConfig{Model: "fixture", Tools: []provider.Tool{{Type: "function", Function: provider.FunctionDef{Name: "inspect_state"}}}}))
	// Then
	stop := findEventType(events, provider.StreamTypeStop)
	if stop < 0 || events[stop].FinishReason != provider.FinishReasonToolCalls || findEventType(events, provider.StreamTypeDone) < 0 {
		t.Fatalf("degraded EOF contract changed: %v", events)
	}
}

func TestGeminiTask2NonErrorEnvelopeControls(t *testing.T) {
	for _, frame := range []string{`{"error":null}`, `{"other":{}}`, `{"error":`, `{"error":"invalid"}`} {
		t.Run(frame, func(t *testing.T) {
			// Given
			server := mockGeminiServer(t, 200, []string{frame, `{"candidates":[{"content":{"parts":[{"text":"ready"}]},"finishReason":"STOP"}]}`})
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// When
			events := collectTask2Events(t, newTestProvider(server).Chat(ctx, nil, &provider.ChatConfig{Model: "fixture"}))
			// Then
			if findEventType(events, provider.StreamTypeError) >= 0 || findDeltaType(events, provider.StreamDeltaTypeTextOutput) == nil || findEventType(events, provider.StreamTypeDone) < 0 {
				t.Fatalf("non-error/malformed frame changed tolerance: %v", events)
			}
		})
	}
}

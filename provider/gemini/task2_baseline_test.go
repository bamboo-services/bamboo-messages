package gemini

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

func collectTask2Events(t *testing.T, ch <-chan provider.StreamEvent) []provider.StreamEvent {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	var events []provider.StreamEvent
	for {
		select {
		case event, open := <-ch:
			if !open {
				return events
			}
			events = append(events, event)
		case <-timer.C:
			t.Fatalf("channel did not close promptly; received %v", events)
		}
	}
}

func TestGeminiTask2BaselineTermination(t *testing.T) {
	for _, tc := range []struct {
		name, part, finish string
		want               provider.FinishReason
	}{
		{"text_stop", `{"text":"ready"}`, "STOP", provider.FinishReasonStop},
		{"tool_stop", `{"functionCall":{"id":"call_A","name":"inspect_state","args":{}}}`, "STOP", provider.FinishReasonToolCalls},
		{"max_tokens", `{"text":"partial"}`, "MAX_TOKENS", provider.FinishReasonLength},
		{"ordinary_eof", `{"text":"partial"}`, "", provider.FinishReasonStop},
		{"safety", `{"text":"partial"}`, "SAFETY", provider.FinishReasonStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			frame := fmt.Sprintf(`{"candidates":[{"content":{"parts":[%s]},"finishReason":%q}]}`, tc.part, tc.finish)
			server := mockGeminiServer(t, 200, []string{frame})
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// When
			events := collectTask2Events(t, newTestProvider(server).Chat(ctx, nil, &provider.ChatConfig{Model: "fixture"}))
			// Then
			stops, dones := 0, 0
			for _, event := range events {
				switch event.Type {
				case provider.StreamTypeStop:
					stops++
					if event.FinishReason != tc.want {
						t.Errorf("finish=%q want=%q", event.FinishReason, tc.want)
					}
				case provider.StreamTypeDone:
					dones++
				case provider.StreamTypeError:
					t.Errorf("unexpected error: %v", event.Err)
				}
			}
			if stops != 1 || dones != 1 {
				t.Fatalf("Stop=%d Done=%d want 1 each", stops, dones)
			}
		})
	}
}

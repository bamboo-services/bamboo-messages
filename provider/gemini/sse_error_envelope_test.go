package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

func TestGeminiSSEErrorEnvelope(t *testing.T) {
	call := `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_A","name":"inspect_state","args":{}}}]}}]}`
	errFrame := `{"error":{"code":429,"message":"fixture quota","status":"RESOURCE_EXHAUSTED"}}`
	for _, tc := range []struct {
		name        string
		frames      []string
		code, calls int
		hold        bool
	}{
		{"first", []string{errFrame}, 429, 0, false},
		{"after_call_eof", []string{call, errFrame}, 429, 1, false},
		{"candidates_error", []string{`{"error":{"code":503,"message":"fixture quota"},"candidates":[{"content":{"parts":[{"functionCall":{"id":"bad","name":"inspect_state","args":{}}}]},"finishReason":"STOP"}]}`}, 503, 0, false},
		{"missing_code", []string{`{"error":{"message":"fixture quota"}}`}, 0, 0, false},
		{"string_code", []string{`{"error":{"code":"429","message":"fixture quota"}}`}, 0, 0, false},
		{"fractional_code", []string{`{"error":{"code":429.5,"message":"fixture quota"}}`}, 0, 0, false},
		{"negative_code", []string{`{"error":{"code":-1,"message":"fixture quota"}}`}, 0, 0, false},
		{"overflow_code", []string{`{"error":{"code":999999999999999999999,"message":"fixture quota"}}`}, 0, 0, false},
		{"null_code", []string{`{"error":{"code":null,"message":"fixture quota"}}`}, 0, 0, false},
		{"held_open", []string{errFrame}, 429, 0, true},
		{"call_then_held_open", []string{call, errFrame}, 429, 1, true},
		{"error_then_call", []string{errFrame, call, errFrame}, 429, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			released := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(released)
				w.Header().Set("Content-Type", "text/event-stream")
				for _, frame := range tc.frames {
					if _, err := fmt.Fprintf(w, "data: %s\n\n", frame); err != nil {
						return
					}
					if err := http.NewResponseController(w).Flush(); err != nil {
						return
					}
				}
				if tc.hold {
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// When
			events := collectTask2Events(t, newTestProvider(server).Chat(ctx, nil, &provider.ChatConfig{Model: "fixture"}))
			// Then
			errors, calls, starts, stops, dones := 0, 0, 0, 0, 0
			var sequence []string
			for _, event := range events {
				sequence = append(sequence, string(event.Type)+":"+string(event.Delta.Type))
				switch event.Type {
				case provider.StreamTypeError:
					errors++
					if event.Err == nil {
						t.Error("nil error detail")
						continue
					}
					if event.Err.Message != "Gemini: fixture quota" || event.Err.StatusCode != tc.code || event.StatusCode != tc.code {
						t.Errorf("error=%+v event code=%d want message=Gemini: fixture quota code=%d", event.Err, event.StatusCode, tc.code)
					}
				case provider.StreamTypeStart:
					starts++
				case provider.StreamTypeStop:
					stops++
				case provider.StreamTypeDone:
					dones++
				case provider.StreamTypeDelta:
					if errors > 0 {
						t.Error("delta after error")
					}
					if event.Delta.Type == provider.StreamDeltaTypeToolCall {
						calls++
					}
				}
			}
			if errors != 1 || stops != 0 || dones != 0 || calls != tc.calls || (tc.calls == 0 && starts != 0) {
				t.Errorf("Error=%d Start=%d Stop=%d Done=%d Calls=%d want one Error/no success/calls=%d", errors, starts, stops, dones, calls, tc.calls)
			}
			select {
			case <-released:
			case <-time.After(2 * time.Second):
				t.Fatal("response body not closed: server still held open")
			}
			t.Logf("wire=%v Error=%d code=%d message=%q Stop=%d Done=%d calls=%d channel_closed=true server_released=true before_cancel=true", sequence, errors, tc.code, "Gemini: fixture quota", stops, dones, calls)
		})
	}
}

func TestGeminiSSEErrorEnvelopeOptionalStatus(t *testing.T) {
	for _, tc := range []struct {
		name, status string
	}{
		{"numeric", `,"status":42`},
		{"omitted", ""},
		{"null", `,"status":null`},
		{"string", `,"status":"RESOURCE_EXHAUSTED"`},
		{"boolean", `,"status":false`},
		{"object", `,"status":{}`},
		{"array", `,"status":[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			frame := `{"error":{"code":429,"message":"review quota"` + tc.status + `},"candidates":[{"content":{"parts":[{"functionCall":{"id":"bad","name":"inspect_state","args":{}}}]},"finishReason":"STOP"}]}`
			server := mockGeminiServer(t, http.StatusOK, []string{frame})
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// When
			events := collectTask2Events(t, newTestProvider(server).Chat(ctx, nil, &provider.ChatConfig{Model: "review-fixture"}))
			// Then
			wire, err := json.Marshal(events)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("events=%s channel_closed=true", wire)
			if len(events) != 1 || events[0].Type != provider.StreamTypeError {
				t.Fatalf("error object lost precedence: %s", wire)
			}
			event := events[0]
			if event.Err == nil || event.Err.Message != "Gemini: review quota" || event.Err.StatusCode != 429 || event.StatusCode != 429 {
				t.Fatalf("error message/code not preserved: %s", wire)
			}
		})
	}
}

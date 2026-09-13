package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	"github.com/bamboo-services/bamboo-messages/provider"
	geminiprovider "github.com/bamboo-services/bamboo-messages/provider/gemini"
)

func TestGeminiToolLoopNativeErrorBeforeCompletion(t *testing.T) {
	// Given: a complete native functionCall Part stays pending until the terminal event.
	f := newGeminiLoopWire(t, true, []string{`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"pending_A","name":"inspect_state","args":{"round":"A"}}}]}}]}` + "\n\ndata: " + `{"error":{"code":429,"message":"fixture quota"}}`})
	var callbacks atomic.Int32
	callbackErrors := make(chan error, 4)
	// When: the real provider observes functionCall -> error -> EOF.
	frames, err := RelayStream(f.ctx, f.p, []byte(`{"contents":[{"role":"user","parts":[{"text":"inspect fixture"}]}]}`), codec.FormatGemini, codec.FormatGemini, WithErrorCallback(func(err error) {
		callbacks.Add(1)
		callbackErrors <- err
	}))
	if err != nil {
		t.Fatalf("late error returned synchronously: %v", err)
	}
	out := geminiLoopReadSSE(t, geminiLoopDrain(t, frames))
	// Then: one callback/error frame, no executable pending call or synthetic success.
	if callbacks.Load() != 1 {
		t.Fatalf("callbacks=%d", callbacks.Load())
	}
	var be *bamboo.BambooError
	if err := <-callbackErrors; !errors.As(err, &be) || be.StatusCode != 429 || be.Message != "Gemini: fixture quota" {
		t.Fatalf("callback=%v", err)
	}
	if len(out.errors) != 1 || out.errors[0].Error.Code != 429 || out.errors[0].Error.Message != "Gemini: fixture quota" || len(out.model.Parts) != 0 || len(out.finishes) != 0 {
		t.Fatalf("error aggregate=%+v", out)
	}
	if f.requests.Load() != 1 {
		t.Fatalf("requests=%d", f.requests.Load())
	}
	t.Log("ERROR_ASSERTION callback=1 status=429 error_frame=1 executable_calls=0 finish_frames=0")
}

func TestGeminiToolLoopMalformedSerializerArguments(t *testing.T) {
	// Given: malformed partial JSON cannot be represented by a valid Gemini full-Part wire.
	p := &mockProvider{streamEvents: []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		{Type: provider.StreamTypeDelta, Delta: provider.NewToolCallDelta("pending_A", "inspect_state")},
		{Type: provider.StreamTypeDelta, Delta: provider.NewToolCallDeltaData(`{"round":`)},
		{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonToolCalls},
		{Type: provider.StreamTypeDone},
	}}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	callbackErrors := make(chan error, 4)
	// When: real facade and Gemini serializer consume a late malformed argument fragment.
	frames, err := RelayStream(ctx, p, []byte(`{"contents":[{"role":"user","parts":[{"text":"inspect fixture"}]}]}`), codec.FormatGemini, codec.FormatGemini, WithErrorCallback(func(err error) { callbackErrors <- err }))
	if err != nil {
		t.Fatalf("serializer-local error must not be synchronous: %v", err)
	}
	out := geminiLoopReadSSE(t, geminiLoopDrain(t, frames))
	// Then: callback owns the error; Serialize error bytes and later successful flush stay absent.
	if len(callbackErrors) != 1 {
		t.Fatalf("callbacks=%d", len(callbackErrors))
	}
	var be *bamboo.BambooError
	if err := <-callbackErrors; !errors.As(err, &be) || be.Category != "下游" || be.StatusCode != 0 || !strings.Contains(be.Message, "must be a JSON object") {
		t.Fatalf("callback=%v", err)
	}
	if len(out.model.Parts) != 0 || len(out.finishes) != 0 || len(out.errors) != 0 {
		t.Fatalf("malformed aggregate=%+v", out)
	}
	t.Log("ERROR_ASSERTION callback=1 status=0 error_frame=0 executable_calls=0 finish_frames=0; synthetic provider channel closed")
}

func TestGeminiToolLoopCancelledStream(t *testing.T) {
	// Given: localhost sends observable text then holds a pending tool call open.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	release := make(chan struct{})
	disconnected := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(disconnected)
		if requests.Add(1) != 1 {
			t.Error("unexpected repeated request")
			w.WriteHeader(400)
			return
		}
		want := "/v1beta/models/" + geminiLoopModel + ":streamGenerateContent?alt=sse"
		if r.URL.RequestURI() != want {
			t.Errorf("URL=%s want=%s", r.URL.RequestURI(), want)
			w.WriteHeader(400)
			return
		}
		t.Logf("WIRE_URL request1 %s", r.URL.RequestURI())
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
			return
		}
		t.Logf("WIRE_BODY request1 %s", raw)
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"ready"},{"functionCall":{"id":"pending_A","name":"inspect_state","args":{"round":"A"}}}]}}]}`+"\n\n"); err != nil {
			t.Error(err)
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		cancel()
		close(release)
		server.Close()
		t.Log("TEARDOWN: cancelled context and localhost server closed")
	}()
	p := geminiLoopProvider{geminiprovider.NewProviderWithOptions(geminiprovider.WithAPIKey("dummy"), geminiprovider.WithBaseURL(server.URL))}
	frames, err := RelayStream(ctx, p, []byte(`{"contents":[{"role":"user","parts":[{"text":"inspect fixture"}]}]}`), codec.FormatGemini, codec.FormatGemini)
	if err != nil {
		t.Fatal(err)
	}
	var first []byte
	select {
	case first = <-frames:
	case <-ctx.Done():
		t.Fatal("no initial frame")
	}
	ready := geminiLoopReadSSE(t, first)
	t.Logf("WIRE_SSE_INITIAL %q", first)
	if len(ready.model.Parts) != 1 || geminiLoopParts(t, []geminiLoopContent{ready.model})[0].Text != "ready" {
		t.Fatalf("initial=%s", first)
	}
	// When: caller cancels after receiving a real frame, before any tool completion.
	cancel()
	out := geminiLoopReadSSE(t, geminiLoopDrain(t, frames))
	// Then: closure is bounded, no pending executable call or successful completion escapes.
	if len(out.model.Parts) != 0 || len(out.finishes) != 0 {
		t.Fatalf("cancel aggregate=%+v", out)
	}
	select {
	case <-disconnected:
	case <-time.After(3 * time.Second):
		t.Fatal("server handler did not close")
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d", requests.Load())
	}
	t.Log("CANCEL_ASSERTION server_handler_closed=true executable_calls=0 finish_frames=0")
}

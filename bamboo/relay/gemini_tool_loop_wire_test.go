package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	"github.com/bamboo-services/bamboo-messages/provider"
	geminiprovider "github.com/bamboo-services/bamboo-messages/provider/gemini"
)

type geminiLoopWireRequest struct {
	geminiLoopRequest
	SystemInstruction *geminiLoopContent `json:"systemInstruction,omitempty"`
}

type geminiLoopSignedPart struct {
	geminiLoopPart
	ThoughtSignature string `json:"thoughtSignature"`
	Thought          bool   `json:"thought"`
}

type geminiLoopEnvelope struct {
	Candidates []struct {
		Content      geminiLoopContent `json:"content"`
		FinishReason string            `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type geminiLoopOutput struct {
	model    geminiLoopContent
	finishes []string
	errors   []geminiLoopEnvelope
}

type geminiLoopWire struct {
	t        *testing.T
	ctx      context.Context
	p        geminiLoopProvider
	stream   bool
	requests atomic.Int32
	captures chan geminiLoopWireRequest
}

func newGeminiLoopWire(t *testing.T, stream bool, responses []string) *geminiLoopWire {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	f := &geminiLoopWire{t: t, ctx: ctx, stream: stream, captures: make(chan geminiLoopWireRequest, len(responses))}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(f.requests.Add(1))
		want := "/v1beta/models/" + geminiLoopModel + ":generateContent"
		if stream {
			want = "/v1beta/models/" + geminiLoopModel + ":streamGenerateContent?alt=sse"
		}
		// 路由先于请求体断言，防止错误模型路径掩盖历史回归。
		if r.URL.RequestURI() != want || r.Method != http.MethodPost {
			t.Errorf("request%d route=%s %s want POST %s", n, r.Method, r.URL.RequestURI(), want)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.Logf("WIRE_URL request%d %s", n, r.URL.RequestURI())
		if n > len(responses) {
			t.Errorf("request cap exceeded: %d > %d", n, len(responses))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body geminiLoopWireRequest
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.Logf("WIRE_BODY request%d %s", n, raw)
		f.captures <- body
		response := responses[n-1]
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			response = "data: " + response + "\n\n"
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		if _, err := io.WriteString(w, response); err != nil {
			t.Error(err)
		}
	}))
	f.p = geminiLoopProvider{geminiprovider.NewProviderWithOptions(geminiprovider.WithAPIKey("dummy"), geminiprovider.WithBaseURL(server.URL))}
	t.Cleanup(func() {
		cancel()
		server.Close()
		t.Log("TEARDOWN: fixture context cancelled; localhost server closed; no persistent process")
	})
	return f
}

func (f *geminiLoopWire) exchange(body []byte) geminiLoopOutput {
	f.t.Helper()
	if f.stream {
		frames, err := RelayStream(f.ctx, f.p, body, codec.FormatGemini, codec.FormatGemini)
		if err != nil {
			f.t.Fatal(err)
		}
		return geminiLoopReadSSE(f.t, geminiLoopDrain(f.t, frames))
	}
	data, err := Relay(f.ctx, f.p, body, codec.FormatGemini, codec.FormatGemini)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Logf("WIRE_JSON %s", data)
	return geminiLoopDecode(f.t, [][]byte{data})
}

func geminiLoopDrain(t *testing.T, frames <-chan []byte) []byte {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	var aggregate bytes.Buffer
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Logf("WIRE_SSE %q", aggregate.String())
				t.Log("TEARDOWN: relay output channel closed within 3s")
				return aggregate.Bytes()
			}
			aggregate.Write(frame)
		case <-timer.C:
			t.Fatal("relay channel did not close within 3s")
		}
	}
}

func geminiLoopReadSSE(t *testing.T, data []byte) geminiLoopOutput {
	t.Helper()
	scanner := provider.NewSSEScanner(io.NopCloser(bytes.NewReader(data)))
	defer func() {
		if err := scanner.Close(); err != nil {
			t.Error(err)
		}
		t.Log("TEARDOWN: aggregate SSE scanner closed")
	}()
	var frames [][]byte
	for {
		_, raw, done, err := scanner.Next()
		if done || err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, bytes.Clone(raw))
	}
	return geminiLoopDecode(t, frames)
}

func geminiLoopDecode(t *testing.T, frames [][]byte) geminiLoopOutput {
	t.Helper()
	out := geminiLoopOutput{model: geminiLoopContent{Role: "model"}}
	for _, raw := range frames {
		var frame geminiLoopEnvelope
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Error != nil {
			out.errors = append(out.errors, frame)
		}
		for _, c := range frame.Candidates {
			out.model.Parts = append(out.model.Parts, c.Content.Parts...)
			if c.FinishReason != "" {
				out.finishes = append(out.finishes, c.FinishReason)
			}
		}
	}
	return out
}

func geminiLoopJSON[T any](t *testing.T, value T) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func geminiLoopParts(t *testing.T, contents []geminiLoopContent) []geminiLoopSignedPart {
	t.Helper()
	var parts []geminiLoopSignedPart
	for _, content := range contents {
		for _, raw := range content.Parts {
			var part geminiLoopSignedPart
			if err := json.Unmarshal(raw, &part); err != nil {
				t.Fatal(err)
			}
			parts = append(parts, part)
		}
	}
	return parts
}

func geminiLoopPairs(t *testing.T, contents []geminiLoopContent) []string {
	t.Helper()
	var pairs []string
	for _, part := range geminiLoopParts(t, contents) {
		if c := part.FunctionCall; c != nil {
			pairs = append(pairs, fmt.Sprintf("call:%s:%s:%s", c.ID, c.Name, c.Args.Round))
		}
		if r := part.FunctionResponse; r != nil {
			pairs = append(pairs, fmt.Sprintf("result:%s:%s:%s:%s", r.ID, r.Name, r.Response.Output, r.Response.Error))
		}
	}
	return pairs
}

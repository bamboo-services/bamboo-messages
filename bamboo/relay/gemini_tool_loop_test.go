package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/gemini"
	"github.com/bamboo-services/bamboo-messages/provider"
	geminiprovider "github.com/bamboo-services/bamboo-messages/provider/gemini"
)

const geminiLoopModel = "fixture-model"

type geminiLoopProvider struct{ provider.Provider }

func (p geminiLoopProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

func (p geminiLoopProvider) ChatWithSystem(ctx context.Context, system string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	cfg := provider.ChatConfig{}
	if config != nil {
		cfg = *config
	}
	cfg.Model = geminiLoopModel
	return p.Provider.ChatWithSystem(ctx, system, messages, &cfg)
}

func (p geminiLoopProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

func (p geminiLoopProvider) CompleteWithSystem(ctx context.Context, system string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	cfg := provider.ChatConfig{}
	if config != nil {
		cfg = *config
	}
	cfg.Model = geminiLoopModel
	return p.Provider.CompleteWithSystem(ctx, system, messages, &cfg)
}

type geminiLoopContent struct {
	Role  string            `json:"role"`
	Parts []json.RawMessage `json:"parts"`
}

type geminiLoopRequest struct {
	Contents []geminiLoopContent `json:"contents"`
}

type geminiLoopPart struct {
	Text         string `json:"text"`
	FunctionCall *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Args struct {
			Round string `json:"round"`
		} `json:"args"`
	} `json:"functionCall"`
	FunctionResponse *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Response struct {
			Output string `json:"output"`
			Error  string `json:"error"`
		} `json:"response"`
	} `json:"functionResponse"`
}

func TestGeminiToolLoopMissingResponseID(t *testing.T) {
	// Given: real protocol components, local upstream, no system/signature/error frames.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(requests.Add(1))
		wantURL := "/v1beta/models/" + geminiLoopModel + ":streamGenerateContent?alt=sse"
		if r.URL.RequestURI() != wantURL {
			t.Errorf("request%d URL=%q want=%q", n, r.URL.RequestURI(), wantURL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.Logf("request%d fixture-model URL assertion passed: %s", n, r.URL.RequestURI())
		var body geminiLoopRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		decoded, err := json.Marshal(body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		t.Logf("request%d decoded actual localhost body: %s", n, decoded)
		var got []string
		for _, content := range body.Contents {
			for _, raw := range content.Parts {
				var part geminiLoopPart
				if err := json.Unmarshal(raw, &part); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if c := part.FunctionCall; c != nil {
					got = append(got, "call:"+c.ID+":"+c.Name+":"+c.Args.Round)
				}
				if f := part.FunctionResponse; f != nil {
					got = append(got, "result:"+f.ID+":"+f.Name+":"+f.Response.Output+":"+f.Response.Error)
				}
			}
		}
		wants := [][]string{nil, {"call:call_A:inspect_state:A", "result:call_A:inspect_state:result_A:"}, {"call:call_A:inspect_state:A", "result:call_A:inspect_state:result_A:", "call:call_B:inspect_state:B", "result:call_B:inspect_state:result_B:"}}
		if n > 3 {
			t.Errorf("request count exceeded three: %d", n)
			w.WriteHeader(400)
			return
		}
		if !reflect.DeepEqual(got, wants[n-1]) {
			t.Errorf("request%d result correlation: got=%q want=%q", n, got, wants[n-1])
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.Logf("REQUEST_ASSERTION_%d passed; sanitized pairs=%q", n, got)
		response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"finished"}]},"finishReason":"STOP"}]}`
		if n < 3 {
			round := []string{"A", "B"}[n-1]
			response = fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_%s","name":"inspect_state","args":{"round":"%s"}}}]},"finishReason":"STOP"}]}`, round, round)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprintf(w, "data: %s\n\n", response); err != nil {
			t.Error(err)
		}
	}))
	defer func() { server.Close(); t.Log("TEARDOWN: httptest server closed; no persistent process") }()
	p := geminiLoopProvider{geminiprovider.NewProviderWithOptions(geminiprovider.WithAPIKey("dummy"), geminiprovider.WithBaseURL(server.URL))}
	history := geminiLoopRequest{Contents: []geminiLoopContent{{Role: "user", Parts: []json.RawMessage{json.RawMessage(`{"text":"inspect fixture"}`)}}}}
	// When: the caller executes only an in-memory stub and appends results without IDs.
	for round := range 3 {
		body, err := json.Marshal(history)
		if err != nil {
			t.Fatal(err)
		}
		frames, err := RelayStream(ctx, p, body, codec.FormatGemini, codec.FormatGemini)
		if err != nil {
			t.Fatalf("request%d: %v", round+1, err)
		}
		var aggregate bytes.Buffer
		for frame := range frames {
			aggregate.Write(frame)
		}
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		scanner := provider.NewSSEScanner(io.NopCloser(bytes.NewReader(aggregate.Bytes())))
		t.Cleanup(func() {
			if err := scanner.Close(); err != nil {
				t.Error(err)
			}
		})
		model := geminiLoopContent{Role: "model"}
		for {
			_, data, done, err := scanner.Next()
			if done || err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Candidates []struct {
					Content geminiLoopContent `json:"content"`
				} `json:"candidates"`
			}
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			for _, candidate := range response.Candidates {
				model.Parts = append(model.Parts, candidate.Content.Parts...)
			}
		}
		if err := scanner.Close(); err != nil {
			t.Fatal(err)
		}
		if len(model.Parts) != 1 {
			t.Fatalf("round%d aggregate parts=%s", round+1, aggregate.Bytes())
		}
		var part geminiLoopPart
		if err := json.Unmarshal(model.Parts[0], &part); err != nil {
			t.Fatal(err)
		}
		if round == 2 {
			if part.Text != "finished" || part.FunctionCall != nil {
				t.Fatalf("final part=%s", model.Parts[0])
			}
			break
		}
		if part.FunctionCall == nil || part.FunctionCall.Name != "inspect_state" {
			t.Fatalf("unexpected call part=%s", model.Parts[0])
		}
		var reply struct {
			FunctionResponse struct {
				Name     string `json:"name"`
				Response struct {
					Output string `json:"output"`
				} `json:"response"`
			} `json:"functionResponse"`
		}
		reply.FunctionResponse.Name = part.FunctionCall.Name
		reply.FunctionResponse.Response.Output = "result_" + part.FunctionCall.Args.Round
		result, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		history.Contents = append(history.Contents, model, geminiLoopContent{Role: "user", Parts: []json.RawMessage{result}})
	}
	// Then: exactly three upstream requests, each body asserted above, and final text.
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests=%d want=3", got)
	}
	t.Log("TEARDOWN: all three relay channels drained; scanners closed; context cancel deferred")
}

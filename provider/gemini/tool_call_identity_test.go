package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

const task2CallFrame = `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"inspect_state","args":{"slot":1}}},{"functionCall":{"name":"inspect_state","args":{"slot":2}}},{"functionCall":{"id":"explicit_A","name":"inspect_state","args":{}}}]},"finishReason":"STOP"}]}`

func task2IdentityProvider(t *testing.T) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, ":streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			if _, err := fmt.Fprintf(w, "data: %s\n\n", task2CallFrame); err != nil {
				t.Error(err)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, task2CallFrame); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return newTestProvider(server)
}

func task2ResponseIDs(t *testing.T, p *Provider, stream bool) []string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	history := []provider.Message{
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "gemini_call_inspect_state_0", Function: provider.FunctionCall{Name: "inspect_state", Arguments: "{}"}}}},
		{Role: provider.RoleTool, ToolCallID: "gemini_call_inspect_state_0", ToolName: "inspect_state", Content: "ready"},
	}
	config := &provider.ChatConfig{Model: "fixture"}
	var ids []string
	if stream {
		for _, event := range collectTask2Events(t, p.Chat(ctx, history, config)) {
			if event.Type == provider.StreamTypeError {
				t.Fatalf("Chat error: %v", event.Err)
			}
			if event.Delta.Type == provider.StreamDeltaTypeToolCall {
				call, ok := event.Delta.Data.(provider.ToolCallData)
				if !ok {
					t.Fatalf("tool data type=%T", event.Delta.Data)
				}
				ids = append(ids, call.ID)
			}
		}
	} else {
		result, err := p.Complete(ctx, history, config)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range result.ToolCalls {
			ids = append(ids, call.ID)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("call count=%d want=3", len(ids))
	}
	if ids[2] != "explicit_A" {
		t.Errorf("explicit ID changed: %q", ids[2])
	}
	for _, id := range ids[:2] {
		if id == "" || id == history[0].ToolCalls[0].ID {
			t.Errorf("empty or history-colliding ID: %q", id)
		}
	}
	first, firstOrdinal, firstOK := strings.Cut(strings.TrimPrefix(ids[0], "gemini_call_"), "_")
	second, secondOrdinal, secondOK := strings.Cut(strings.TrimPrefix(ids[1], "gemini_call_"), "_")
	a, errA := strconv.ParseUint(firstOrdinal, 10, 64)
	b, errB := strconv.ParseUint(secondOrdinal, 10, 64)
	if !firstOK || !secondOK || first == "" || first != second || errA != nil || errB != nil || b <= a {
		t.Errorf("expected one namespace and increasing ordinal, got %q", ids[:2])
	}
	return ids[:2]
}

func TestGeminiToolCallIdentityConsecutiveResponses(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream_%t", stream), func(t *testing.T) {
			// Given
			p := task2IdentityProvider(t)
			// When
			ids := append(task2ResponseIDs(t, p, stream), task2ResponseIDs(t, p, stream)...)
			// Then
			seen := make(map[string]bool)
			for _, id := range ids {
				if seen[id] {
					t.Errorf("response/call ID reused: %q", id)
				}
				seen[id] = true
			}
		})
	}
}

func TestGeminiToolCallIdentityConcurrentProvider(t *testing.T) {
	// Given
	p := task2IdentityProvider(t)
	ids := make(chan []string, 12)
	// When
	t.Run("requests", func(t *testing.T) {
		for i := range 12 {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				ids <- task2ResponseIDs(t, p, i%2 == 0)
			})
		}
	})
	close(ids)
	// Then
	seen := make(map[string]bool)
	for response := range ids {
		for _, id := range response {
			if seen[id] {
				t.Errorf("concurrent response ID reused: %q", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != 24 {
		t.Errorf("distinct IDs=%d want=24", len(seen))
	}
}

func TestGeminiToolCallIdentityAcrossChunks(t *testing.T) {
	// Given
	frame := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"inspect_state","args":{}}}]}}]}`
	server := mockGeminiServer(t, 200, []string{frame, frame, `{"candidates":[{"finishReason":"STOP"}]}`})
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// When
	events := collectTask2Events(t, newTestProvider(server).Chat(ctx, nil, &provider.ChatConfig{Model: "fixture"}))
	// Then
	var ids []string
	for _, event := range events {
		if event.Delta.Type == provider.StreamDeltaTypeToolCall {
			call, ok := event.Delta.Data.(provider.ToolCallData)
			if !ok {
				t.Fatalf("tool data type=%T", event.Delta.Data)
			}
			ids = append(ids, call.ID)
		}
	}
	if len(ids) != 2 || ids[0] == "" || ids[0] == ids[1] {
		t.Fatalf("distinct full Parts need distinct nonempty IDs: %q", ids)
	}
}

func TestGeminiUnspecifiedFinishBeforeContent(t *testing.T) {
	for _, tool := range []bool{false, true} {
		t.Run(fmt.Sprintf("tool_%t", tool), func(t *testing.T) {
			// Given
			part, want := `{"text":"ready"}`, provider.FinishReasonStop
			if tool {
				part, want = `{"functionCall":{"id":"call_A","name":"inspect_state","args":{}}}`, provider.FinishReasonToolCalls
			}
			server := mockGeminiServer(t, 200, []string{
				`{"candidates":[{"finishReason":"FINISH_REASON_UNSPECIFIED"}]}`,
				fmt.Sprintf(`{"candidates":[{"content":{"parts":[%s]}}]}`, part),
				`{"candidates":[{"finishReason":"STOP"}]}`,
			})
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// When
			events := collectTask2Events(t, newTestProvider(server).Chat(ctx, nil, &provider.ChatConfig{Model: "fixture"}))
			// Then
			stops, dones, content := 0, 0, false
			for _, event := range events {
				switch event.Type {
				case provider.StreamTypeDelta:
					content = true
				case provider.StreamTypeStop:
					stops++
					if !content || event.FinishReason != want {
						t.Errorf("premature/wrong Stop: content=%t reason=%q want=%q", content, event.FinishReason, want)
					}
				case provider.StreamTypeDone:
					dones++
				case provider.StreamTypeError:
					t.Errorf("unexpected error: %v", event.Err)
				}
			}
			if stops != 1 || dones != 1 {
				t.Errorf("Stop=%d Done=%d want=1 each", stops, dones)
			}
		})
	}
}

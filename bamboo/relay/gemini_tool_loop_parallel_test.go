package relay

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestGeminiToolLoopParallelSignatureReplay(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			// Given: native signature metadata precedes the first of two same-name calls.
			f := newGeminiLoopWire(t, stream, []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"","thoughtSignature":"fixture-signature"},{"functionCall":{"id":"call_A","name":"inspect_state","args":{"round":"A"}}},{"functionCall":{"id":"call_B","name":"inspect_state","args":{"round":"B"}}}]},"finishReason":"STOP"}]}`,
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"finished"}]},"finishReason":"STOP"}]}`,
			})
			history := geminiLoopRequest{Contents: []geminiLoopContent{{Role: "user", Parts: []json.RawMessage{json.RawMessage(`{"text":"inspect fixture"}`)}}}}
			// When: replay exact returned Parts with same-name results omitting IDs.
			out := f.exchange(geminiLoopJSON(t, history))
			<-f.captures
			parts := geminiLoopParts(t, []geminiLoopContent{out.model})
			if len(parts) != 2 || len(out.errors) != 0 || !reflect.DeepEqual(out.finishes, []string{"STOP"}) {
				t.Fatalf("parallel output=%+v", out)
			}
			var results []json.RawMessage
			for i, part := range parts {
				label := []string{"A", "B"}[i]
				if part.FunctionCall == nil || part.FunctionCall.ID != "call_"+label || part.FunctionCall.Args.Round != label || part.FunctionCall.Name != "inspect_state" || part.Thought {
					t.Fatalf("parallel part=%s", out.model.Parts[i])
				}
				wantSignature := ""
				if i == 0 {
					wantSignature = "fixture-signature"
				}
				if part.ThoughtSignature != wantSignature {
					t.Fatalf("part%d signature=%q", i, part.ThoughtSignature)
				}
				results = append(results, json.RawMessage(fmt.Sprintf(`{"functionResponse":{"name":"inspect_state","response":{"output":"result_%s"}}}`, part.FunctionCall.Args.Round)))
			}
			history.Contents = append(history.Contents, out.model, geminiLoopContent{Role: "user", Parts: results})
			final := f.exchange(geminiLoopJSON(t, history))
			capture := <-f.captures
			// Then: both pairs and only the first call's signature survive real upstream replay.
			want := []string{"call:call_A:inspect_state:A", "call:call_B:inspect_state:B", "result:call_A:inspect_state:result_A:", "result:call_B:inspect_state:result_B:"}
			if got := geminiLoopPairs(t, capture.Contents); !reflect.DeepEqual(got, want) {
				t.Fatalf("pairs=%q want=%q", got, want)
			}
			signatures := 0
			for _, part := range geminiLoopParts(t, capture.Contents) {
				if part.ThoughtSignature != "" {
					signatures++
					if part.ThoughtSignature != "fixture-signature" || part.FunctionCall == nil || part.FunctionCall.ID != "call_A" || part.Thought {
						t.Fatalf("replayed signature part=%+v", part)
					}
				}
			}
			if signatures != 1 {
				t.Fatalf("replayed signatures=%d", signatures)
			}
			if len(final.model.Parts) != 1 || geminiLoopParts(t, []geminiLoopContent{final.model})[0].Text != "finished" || len(final.errors) != 0 || !reflect.DeepEqual(final.finishes, []string{"STOP"}) {
				t.Fatalf("final=%+v", final)
			}
			if f.requests.Load() != 2 {
				t.Fatalf("requests=%d", f.requests.Load())
			}
		})
	}
}

func TestGeminiToolLoopResultHistories(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, results string
			want          []string
		}{
			{"unknown_explicit", `{"role":"user","parts":[{"functionResponse":{"id":"unknown","name":"inspect_state","response":{"output":"must_not_match"}}}]}`, nil},
			{"failed_payload", `{"role":"user","parts":[{"functionResponse":{"name":"inspect_state","response":{"error":{"code":"STUB_FAILED","detail":"benign failure"}}}}]}`, []string{"call:call_A:inspect_state:A", `result:call_A:inspect_state:{"error":{"code":"STUB_FAILED","detail":"benign failure"}}:`}},
			{"split_missing_B_explicit_A", `{"role":"user","parts":[{"functionResponse":{"name":"inspect_state","response":{"output":"result_B"}}}]},{"role":"user","parts":[{"functionResponse":{"id":"call_A","name":"inspect_state","response":{"output":"result_A"}}}]}`, []string{"call:call_A:inspect_state:A", "call:call_B:inspect_state:B", "result:call_A:inspect_state:result_A:", "result:call_B:inspect_state:result_B:"}},
		} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, tc.name), func(t *testing.T) {
				// Given: caller histories are intentionally not normalized by the harness.
				calls := `{"functionCall":{"id":"call_A","name":"inspect_state","args":{"round":"A"}}}`
				if tc.name == "split_missing_B_explicit_A" {
					calls += `,{"functionCall":{"id":"call_B","name":"inspect_state","args":{"round":"B"}}}`
				}
				body := []byte(`{"contents":[{"role":"user","parts":[{"text":"inspect fixture"}]},{"role":"model","parts":[` + calls + `]},` + tc.results + `]}`)
				f := newGeminiLoopWire(t, stream, []string{`{"candidates":[{"content":{"role":"model","parts":[{"text":"finished"}]},"finishReason":"STOP"}]}`})
				// When: actual codec, facade and provider build the next request.
				out := f.exchange(body)
				capture := <-f.captures
				// Then: invalid explicit IDs stay rejected; valid distinct payloads retain exact identity.
				if got := geminiLoopPairs(t, capture.Contents); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("pairs=%q want=%q", got, tc.want)
				}
				if len(out.errors) != 0 || len(out.model.Parts) != 1 || geminiLoopParts(t, []geminiLoopContent{out.model})[0].Text != "finished" {
					t.Fatalf("output=%+v", out)
				}
				if f.requests.Load() != 1 {
					t.Fatalf("requests=%d", f.requests.Load())
				}
			})
		}
	}
}

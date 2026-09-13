package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	geminiprovider "github.com/bamboo-services/bamboo-messages/provider/gemini"
)

func TestGeminiToolHistoryBaselineExplicitPair(t *testing.T) {
	// Given: an explicit pair already supported before correlation repair.
	body := []byte(`{"contents":[{"role":"model","parts":[{"functionCall":{"id":"call_A","name":"inspect_state","args":{"round":"A"}}}]},{"role":"user","parts":[{"functionResponse":{"id":"call_A","name":"inspect_state","response":{"output":"result_A"}}}]}]}`)
	// When
	req, err := Codec.ParseRequest(body)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	call := mustToolUseBlock(t, req.Messages[0].Content[0])
	result := mustToolResultBlock(t, req.Messages[1].Content[0])
	if call.ID != "call_A" || result.ToolUseID != "call_A" || result.ToolName != "inspect_state" || result.Content != "result_A" {
		t.Fatalf("call=%+v result=%+v", call, result)
	}
}

func historyCall(id, name string) geminiPart {
	return geminiPart{FunctionCall: &geminiFunctionCall{ID: id, Name: name, Args: json.RawMessage(`{}`)}}
}

func historyResult(id, name, output string) geminiPart {
	raw, err := json.Marshal(output)
	if err != nil {
		panic(err)
	}
	return geminiPart{FunctionResponse: &geminiFuncResponse{ID: id, Name: name, Response: raw}}
}

func historyContent(role string, parts ...geminiPart) geminiContent {
	return geminiContent{Role: role, Parts: parts}
}

func historyBlocks(messages []bamboo.BambooMessage) []string {
	var out []string
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch b := block.(type) {
			case *bamboo.ToolUseBlock:
				out = append(out, "call:"+b.ID+":"+b.Name)
			case *bamboo.ToolResultBlock:
				out = append(out, "result:"+b.ToolUseID+":"+b.ToolName+":"+b.Content)
			case *bamboo.TextBlock:
				out = append(out, "text:"+b.Text)
			case *bamboo.ImageBlock:
				out = append(out, "image:"+b.Source.Data)
			default:
				out = append(out, fmt.Sprintf("unexpected:%T", b))
			}
		}
	}
	return out
}

func TestGeminiToolHistoryPairing(t *testing.T) {
	for _, role := range []string{"user", "function"} {
		t.Run(role, func(t *testing.T) {
			a, b := historyCall("A", "inspect"), historyCall("B", "inspect")
			model := historyContent("model", a, b)
			one := historyContent("model", a)
			r := func(parts ...geminiPart) geminiContent { return historyContent(role, parts...) }
			ra, rb := historyResult("A", "inspect", "a"), historyResult("B", "inspect", "b")
			na, nb := historyResult("", "inspect", "a"), historyResult("", "inspect", "b")
			// Given: exact expected identities and original part order, independent of parser logic.
			tests := []struct {
				name     string
				contents []geminiContent
				want     []string
			}{
				{"explicit_call_missing_result", []geminiContent{one, r(na)}, []string{"call:A:inspect", "result:A:inspect:a"}},
				{"both_absent", []geminiContent{historyContent("model", historyCall("", "inspect")), r(na)}, []string{"call:gemini_call_inspect_0:inspect", "result:gemini_call_inspect_0:inspect:a"}},
				{"both_explicit", []geminiContent{one, r(ra)}, []string{"call:A:inspect", "result:A:inspect:a"}},
				{"mixed_batch", []geminiContent{model, r(nb, ra)}, []string{"call:A:inspect", "call:B:inspect", "result:B:inspect:b", "result:A:inspect:a"}},
				{"split_explicit_reservation", []geminiContent{model, r(nb), r(ra)}, []string{"call:A:inspect", "call:B:inspect", "result:B:inspect:b", "result:A:inspect:a"}},
				{"reversed_explicit", []geminiContent{model, r(rb, ra)}, []string{"call:A:inspect", "call:B:inspect", "result:B:inspect:b", "result:A:inspect:a"}},
				{"parallel_positional", []geminiContent{model, r(na, nb)}, []string{"call:A:inspect", "call:B:inspect", "result:A:inspect:a", "result:B:inspect:b"}},
				{"split_positional", []geminiContent{model, r(na), r(nb)}, []string{"call:A:inspect", "call:B:inspect", "result:A:inspect:a", "result:B:inspect:b"}},
				{"two_rounds", []geminiContent{one, r(na), historyContent("model", b), r(nb)}, []string{"call:A:inspect", "result:A:inspect:a", "call:B:inspect", "result:B:inspect:b"}},
				{"two_generated_rounds", []geminiContent{historyContent("model", historyCall("", "inspect")), r(na), historyContent("model", historyCall("", "inspect")), r(nb)}, []string{"call:gemini_call_inspect_0:inspect", "result:gemini_call_inspect_0:inspect:a", "call:gemini_call_inspect_1:inspect", "result:gemini_call_inspect_1:inspect:b"}},
				{"mixed_media_run", []geminiContent{model, r(geminiPart{Text: "before"}, nb, geminiPart{InlineData: &geminiInlineData{MimeType: "image/png", Data: "aA=="}}), r(ra, geminiPart{Text: "after"})}, []string{"call:A:inspect", "call:B:inspect", "text:before", "result:B:inspect:b", "image:aA==", "result:A:inspect:a", "text:after"}},
				{"fill_explicit_name", []geminiContent{one, r(historyResult("A", "", "a"))}, []string{"call:A:inspect", "result:A:inspect:a"}},
				{"wrong_id", []geminiContent{one, r(historyResult("X", "inspect", "bad"), na)}, []string{"call:A:inspect", "result::inspect:bad", "result:A:inspect:a"}},
				{"wrong_name", []geminiContent{one, r(historyResult("A", "other", "bad"), na)}, []string{"call:A:inspect", "result::other:bad", "result:A:inspect:a"}},
				{"duplicate_explicit", []geminiContent{model, r(nb, ra), r(historyResult("A", "inspect", "duplicate"))}, []string{"call:A:inspect", "call:B:inspect", "result:B:inspect:b", "result:A:inspect:a", "result::inspect:duplicate"}},
				{"extra_missing", []geminiContent{one, r(na, nb)}, []string{"call:A:inspect", "result:A:inspect:a", "result::inspect:b"}},
				{"before_future_call", []geminiContent{r(ra, na), one}, []string{"result::inspect:a", "result::inspect:a", "call:A:inspect"}},
				{"future_id_in_active_run", []geminiContent{one, r(rb), historyContent("model", b)}, []string{"call:A:inspect", "result::inspect:b", "call:B:inspect"}},
				{"pure_user_ends_group", []geminiContent{one, historyContent("user", geminiPart{Text: "boundary"}), r(ra, na)}, []string{"call:A:inspect", "text:boundary", "result::inspect:a", "result::inspect:a"}},
				{"model_ends_group", []geminiContent{one, historyContent("model", geminiPart{Text: "boundary"}), r(ra)}, []string{"call:A:inspect", "text:boundary", "result::inspect:a"}},
				{"old_group_id", []geminiContent{one, r(na), historyContent("model", b), r(ra, nb)}, []string{"call:A:inspect", "result:A:inspect:a", "call:B:inspect", "result::inspect:a", "result:B:inspect:b"}},
				{"standalone_explicit", []geminiContent{r(ra)}, []string{"result:A:inspect:a"}},
				{"standalone_missing", []geminiContent{r(na)}, []string{"result::inspect:a"}},
				{"missing_name_no_fallback", []geminiContent{one, r(historyResult("", "", "bad"))}, []string{"call:A:inspect", "result:::bad"}},
				{"later_synthetic_collision", []geminiContent{historyContent("model", historyCall("", "inspect")), r(na), historyContent("model", historyCall("gemini_call_inspect_0", "inspect")), r(nb)}, []string{"call:gemini_call_inspect_1:inspect", "result:gemini_call_inspect_1:inspect:a", "call:gemini_call_inspect_0:inspect", "result:gemini_call_inspect_0:inspect:b"}},
				{"explicit_duplicate_calls_unchanged", []geminiContent{historyContent("model", a, a), r(ra, ra)}, []string{"call:A:inspect", "call:A:inspect", "result:A:inspect:a", "result::inspect:a"}},
				{"call_in_result_content_not_active", []geminiContent{one, r(historyCall("B", "inspect"), ra), r(nb)}, []string{"call:A:inspect", "call:B:inspect", "result:A:inspect:a", "result::inspect:b"}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					body, err := json.Marshal(geminiRequest{Contents: tt.contents})
					if err != nil {
						t.Fatal(err)
					}
					// When
					req, err := Codec.ParseRequest(body)
					// Then
					if err != nil {
						t.Fatal(err)
					}
					if got := historyBlocks(req.Messages); !reflect.DeepEqual(got, tt.want) {
						t.Fatalf("blocks=%q want=%q", got, tt.want)
					}
				})
			}
		})
	}
}

func TestGeminiToolHistoryReuseConcurrent(t *testing.T) {
	for i := range 32 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			// Given: the singleton codec sees distinct requests and repeated ID-less histories.
			for round := range 5 {
				name := fmt.Sprintf("inspect_%d_%d", i, round)
				contents := []geminiContent{historyContent("model", historyCall("", name)), historyContent("user", historyResult("", name, "result"))}
				body, err := json.Marshal(geminiRequest{Contents: contents})
				if err != nil {
					t.Fatal(err)
				}
				// When
				req, err := Codec.ParseRequest(body)
				// Then
				if err != nil {
					t.Fatal(err)
				}
				want := []string{"call:gemini_call_" + name + "_0:" + name, "result:gemini_call_" + name + "_0:" + name + ":result"}
				if got := historyBlocks(req.Messages); !reflect.DeepEqual(got, want) {
					t.Fatalf("blocks=%q want=%q", got, want)
				}
			}
		})
	}
}

func TestGeminiToolHistoryInputUnchanged(t *testing.T) {
	// Given
	contents := []geminiContent{historyContent("model", historyCall("", "inspect")), historyContent("user", historyResult("", "inspect", "result"))}
	before, err := json.Marshal(contents)
	if err != nil {
		t.Fatal(err)
	}
	// When
	_, err = parseContents(contents)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(contents)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("input DTO mutated: %s -> %s", before, after)
	}
}

func TestGeminiToolHistorySplitUpstream(t *testing.T) {
	for _, role := range []string{"user", "function"} {
		t.Run(role, func(t *testing.T) {
			// Given: B's ID-less result precedes A's explicit result in another content.
			contents := []geminiContent{historyContent("model", historyCall("A", "inspect"), historyCall("B", "inspect")), historyContent(role, historyResult("", "inspect", "result_B")), historyContent(role, historyResult("A", "inspect", "result_A"))}
			captured := make(chan geminiRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body geminiRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				captured <- body
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			body, err := json.Marshal(geminiRequest{Contents: contents})
			if err != nil {
				t.Fatal(err)
			}
			req, err := Codec.ParseRequest(body)
			if err != nil {
				t.Fatal(err)
			}
			req.Config.Model = "fixture-model"
			client := bamboo.NewClient(geminiprovider.NewProviderWithOptions(geminiprovider.WithAPIKey("dummy"), geminiprovider.WithBaseURL(server.URL)))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			// When: real facade filtering and provider HTTP body construction.
			_, err = client.Complete(ctx, req.Messages, "", req.Config)
			// Then
			if err != nil {
				t.Fatal(err)
			}
			var wire geminiRequest
			select {
			case wire = <-captured:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var pairs []string
			for _, content := range wire.Contents {
				for _, part := range content.Parts {
					if r := part.FunctionResponse; r != nil {
						pairs = append(pairs, r.ID+":"+r.Name+":"+serializeFuncResponse(r.Response))
					}
				}
			}
			t.Logf("decoded localhost response pairs=%q", pairs)
			if !reflect.DeepEqual(pairs, []string{"A:inspect:result_A", "B:inspect:result_B"}) {
				t.Fatalf("upstream pairs=%q", pairs)
			}
		})
	}
}

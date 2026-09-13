package relay

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestGeminiToolLoopThreeRequests(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, system := range []bool{false, true} {
			for _, generated := range []bool{false, true} {
				for _, explicitResult := range []bool{false, true} {
					t.Run(fmt.Sprintf("stream=%t/system=%t/generated=%t/explicit_result=%t", stream, system, generated, explicitResult), func(t *testing.T) {
						// Given: two distinct same-name calls, followed by final text.
						responses := make([]string, 0, 3)
						for _, round := range []string{"A", "B"} {
							id := fmt.Sprintf(`"id":"call_%s",`, round)
							if generated {
								id = ""
							}
							responses = append(responses, fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{%s"name":"inspect_state","args":{"round":"%s"}}}]},"finishReason":"STOP"}]}`, id, round))
						}
						responses = append(responses, `{"candidates":[{"content":{"role":"model","parts":[{"text":"finished"}]},"finishReason":"STOP"}]}`)
						f := newGeminiLoopWire(t, stream, responses)
						history := geminiLoopWireRequest{geminiLoopRequest: geminiLoopRequest{Contents: []geminiLoopContent{{Role: "user", Parts: []json.RawMessage{json.RawMessage(`{"text":"inspect fixture"}`)}}}}}
						if system {
							history.SystemInstruction = &geminiLoopContent{Parts: []json.RawMessage{json.RawMessage(`{"text":"fixture-system"}`)}}
						}
						var wantPairs []string
						seen := make(map[string]bool)
						// When: replay returned Parts verbatim, execute only an in-memory stub.
						for round := range 3 {
							out := f.exchange(geminiLoopJSON(t, history))
							capture := <-f.captures
							// Then: exact previous pairs, no repeated initial history or dummy result.
							if got := geminiLoopPairs(t, capture.Contents); !reflect.DeepEqual(got, wantPairs) {
								t.Fatalf("round%d pairs=%q want=%q", round+1, got, wantPairs)
							}
							if len(capture.Contents) != 1+2*round {
								t.Fatalf("round%d contents=%d", round+1, len(capture.Contents))
							}
							if (capture.SystemInstruction != nil) != system {
								t.Fatalf("system presence=%v want=%v", capture.SystemInstruction != nil, system)
							}
							if len(out.errors) != 0 || !reflect.DeepEqual(out.finishes, []string{"STOP"}) || len(out.model.Parts) != 1 {
								t.Fatalf("unexpected output: %+v", out)
							}
							part := geminiLoopParts(t, []geminiLoopContent{out.model})[0]
							if round == 2 {
								if part.Text != "finished" || part.FunctionCall != nil {
									t.Fatalf("final part=%s", out.model.Parts[0])
								}
								break
							}
							label := []string{"A", "B"}[round]
							call := part.FunctionCall
							if call == nil || call.ID == "" || call.Name != "inspect_state" || call.Args.Round != label {
								t.Fatalf("call=%s", out.model.Parts[0])
							}
							if seen[call.ID] {
								t.Fatalf("cross-round ID collision: %s", call.ID)
							}
							seen[call.ID] = true
							if !generated && call.ID != "call_"+label {
								t.Fatalf("explicit ID changed: %s", call.ID)
							}
							resultID := ""
							if explicitResult {
								resultID = fmt.Sprintf(`"id":%q,`, call.ID)
							}
							result := json.RawMessage(fmt.Sprintf(`{"functionResponse":{%s"name":"inspect_state","response":{"output":"result_%s"}}}`, resultID, call.Args.Round))
							history.Contents = append(history.Contents, out.model, geminiLoopContent{Role: "user", Parts: []json.RawMessage{result}})
							wantPairs = append(wantPairs, "call:"+call.ID+":inspect_state:"+label, "result:"+call.ID+":inspect_state:result_"+label+":")
						}
						if got := f.requests.Load(); got != 3 {
							t.Fatalf("requests=%d want=3", got)
						}
					})
				}
			}
		}
	}
}

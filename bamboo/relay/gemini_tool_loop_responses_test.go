package relay

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/responses"
)

func TestGeminiToolLoopResponsesExplicitIDControl(t *testing.T) {
	// Given: Responses input explicitly pairs call_id, without any signature claim.
	f := newGeminiLoopWire(t, false, []string{`{"candidates":[{"content":{"role":"model","parts":[{"text":"finished"}]},"finishReason":"STOP"}]}`})
	body := []byte(`{"model":"fixture-model","input":[{"role":"user","content":"inspect fixture"},{"type":"function_call","id":"item_A","call_id":"call_A","name":"inspect_state","arguments":"{\"round\":\"A\"}"},{"type":"function_call_output","call_id":"call_A","output":"result_A"}]}`)
	// When: real Responses codec -> facade -> Gemini HTTP provider -> Responses output.
	data, err := Relay(f.ctx, f.p, body, codec.FormatResponses, codec.FormatResponses)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WIRE_RESPONSES %s", data)
	var response struct {
		Status string `json:"status"`
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "completed" || len(response.Output) != 1 || len(response.Output[0].Content) != 1 || response.Output[0].Content[0].Type != "output_text" || response.Output[0].Content[0].Text != "finished" {
		t.Fatalf("response=%s", data)
	}
	capture := <-f.captures
	// Then: original call_id, name, argument and result survive actual upstream construction.
	want := []string{"call:call_A:inspect_state:A", "result:call_A:inspect_state:result_A:"}
	if got := geminiLoopPairs(t, capture.Contents); !reflect.DeepEqual(got, want) {
		t.Fatalf("pairs=%q want=%q", got, want)
	}
	if f.requests.Load() != 1 {
		t.Fatalf("requests=%d", f.requests.Load())
	}
}

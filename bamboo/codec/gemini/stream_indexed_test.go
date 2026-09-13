package gemini

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/provider"
)

type task3Part struct {
	Text      *string            `json:"text"`
	Thought   bool               `json:"thought"`
	Signature string             `json:"thoughtSignature"`
	Call      *geminiFuncCallOut `json:"functionCall"`
}

type task3Wire struct {
	Parts    []task3Part
	Finishes []string
	Errors   []geminiStreamErrorBody
}

func task3Parse(t *testing.T, raw []byte) task3Wire {
	t.Helper()
	var wire task3Wire
	for _, frame := range strings.Split(string(raw), "\n\n") {
		if frame == "" || strings.HasPrefix(frame, ":") {
			continue
		}
		if !strings.HasPrefix(frame, "data: ") {
			t.Fatalf("invalid SSE frame: %q", frame)
		}
		var chunk struct {
			Candidates []struct {
				Content *struct {
					Parts []task3Part `json:"parts"`
				} `json:"content"`
				Finish string `json:"finishReason"`
			} `json:"candidates"`
			Error *geminiStreamErrorBody `json:"error"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.Error != nil {
			wire.Errors = append(wire.Errors, *chunk.Error)
		}
		for _, candidate := range chunk.Candidates {
			if candidate.Content != nil {
				wire.Parts = append(wire.Parts, candidate.Content.Parts...)
			}
			if candidate.Finish != "" {
				wire.Finishes = append(wire.Finishes, candidate.Finish)
			}
		}
	}
	t.Logf("aggregated SSE: %s", raw)
	return wire
}

func task3Convert(events []provider.StreamEvent) []bamboo.StreamEvent {
	c := bamboo.NewStreamConverterForProvider(provider.ProviderGemini)
	var out []bamboo.StreamEvent
	for _, event := range events {
		out = append(out, c.Convert(event)...)
	}
	return out
}

func task3Delta(delta provider.StreamDelta[any]) provider.StreamEvent {
	return provider.StreamEvent{Type: provider.StreamTypeDelta, Delta: delta}
}

func task3Serialize(t *testing.T, events []bamboo.StreamEvent) task3Wire {
	t.Helper()
	s := Codec.NewSerializer("fixture-model")
	var raw []byte
	for _, event := range events {
		data, err := s.Serialize(event)
		if err != nil {
			t.Fatalf("Serialize(%s index=%d): %v", event.Type, event.Index, err)
		}
		raw = append(raw, data...)
	}
	data, err := s.Flush()
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, data...)
	return task3Parse(t, raw)
}

func TestGeminiStreamIndexedCallsBaselineSequentialConverter(t *testing.T) {
	// Given: two sequential calls through the real converter.
	events := []provider.StreamEvent{
		{Type: provider.StreamTypeStart},
		task3Delta(provider.NewToolCallDeltaWithIndex("call_A", "inspect_state", 0)),
		task3Delta(provider.NewToolCallDeltaDataWithIndex(`{"step":1}`, 0)),
		task3Delta(provider.NewBlockStopDelta(0)),
		task3Delta(provider.NewToolCallDeltaWithIndex("call_B", "inspect_state", 1)),
		task3Delta(provider.NewToolCallDeltaDataWithIndex(`{"step":2}`, 1)),
		task3Delta(provider.NewBlockStopDelta(1)),
		{Type: provider.StreamTypeDone},
	}
	// When
	wire := task3Serialize(t, task3Convert(events))
	// Then
	if len(wire.Parts) != 2 {
		t.Fatalf("parts=%+v", wire.Parts)
	}
	for i, id := range []string{"call_A", "call_B"} {
		call := wire.Parts[i].Call
		if call == nil || call.ID != id || call.Name != "inspect_state" {
			t.Fatalf("call %d=%+v", i, call)
		}
		var args map[string]int
		if err := json.Unmarshal(call.Args, &args); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(args, map[string]int{"step": i + 1}) {
			t.Fatalf("args=%v", args)
		}
	}
	if !reflect.DeepEqual(wire.Finishes, []string{"STOP"}) {
		t.Fatalf("finishes=%v", wire.Finishes)
	}
}

func TestGeminiStreamIndexedCallsOverlappingConverter(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "reverse"}[reverse], func(t *testing.T) {
			// Given
			first, second := 0, 1
			if reverse {
				first, second = second, first
			}
			events := task3Convert([]provider.StreamEvent{
				{Type: provider.StreamTypeStart},
				task3Delta(provider.NewToolCallDeltaWithIndex("call_A", "inspect_state", 8)),
				task3Delta(provider.NewToolCallDeltaDataWithIndex(`{"step":`, 8)),
				task3Delta(provider.NewToolCallDeltaWithIndex("call_B", "inspect_state", 3)),
				task3Delta(provider.NewToolCallDeltaDataWithIndex(`{"step":`, 3)),
				task3Delta(provider.NewToolCallDeltaDataWithIndex(`1}`, 8)),
				task3Delta(provider.NewToolCallDeltaDataWithIndex(`2}`, 3)),
				task3Delta(provider.NewBlockStopDelta(first)),
				task3Delta(provider.NewBlockStopDelta(second)),
				{Type: provider.StreamTypeDone},
			})
			// When
			s := Codec.NewSerializer("")
			var raw []byte
			for _, event := range events {
				data, err := s.Serialize(event)
				if err != nil {
					t.Fatalf("index %d: %v", event.Index, err)
				}
				if reverse && event.Type == bamboo.EventContentBlockStop && event.Index == 1 && len(data) != 0 {
					t.Fatalf("later call escaped before A closed: %s", data)
				}
				raw = append(raw, data...)
			}
			// Then
			wire := task3Parse(t, raw)
			if len(wire.Parts) != 2 {
				t.Fatalf("want 2 calls, got %+v", wire.Parts)
			}
			for i, id := range []string{"call_A", "call_B"} {
				call := wire.Parts[i].Call
				if call == nil || call.ID != id || call.Name != "inspect_state" {
					t.Fatalf("call %d=%+v", i, call)
				}
				var args map[string]int
				if err := json.Unmarshal(call.Args, &args); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(args, map[string]int{"step": i + 1}) {
					t.Fatalf("args=%v", args)
				}
			}
		})
	}
}

func task3Start(index int, id string) bamboo.StreamEvent {
	return bamboo.StreamEvent{Type: bamboo.EventContentBlockStart, Index: index, ContentBlock: bamboo.NewToolUseBlock(id, "inspect_state", nil)}
}

func task3Args(index int, args string) bamboo.StreamEvent {
	return bamboo.StreamEvent{Type: bamboo.EventContentBlockDelta, Index: index, Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: args}}
}

func TestGeminiStreamIndexedCallsStopsAndEmptyArgs(t *testing.T) {
	// Given
	s := Codec.NewSerializer("")
	for _, event := range []bamboo.StreamEvent{task3Start(7, "call_A"), {Type: bamboo.EventContentBlockStart, Index: 2, ContentBlock: bamboo.NewTextBlock("")}} {
		if _, err := s.Serialize(event); err != nil {
			t.Fatal(err)
		}
	}
	// When: unrelated stops must not close the call.
	for _, index := range []int{2, 99} {
		data, err := s.Serialize(bamboo.StreamEvent{Type: bamboo.EventContentBlockStop, Index: index})
		if err != nil || len(data) != 0 {
			t.Fatalf("unrelated stop %d: %s %v", index, data, err)
		}
	}
	data, err := s.Serialize(bamboo.StreamEvent{Type: bamboo.EventContentBlockStop, Index: 7})
	if err != nil {
		t.Fatal(err)
	}
	// Then
	wire := task3Parse(t, data)
	if len(wire.Parts) != 1 || wire.Parts[0].Call == nil || string(wire.Parts[0].Call.Args) != "{}" {
		t.Fatalf("empty args: %+v", wire.Parts)
	}
	for range 2 {
		data, err = s.Serialize(bamboo.StreamEvent{Type: bamboo.EventContentBlockStop, Index: 7})
		if err != nil || len(data) != 0 {
			t.Fatalf("repeated stop: %s %v", data, err)
		}
		data, err = s.Flush()
		if err != nil || len(data) != 0 {
			t.Fatalf("repeated Flush: %s %v", data, err)
		}
	}
}

func TestGeminiStreamIndexedCallsFailureIsSticky(t *testing.T) {
	for _, args := range []string{`{"x":`, `[]`, `null`, `"object"`, `42`, `true`, `{} {}`, " "} {
		t.Run(args, func(t *testing.T) {
			// Given: B is valid but cannot precede A.
			s := Codec.NewSerializer("")
			for _, event := range []bamboo.StreamEvent{task3Start(5, "call_A"), task3Args(5, args), task3Start(2, "call_B"), task3Args(2, `{}`)} {
				if _, err := s.Serialize(event); err != nil {
					t.Fatal(err)
				}
			}
			// When
			data, err := s.Serialize(bamboo.StreamEvent{Type: bamboo.EventContentBlockStop, Index: 5})
			// Then
			var be *bamboo.BambooError
			if !errors.As(err, &be) || len(data) != 0 {
				t.Fatalf("want codec error with zero bytes, got %s %v", data, err)
			}
			task3AssertSuppressed(t, s)
		})
	}
}

func TestGeminiStreamIndexedCallsUnknownAndIncomplete(t *testing.T) {
	for _, terminal := range []string{"unknown", "message_delta", "message_stop", "flush"} {
		t.Run(terminal, func(t *testing.T) {
			// Given
			s := Codec.NewSerializer("")
			if _, err := s.Serialize(task3Start(3, "call_A")); err != nil {
				t.Fatal(err)
			}
			// When
			var data []byte
			var err error
			switch terminal {
			case "unknown":
				data, err = s.Serialize(task3Args(42, `{}`))
			case "message_delta":
				data, err = s.Serialize(bamboo.StreamEvent{Type: bamboo.EventMessageDelta, Delta: &bamboo.MessageDelta{StopReason: bamboo.FinishReasonToolUse}})
			case "message_stop":
				data, err = s.Serialize(bamboo.StreamEvent{Type: bamboo.EventMessageStop})
			case "flush":
				data, err = s.Flush()
			}
			// Then
			var be *bamboo.BambooError
			if !errors.As(err, &be) || len(data) != 0 {
				t.Fatalf("want codec error with zero bytes, got %s %v", data, err)
			}
			task3AssertSuppressed(t, s)
		})
	}
}

func task3AssertSuppressed(t *testing.T, s interface {
	Serialize(bamboo.StreamEvent) ([]byte, error)
	Flush() ([]byte, error)
}) {
	t.Helper()
	for _, event := range []bamboo.StreamEvent{
		{Type: bamboo.EventContentBlockStop, Index: 2}, {Type: bamboo.EventContentBlockStop, Index: 3},
		{Type: bamboo.EventMessageDelta, Delta: &bamboo.MessageDelta{StopReason: bamboo.FinishReasonEndTurn}},
		{Type: bamboo.EventMessageStop}, {Type: bamboo.EventError}, {Type: bamboo.EventMessageStart}, task3Start(9, "new"), task3Args(9, `{}`),
	} {
		data, err := s.Serialize(event)
		if err != nil || len(data) != 0 {
			t.Errorf("after failure %s: %s %v", event.Type, data, err)
		}
	}
	for range 2 {
		data, err := s.Flush()
		if err != nil || len(data) != 0 {
			t.Errorf("after failure Flush: %s %v", data, err)
		}
	}
}

func TestGeminiStreamIndexedCallsMalformedConverterHasNoSuccess(t *testing.T) {
	// Given
	events := task3Convert([]provider.StreamEvent{{Type: provider.StreamTypeStart}, task3Delta(provider.NewSignatureDelta("discard")), task3Delta(provider.NewToolCallDelta("call_A", "inspect_state")), task3Delta(provider.NewToolCallDeltaData(`[]`)), {Type: provider.StreamTypeDone}})
	s := Codec.NewSerializer("")
	var raw []byte
	errorsSeen := 0
	// When: mirror relay's continue-on-local-error policy.
	for _, event := range events {
		data, err := s.Serialize(event)
		if err != nil {
			errorsSeen++
			if len(data) != 0 {
				t.Fatalf("bytes alongside error: %s", data)
			}
			continue
		}
		raw = append(raw, data...)
	}
	// Then
	wire := task3Parse(t, raw)
	if errorsSeen != 1 || len(wire.Parts) != 0 || len(wire.Finishes) != 0 || len(wire.Errors) != 0 {
		t.Fatalf("errors=%d wire=%+v", errorsSeen, wire)
	}
	task3AssertSuppressed(t, s)
}

func TestGeminiStreamIndexedCallsHeldCompletionDiscardedOnFailure(t *testing.T) {
	// Given: valid B completes while A is still incomplete.
	s := Codec.NewSerializer("")
	for _, event := range []bamboo.StreamEvent{task3Start(4, "call_A"), task3Args(4, `{"broken":`), task3Start(1, "call_B"), task3Args(1, `{}`), {Type: bamboo.EventContentBlockStop, Index: 1}} {
		data, err := s.Serialize(event)
		if err != nil || len(data) != 0 {
			t.Fatalf("pending B escaped: %s %v", data, err)
		}
	}
	// When
	data, err := s.Serialize(bamboo.StreamEvent{Type: bamboo.EventContentBlockStop, Index: 4})
	// Then
	if err == nil || len(data) != 0 {
		t.Fatalf("invalid A: %s %v", data, err)
	}
	task3AssertSuppressed(t, s)
}

func TestGeminiStreamIndexedCallsSuccessTerminalOnce(t *testing.T) {
	// Given
	s := Codec.NewSerializer("")
	event := bamboo.StreamEvent{Type: bamboo.EventMessageDelta, Delta: &bamboo.MessageDelta{StopReason: bamboo.FinishReasonEndTurn}, Usage: &bamboo.Usage{InputTokens: 7, OutputTokens: 3, CacheReadInputTokens: 2}}
	// When
	data, err := s.Serialize(event)
	if err != nil {
		t.Fatal(err)
	}
	// Then
	chunk := parseGeminiSSE(t, data)
	if chunk.UsageMetadata == nil || chunk.UsageMetadata.TotalTokenCount != 10 || chunk.UsageMetadata.CachedContentTokenCount != 2 {
		t.Fatalf("usage=%+v", chunk.UsageMetadata)
	}
	if !reflect.DeepEqual(task3Parse(t, data).Finishes, []string{"STOP"}) {
		t.Fatal("missing terminal frame")
	}
	for range 2 {
		data, err = s.Serialize(event)
		if err != nil || len(data) != 0 {
			t.Fatalf("repeated terminal: %s %v", data, err)
		}
		data, err = s.Flush()
		if err != nil || len(data) != 0 {
			t.Fatalf("Flush: %s %v", data, err)
		}
	}
}

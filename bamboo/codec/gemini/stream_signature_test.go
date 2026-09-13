package gemini

import (
	"context"
	"reflect"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/provider"
)

func TestGeminiStreamSignatureAttachmentConverter(t *testing.T) {
	for _, target := range []string{"parallel_calls", "text", "terminal", "late_thinking", "foreign"} {
		t.Run(target, func(t *testing.T) {
			// Given
			events := []provider.StreamEvent{{Type: provider.StreamTypeStart}}
			if target == "late_thinking" {
				events = append(events, task3Delta(provider.NewThinkingDelta("reason")))
			}
			signature := provider.NewSignatureDelta("sig_A")
			if target == "foreign" {
				signature = provider.NewSignatureDeltaWithProvider("foreign", bamboo.SignatureProviderAnthropic)
			}
			events = append(events, task3Delta(signature))
			switch target {
			case "parallel_calls", "late_thinking", "foreign":
				events = append(events, task3Delta(provider.NewToolCallDeltaWithIndex("call_A", "inspect_state", 0)), task3Delta(provider.NewToolCallDeltaDataWithIndex(`{"step":1}`, 0)))
				if target == "parallel_calls" {
					events = append(events, task3Delta(provider.NewToolCallDeltaWithIndex("call_B", "inspect_state", 1)), task3Delta(provider.NewToolCallDeltaDataWithIndex(`{"step":2}`, 1)))
				}
			case "text":
				events = append(events, task3Delta(provider.NewTextDelta("answer")))
			}
			events = append(events, provider.StreamEvent{Type: provider.StreamTypeDone})
			// When
			wire := task3Serialize(t, task3Convert(events))
			// Then
			if !reflect.DeepEqual(wire.Finishes, []string{"STOP"}) {
				t.Fatalf("finishes=%v", wire.Finishes)
			}
			switch target {
			case "parallel_calls":
				if len(wire.Parts) != 2 || wire.Parts[0].Call == nil || wire.Parts[1].Call == nil {
					t.Fatalf("parts=%+v", wire.Parts)
				}
				if wire.Parts[0].Call.ID != "call_A" || wire.Parts[1].Call.ID != "call_B" || wire.Parts[0].Signature != "sig_A" || wire.Parts[1].Signature != "" {
					t.Fatalf("call signature/order: %+v", wire.Parts)
				}
				for i, p := range wire.Parts {
					if p.Thought {
						t.Fatalf("call %d has thought=true", i)
					}
					if string(p.Call.Args) != []string{`{"step":1}`, `{"step":2}`}[i] {
						t.Fatalf("args=%s", p.Call.Args)
					}
				}
			case "text":
				if len(wire.Parts) != 1 || wire.Parts[0].Text == nil || *wire.Parts[0].Text != "answer" || wire.Parts[0].Signature != "sig_A" || wire.Parts[0].Thought {
					t.Fatalf("parts=%+v", wire.Parts)
				}
			case "terminal":
				if len(wire.Parts) != 1 {
					t.Fatalf("parts=%+v", wire.Parts)
				}
				task3AssertEmptySignature(t, wire.Parts[0], "sig_A")
			case "late_thinking":
				if len(wire.Parts) != 3 || wire.Parts[0].Text == nil || *wire.Parts[0].Text != "reason" || !wire.Parts[0].Thought || wire.Parts[1].Call == nil || wire.Parts[1].Signature != "" {
					t.Fatalf("parts=%+v", wire.Parts)
				}
				task3AssertEmptySignature(t, wire.Parts[2], "sig_A")
			case "foreign":
				if len(wire.Parts) != 1 || wire.Parts[0].Call == nil || wire.Parts[0].Signature != "" {
					t.Fatalf("parts=%+v", wire.Parts)
				}
			}
		})
	}
}

func task3AssertEmptySignature(t *testing.T, part task3Part, signature string) {
	t.Helper()
	if part.Text == nil || *part.Text != "" || part.Thought || part.Signature != signature || part.Call != nil {
		t.Fatalf("want explicit empty text signature %q without thought/call, got %+v", signature, part)
	}
}

func TestGeminiStreamSignatureAttachmentNativeSignedThinking(t *testing.T) {
	// Given
	events := []bamboo.StreamEvent{
		{Type: bamboo.EventContentBlockStart, Index: 0, ContentBlock: bamboo.NewThinkingBlockWithProvider("reason", "own", bamboo.SignatureProviderGemini)},
		{Type: bamboo.EventContentBlockStop, Index: 0}, task3Start(1, "call_A"), task3Args(1, `{}`), {Type: bamboo.EventContentBlockStop, Index: 1},
	}
	// When
	wire := task3Serialize(t, events)
	// Then
	if len(wire.Parts) != 2 || wire.Parts[0].Signature != "own" || !wire.Parts[0].Thought || wire.Parts[1].Signature != "" {
		t.Fatalf("parts=%+v", wire.Parts)
	}
}

func TestGeminiStreamSignatureAttachmentIndependentAndFlush(t *testing.T) {
	// Given: two independent native credentials with no target.
	events := task3Convert([]provider.StreamEvent{{Type: provider.StreamTypeStart}, task3Delta(provider.NewSignatureDelta("opaque_A")), task3Delta(provider.NewBlockStopDelta(0)), task3Delta(provider.NewSignatureDelta("opaque_B"))})
	s := Codec.NewSerializer("")
	var raw []byte
	for _, event := range events {
		data, err := s.Serialize(event)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, data...)
	}
	// When
	data, err := s.Flush()
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, data...)
	// Then
	wire := task3Parse(t, raw)
	if len(wire.Parts) != 2 {
		t.Fatalf("parts=%+v", wire.Parts)
	}
	task3AssertEmptySignature(t, wire.Parts[0], "opaque_A")
	task3AssertEmptySignature(t, wire.Parts[1], "opaque_B")
	for range 2 {
		data, err = s.Flush()
		if err != nil || len(data) != 0 {
			t.Fatalf("repeated Flush=%s %v", data, err)
		}
	}
	fresh := task3Serialize(t, []bamboo.StreamEvent{task3Start(0, "fresh"), {Type: bamboo.EventContentBlockStop, Index: 0}})
	if len(fresh.Parts) != 1 || fresh.Parts[0].Signature != "" {
		t.Fatalf("metadata leaked: %+v", fresh.Parts)
	}
}

func TestGeminiStreamSignatureAttachmentErrorSuppressesConverterStops(t *testing.T) {
	for _, upstream := range []*bamboo.BambooError{bamboo.NewBambooError("upstream", "quota exceeded", 429), bamboo.NewBambooError("SDK", context.Canceled.Error(), 0)} {
		t.Run(upstream.Message, func(t *testing.T) {
			// Given: signed pending call followed by error/cancellation, then synthetic stops.
			events := task3Convert([]provider.StreamEvent{{Type: provider.StreamTypeStart}, task3Delta(provider.NewSignatureDelta("discard")), task3Delta(provider.NewToolCallDelta("call_A", "inspect_state")), task3Delta(provider.NewToolCallDeltaData(`{"pending":`)), {Type: provider.StreamTypeError, Err: upstream}, {Type: provider.StreamTypeDone}})
			// When
			s := Codec.NewSerializer("")
			var raw []byte
			for _, event := range events {
				data, err := s.Serialize(event)
				if err != nil {
					t.Fatalf("unexpected local error %v", err)
				}
				raw = append(raw, data...)
			}
			// Then
			wire := task3Parse(t, raw)
			if len(wire.Errors) != 1 || wire.Errors[0].Message != upstream.Message || len(wire.Parts) != 0 || len(wire.Finishes) != 0 {
				t.Fatalf("failed stream=%+v", wire)
			}
			task3AssertSuppressed(t, s)
		})
	}
}

func TestGeminiStreamSignatureAttachmentEmptyTextTarget(t *testing.T) {
	// Given
	events := task3Convert([]provider.StreamEvent{
		{Type: provider.StreamTypeStart}, task3Delta(provider.NewSignatureDelta("empty_target")),
		task3Delta(provider.NewTextDelta("")), {Type: provider.StreamTypeDone},
	})
	// When
	wire := task3Serialize(t, events)
	// Then
	if len(wire.Parts) != 1 {
		t.Fatalf("parts=%+v", wire.Parts)
	}
	task3AssertEmptySignature(t, wire.Parts[0], "empty_target")
}

func TestGeminiStreamSignatureAttachmentCompletedBlockAndIndexProvenance(t *testing.T) {
	// Given: foreign block starts after the native empty block, before its close.
	events := []bamboo.StreamEvent{
		{Type: bamboo.EventContentBlockStart, Index: 8, ContentBlock: bamboo.NewThinkingBlockWithProvider("", "native", bamboo.SignatureProviderGemini)},
		{Type: bamboo.EventContentBlockStart, Index: 2, ContentBlock: bamboo.NewThinkingBlockWithProvider("", "foreign", bamboo.SignatureProviderAnthropic)},
		{Type: bamboo.EventContentBlockDelta, Index: 2, Delta: &bamboo.StreamDelta{Type: bamboo.DeltaSignature, Signature: "foreign_delta"}},
		{Type: bamboo.EventContentBlockStop, Index: 8}, task3Start(5, "call_A"),
		{Type: bamboo.EventContentBlockStop, Index: 5},
	}
	// When
	wire := task3Serialize(t, events)
	// Then
	if len(wire.Parts) != 1 || wire.Parts[0].Call == nil || wire.Parts[0].Call.ID != "call_A" || wire.Parts[0].Signature != "native" || wire.Parts[0].Thought {
		t.Fatalf("parts=%+v", wire.Parts)
	}
}

func TestGeminiStreamSignatureAttachmentIndependentSameIndex(t *testing.T) {
	// Given
	events := task3Convert([]provider.StreamEvent{{Type: provider.StreamTypeStart}, task3Delta(provider.NewSignatureDelta("opaque_A")), task3Delta(provider.NewSignatureDelta("opaque_B")), {Type: provider.StreamTypeDone}})
	// When
	wire := task3Serialize(t, events)
	// Then
	if len(wire.Parts) != 2 {
		t.Fatalf("parts=%+v", wire.Parts)
	}
	task3AssertEmptySignature(t, wire.Parts[0], "opaque_A")
	task3AssertEmptySignature(t, wire.Parts[1], "opaque_B")
}

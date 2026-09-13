package relay

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

func TestGeminiToolLoopWrapperConfigCopy(t *testing.T) {
	for _, method := range []string{"Chat", "ChatWithSystem", "Complete", "CompleteWithSystem"} {
		for _, supplied := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/config=%t", method, supplied), func(t *testing.T) {
				// Given: each wrapper method must route nil or copied caller config to the fixture model.
				stream := method == "Chat" || method == "ChatWithSystem"
				f := newGeminiLoopWire(t, stream, []string{`{"candidates":[{"content":{"role":"model","parts":[{"text":"finished"}]},"finishReason":"STOP"}]}`})
				original := provider.ChatConfig{Model: "caller-model", MaxTokens: 17}
				before := original
				var cfg *provider.ChatConfig
				if supplied {
					cfg = &original
				}
				messages := []provider.Message{{Role: provider.RoleUser, Content: "inspect fixture"}}
				var events <-chan provider.StreamEvent
				var result *provider.CompletionResult
				var err error
				// When: invoke the actual wrapper over the actual Gemini HTTP provider.
				switch method {
				case "Chat":
					events = f.p.Chat(f.ctx, messages, cfg)
				case "ChatWithSystem":
					events = f.p.ChatWithSystem(f.ctx, "fixture-system", messages, cfg)
				case "Complete":
					result, err = f.p.Complete(f.ctx, messages, cfg)
				case "CompleteWithSystem":
					result, err = f.p.CompleteWithSystem(f.ctx, "fixture-system", messages, cfg)
				default:
					t.Fatal("unhandled wrapper method")
				}
				if err != nil {
					t.Fatal(err)
				}
				if stream {
					timer := time.NewTimer(3 * time.Second)
					defer timer.Stop()
					text := ""
					stops, done := 0, 0
				collect:
					for {
						select {
						case event, ok := <-events:
							if !ok {
								break collect
							}
							switch event.Type {
							case provider.StreamTypeError:
								t.Fatalf("provider error=%v", event.Err)
							case provider.StreamTypeStop:
								stops++
							case provider.StreamTypeDone:
								done++
							case provider.StreamTypeDelta:
								if delta, ok := event.Delta.Data.(provider.TextData); ok {
									text += string(delta)
								}
							}
						case <-timer.C:
							t.Fatal("provider channel did not close")
						}
					}
					if text != "finished" || stops != 1 || done != 1 {
						t.Fatalf("text=%q stops=%d done=%d", text, stops, done)
					}
					t.Log("TEARDOWN: direct provider channel closed within 3s")
				} else if result == nil || result.Content != "finished" {
					t.Fatalf("result=%+v", result)
				}
				// Then: actual URL was checked before body, and caller config is unchanged.
				if !reflect.DeepEqual(original, before) {
					t.Fatalf("caller config mutated: %+v", original)
				}
				capture := <-f.captures
				wantSystem := method == "ChatWithSystem" || method == "CompleteWithSystem"
				if (capture.SystemInstruction != nil) != wantSystem {
					t.Fatalf("system presence=%t want=%t", capture.SystemInstruction != nil, wantSystem)
				}
				if f.requests.Load() != 1 {
					t.Fatalf("requests=%d", f.requests.Load())
				}
			})
		}
	}
}

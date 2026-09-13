package bamboo

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

type pendingChatProvider struct {
	mockProviderWithEvents
	events <-chan provider.StreamEvent
}

func (p *pendingChatProvider) Chat(context.Context, []provider.Message, *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.events
}

type pendingChatFixture struct {
	ctx           context.Context
	cancel        context.CancelFunc
	providerCh    chan provider.StreamEvent
	closeProvider func()
	out           <-chan StreamEvent
	toolIndex     int
}

func newPendingChatFixture(t *testing.T) pendingChatFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	providerCh := make(chan provider.StreamEvent, 3)
	closeProvider := sync.OnceFunc(func() { close(providerCh) })
	t.Cleanup(closeProvider)
	providerCh <- provider.StreamEvent{Type: provider.StreamTypeStart}
	providerCh <- provider.StreamEvent{Type: provider.StreamTypeDelta, Delta: provider.NewToolCallDelta("pending_A", "inspect_state")}
	providerCh <- provider.StreamEvent{Type: provider.StreamTypeDelta, Delta: provider.NewToolCallDeltaData(`{"round":"A"}`)}
	out, err := NewClient(&pendingChatProvider{events: providerCh}).Chat(ctx, []BambooMessage{NewUserMessage("inspect fixture")}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []StreamEventType{EventMessageStart, EventContentBlockStart, EventContentBlockDelta} {
		select {
		case event, ok := <-out:
			if !ok || event.Type != want {
				t.Fatalf("pending precondition: event=%+v open=%t want=%s", event, ok, want)
			}
			switch event.Type {
			case EventContentBlockStart:
				tool, ok := event.ContentBlock.(*ToolUseBlock)
				if !ok || tool.ID != "pending_A" || tool.Name != "inspect_state" || event.Index != 0 {
					t.Fatalf("pending tool=%+v index=%d", event.ContentBlock, event.Index)
				}
			case EventContentBlockDelta:
				delta, ok := event.Delta.(*StreamDelta)
				if !ok || delta.Type != DeltaInputJSON || delta.PartialJSON != `{"round":"A"}` || event.Index != 0 {
					t.Fatalf("pending arguments=%+v index=%d", event.Delta, event.Index)
				}
			}
		case <-ctx.Done():
			t.Fatal("pending tool precondition timed out")
		}
	}
	t.Log("PRECONDITION pending_A argument delta consumed; tool index=0 remains open")
	return pendingChatFixture{ctx: ctx, cancel: cancel, providerCh: providerCh, closeProvider: closeProvider, out: out, toolIndex: 0}
}

func drainPendingChat(t *testing.T, out <-chan StreamEvent) []StreamEvent {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	var events []StreamEvent
	for {
		select {
		case event, ok := <-out:
			if !ok {
				t.Logf("DRAIN closed=true events=%+v", events)
				return events
			}
			events = append(events, event)
		case <-timer.C:
			t.Fatal("facade output did not close within independent drain deadline")
		}
	}
}

func TestChatCancelPending_OrdinaryEOFCompletesOnce(t *testing.T) {
	// Given: 已消费参数，但工具块仍未关闭。
	f := newPendingChatFixture(t)
	// When: 未取消的 provider 直接关闭，不发送 Stop/Done。
	f.closeProvider()
	events := drainPendingChat(t, f.out)
	// Then: 工具和消息各完成一次，保留工具完成原因。
	var types []StreamEventType
	for _, event := range events {
		types = append(types, event.Type)
	}
	if !slices.Equal(types, []StreamEventType{EventContentBlockStop, EventMessageDelta, EventMessageStop}) {
		t.Fatalf("EOF lifecycle=%v", types)
	}
	if events[0].Index != f.toolIndex {
		t.Fatalf("closed index=%d want=%d", events[0].Index, f.toolIndex)
	}
	delta, ok := events[1].Delta.(*MessageDelta)
	if !ok || delta.StopReason != FinishReasonToolUse {
		t.Fatalf("EOF completion=%+v", events[1].Delta)
	}
}

func TestChatCancelPending_OrdinaryErrorPrecedesTermination(t *testing.T) {
	// Given: 工具待完成，上游错误具有明确状态码。
	f := newPendingChatFixture(t)
	upstreamErr := pkgErrors.NewBambooError("upstream", "fixture failure", 503)
	// When: provider 发送错误后关闭。
	f.providerCh <- provider.StreamEvent{Type: provider.StreamTypeError, Err: upstreamErr}
	f.closeProvider()
	events := drainPendingChat(t, f.out)
	// Then: 原始错误先于唯一的终止序列。
	var types []StreamEventType
	for _, event := range events {
		types = append(types, event.Type)
	}
	if !slices.Equal(types, []StreamEventType{EventError, EventContentBlockStop, EventMessageDelta, EventMessageStop}) {
		t.Fatalf("error lifecycle=%v", types)
	}
	if events[0].Error != upstreamErr || events[1].Index != f.toolIndex {
		t.Fatalf("error=%+v closed index=%d", events[0].Error, events[1].Index)
	}
}

func TestChatCancelPending_CancelledEOFDoesNotComplete(t *testing.T) {
	// Given: 参数已到达 facade 消费端，工具尚未收到完成事件。
	f := newPendingChatFixture(t)
	// When: 取消已可见后，才允许 provider 关闭，不发送 Stop/Done。
	f.cancel()
	<-f.ctx.Done()
	if f.ctx.Err() != context.Canceled {
		t.Fatalf("cancellation precondition=%v", f.ctx.Err())
	}
	f.closeProvider()
	events := drainPendingChat(t, f.out)
	// Then: 独立超时内关闭，不能把待完成工具转换为成功结果。
	for _, event := range events {
		switch event.Type {
		case EventContentBlockStop:
			if event.Index == f.toolIndex {
				t.Errorf("post-cancel pending tool completed at index=%d", event.Index)
			}
		case EventMessageDelta, EventMessageStop:
			t.Errorf("post-cancel message completion=%s", event.Type)
		}
	}
}

package responses

import (
	"context"
	"sync"
	"time"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// streamDrainTimeout 是收到 response.completed 后等待上游关闭连接的最大时长。
//
// 背景：openai-go SDK 的 Stream.Next() 在收到终止事件后不 break，
// 而是继续 drain 直到底层 HTTP 连接关闭（io.EOF）。部分上游
// 在发送终止事件后不主动关闭 TCP 连接，导致 Stream.Next() 永久阻塞。
const streamDrainTimeout = 5 * time.Second

// Chat 流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Responses 格式，
// 通过底层 SDK 发起流式请求，返回 StreamEvent channel。
func (p *ResponsesProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 在消息前插入系统提示，然后调用 OpenAI Responses API 发起流式请求，
// 返回包含文本、推理、工具调用等事件的 StreamEvent channel。
func (p *ResponsesProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		if config == nil {
			config = &provider.ChatConfig{}
		}

		params := p.buildResponseNewParams(config.Model, p.buildInput(systemPrompt, messages), config)

		provider.DebugRequest(
			"openai-responses",
			"POST /responses (streaming, model="+config.Model+")",
			nil,
			params,
		)

		stream := p.Client.Responses.NewStreaming(ctx, params)
		defer stream.Close()

		var drainMu sync.Mutex
		drainTimedOut := false
		drainStarted := false

		startDrainTimer := func() {
			drainMu.Lock()
			already := drainStarted
			drainStarted = true
			drainMu.Unlock()
			if already {
				return
			}
			time.AfterFunc(streamDrainTimeout, func() {
				drainMu.Lock()
				drainTimedOut = true
				drainMu.Unlock()
				_ = stream.Close()
			})
		}

		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false
		stopSent := false

		for stream.Next() {
			if !startSent {
				startSent = true
				select {
				case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
				case <-ctx.Done():
					return
				}
			}

			event := stream.Current()
			events := p.handleStreamEvent(ctx, event, &textBlockStarted, &thinkingBlockStarted)
			for _, e := range events {
				if e.Type == provider.StreamTypeStop {
					stopSent = true
				}
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}

			if stopSent {
				startDrainTimer()
			}
		}

		drainMu.Lock()
		timedOut := drainTimedOut
		drainMu.Unlock()

		if !timedOut {
			if err := stream.Err(); err != nil {
				select {
				case eventCh <- provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  xError.NewError(ctx, nil, "OpenAI 流式对话失败", false, err),
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

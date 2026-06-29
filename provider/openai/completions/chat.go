package completions

import (
	"context"
	"sync"
	"time"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// streamDrainTimeout 是收到 finish_reason 后等待上游关闭连接的最大时长。
//
// 背景：openai-go SDK 的 Stream.Next() 在收到 data: [DONE] 后不 break，
// 而是继续 drain 直到底层 HTTP 连接关闭（io.EOF）。部分上游（如智谱 GLM）
// 在发送 [DONE] 后不主动关闭 TCP 连接，导致 Stream.Next() 永久阻塞。
// 此超时作为兜底：finish_reason 到达后若连接在此时长内仍未关闭，则强制关闭流。
//
// 5 秒足够覆盖正常情况下 usage chunk 和 [DONE] 帧的到达间隔，
// 同时避免异常场景下 goroutine 长时间挂起。
const streamDrainTimeout = 5 * time.Second

// Chat 流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Chat Completions 格式，
// 通过底层 SDK 发起流式请求，返回 StreamEvent channel。
func (p *CompletionsProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 在消息列表前插入系统提示，然后发起流式对话。
func (p *CompletionsProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		params := p.buildParams(systemPrompt, messages, config)
		params.StreamOptions = p.buildStreamOptions()

		provider.DebugRequest(
			"openai-completions",
			"POST /chat/completions (streaming, model="+config.Model+")",
			nil,
			params,
		)

		stream := p.Client.Chat.Completions.NewStreaming(ctx, params)
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

			chunk := stream.Current()
			events := p.handleChunk(chunk, &textBlockStarted, &thinkingBlockStarted, &stopSent)

			if stopSent {
				startDrainTimer()
			}

			for _, e := range events {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
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
					Err:  xError.NewError(ctx, nil, formatUpstreamError(err), false, err),
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

package completions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

var defaultStreamDrainTimeout = 5 * time.Second

type streamErrKind int

const (
	errKindNone       streamErrKind = iota
	errKindJSONParse
	errKindFatal
)

func classifyStreamError(err error) streamErrKind {
	if err == nil {
		return errKindNone
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return errKindJSONParse
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errKindFatal
	}

	msg := err.Error()
	for _, kw := range []string{
		"invalid character",
		"unexpected end of JSON",
		"JSON parse error",
		"cannot unmarshal",
		"error calling MarshalJSON",
	} {
		if strings.Contains(msg, kw) {
			return errKindJSONParse
		}
	}
	return errKindFatal
}

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

		drainTimeout := p.streamDrainTimeout
		if drainTimeout <= 0 {
			drainTimeout = defaultStreamDrainTimeout
		}

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
			time.AfterFunc(drainTimeout, func() {
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
				startDrainTimer()
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
				errKind := classifyStreamError(err)

				if errKind == errKindJSONParse {
					if !stopSent {
						select {
						case eventCh <- provider.StreamEvent{
							Type:         provider.StreamTypeStop,
							FinishReason: provider.FinishReasonStop,
						}:
						case <-ctx.Done():
							return
						}
					}
				} else {
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
		}

		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

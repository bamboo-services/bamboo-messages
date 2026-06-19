package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

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

		// 发送流开始事件
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
		case <-ctx.Done():
			return
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

	textBlockStarted := false
	thinkingBlockStarted := false

	for stream.Next() {
		event := stream.Current()
		events := p.handleStreamEvent(ctx, event, &textBlockStarted, &thinkingBlockStarted)
		for _, e := range events {
			select {
			case eventCh <- e:
			case <-ctx.Done():
				return
			}
		}
	}

		if err := stream.Err(); err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  xError.NewError(ctx, nil, "OpenAI 流式对话失败", false, err),
			}:
			case <-ctx.Done():
			}
			return
		}

		// 发送完成事件
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

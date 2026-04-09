package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Chat 流式对话
func (p *ResponsesProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话
func (p *ResponsesProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 发送流开始事件
		eventCh <- provider.StreamEvent{
			Type: provider.StreamTypeStart,
		}

		params := responses.ResponseNewParams{
			Model: config.Model,
			Input: p.buildInput(systemPrompt, messages),
		}

		if config.MaxTokens > 0 {
			params.MaxOutputTokens = openai.Int(config.MaxTokens)
		}

		stream := p.Client.Responses.NewStreaming(ctx, params)

		for stream.Next() {
			event := stream.Current()
			events := p.handleStreamEvent(event)
			for _, e := range events {
				eventCh <- e
			}
		}

		if err := stream.Err(); err != nil {
			eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  xError.NewError(ctx, xError.OperationFailed, "OpenAI 流式对话失败", false, err),
			}
			return
		}

		// 发送完成事件
		eventCh <- provider.StreamEvent{
			Type: provider.StreamTypeDone,
		}
	}()

	return eventCh
}

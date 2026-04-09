package completions

import (
	"context"

	"github.com/openai/openai-go/v3"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Chat 流式对话
func (p *CompletionsProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话
func (p *CompletionsProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 发送流开始事件
		eventCh <- provider.StreamEvent{
			Type: provider.StreamTypeStart,
		}

		params := openai.ChatCompletionNewParams{
			Model:    config.Model,
			Messages: p.buildMessages(systemPrompt, messages),
		}

		if config.MaxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(config.MaxTokens)
		}

		if config.Temperature != nil {
			params.Temperature = openai.Float(*config.Temperature)
		}

		if config.TopP != nil {
			params.TopP = openai.Float(*config.TopP)
		}

		// 启用 usage 流式返回
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}

		stream := p.Client.Chat.Completions.NewStreaming(ctx, params)

		for stream.Next() {
			chunk := stream.Current()
			events := p.handleChunk(chunk)
			for _, e := range events {
				eventCh <- e
			}
		}

		if err := stream.Err(); err != nil {
			eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  xError.NewError(ctx, xError.OperationFailed, "OpenAI Completions 流式对话失败", false, err),
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

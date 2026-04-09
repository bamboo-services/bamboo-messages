package anthropic

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Chat 流式对话
func (p *Provider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话
func (p *Provider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 发送流开始事件
		eventCh <- provider.StreamEvent{
			Type: provider.StreamTypeStart,
		}

		params := anthropic.BetaMessageNewParams{
			MaxTokens: config.MaxTokens,
			Messages:  p.buildMessages(messages),
			Model:     config.Model,
		}

		// 设置系统提示
		if systemPrompt != "" {
			params.System = []anthropic.BetaTextBlockParam{
				{Text: systemPrompt},
			}
		}

		// 设置可选参数（检查 nil 避免空指针解引用）
		if config.Temperature != nil {
			params.Temperature = anthropic.Float(*config.Temperature)
		}
		if config.TopP != nil {
			params.TopP = anthropic.Float(*config.TopP)
		}

		stream := p.Client.Beta.Messages.NewStreaming(ctx, params)

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
				Err:  xError.NewError(ctx, xError.OperationFailed, "Anthropic 流式对话失败", false, err),
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

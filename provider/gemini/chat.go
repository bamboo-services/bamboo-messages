package gemini

import (
	"context"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Chat 流式对话。
//
// 无系统提示的流式对话，内部调用 ChatWithSystem 并传入空 systemPrompt。
// 返回 StreamEvent channel，按需消费流式输出。
func (p *Provider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 将统一 provider.Message 转换为 Gemini 协议格式，
// 通过底层 SDK 发起流式请求（GenerateContentStream），返回 StreamEvent channel。
// 支持系统提示、温度、TopP、MaxTokens、Stop 序列、工具调用、Thinking 配置、ToolChoice 等参数。
func (p *Provider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
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

		contents := p.buildMessages(messages)
		gc := p.buildContentConfig(systemPrompt, config)

		provider.DebugRequest(
			"gemini",
			"GenerateContentStream (model="+config.Model+")",
			nil,
			map[string]any{
				"contents": contents,
				"config":   gc,
			},
		)

		textBlockStarted := false
		thinkingBlockStarted := false

		for resp, err := range p.Client.Models.GenerateContentStream(ctx, config.Model, contents, gc) {
			if err != nil {
				select {
				case eventCh <- provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  xError.NewError(ctx, nil, "Gemini 流式对话失败", false, err),
				}:
				case <-ctx.Done():
				}
				return
			}

			events := p.handleStreamEvent(resp, &textBlockStarted, &thinkingBlockStarted)
			for _, e := range events {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		// 发送完成事件
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

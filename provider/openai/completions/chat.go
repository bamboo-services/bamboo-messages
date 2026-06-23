package completions

import (
	"context"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

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

		// 构建请求参数（共享逻辑提取至 buildParams）
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

		// 追踪文本块是否已开始，用于合成 OpenAI Completions 缺失的 content_block_start 事件
		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false

		for stream.Next() {
			// 延迟发送 StreamTypeStart：首次成功读取数据后确认连接正常才发送
			if !startSent {
				startSent = true
				select {
				case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
				case <-ctx.Done():
					return
				}
			}

			chunk := stream.Current()
			events := p.handleChunk(chunk, &textBlockStarted, &thinkingBlockStarted)
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
				Err:  xError.NewError(ctx, nil, formatUpstreamError(err), false, err),
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

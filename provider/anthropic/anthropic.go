package anthropic

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Provider Anthropic (Claude) AI 服务提供商实现
type Provider provider.BaseProvider[anthropic.Client]

// NewProvider 创建 Anthropic Provider 实例
func NewProvider(apiKey string) *Provider {
	client := anthropic.NewClient(
		option.WithHeader("User-Agent", "vesper-ling/agent 0.0.1"),
		option.WithAPIKey(apiKey),
	)

	return &Provider{
		Client: client,
	}
}

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

		stream := p.Client.Beta.Messages.NewStreaming(
			ctx, anthropic.BetaMessageNewParams{
				MaxTokens:   config.MaxTokens,
				Messages:    p.buildMessages(messages),
				Model:       config.Model,
				Temperature: anthropic.Float(*config.Temperature),
				TopP:        anthropic.Float(*config.TopP),
			},
		)

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

// GetProviderType 获取提供商类型
func (p *Provider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

// GetAvailableModels 获取可用模型列表
func (p *Provider) GetAvailableModels() []string {
	return []string{
		"claude-sonnet-4-20250514",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}
}

// ==============================
// 内部方法
// ==============================

// buildMessages 将内部消息格式转换为 Anthropic SDK 消息格式
func (p *Provider) buildMessages(messages []provider.Message) []anthropic.BetaMessageParam {
	result := make([]anthropic.BetaMessageParam, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			result = append(result, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(msg.Content)))
		case provider.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(msg.ToolCalls)+1)
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewBetaTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					blocks = append(blocks, anthropic.NewBetaToolUseBlock(tc.ID, tc.Function.Arguments, tc.Function.Name))
				}
				result = append(result, anthropic.BetaMessageParam{
					Role:    anthropic.BetaMessageParamRoleAssistant,
					Content: blocks,
				})
			} else {
				result = append(result, anthropic.BetaMessageParam{
					Role:    anthropic.BetaMessageParamRoleAssistant,
					Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(msg.Content)},
				})
			}
		case provider.RoleTool:
			result = append(result, anthropic.NewBetaUserMessage(
				anthropic.NewBetaToolResultBlock(msg.ToolCallID, msg.Content, false),
			))
		}
	}
	return result
}

// handleStreamEvent 根据事件类型分发到对应的处理方法
func (p *Provider) handleStreamEvent(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	switch event.Type {
	case "message_start":
		return p.contentMessageStart(event)
	case "content_block_start":
		return p.contentBlockStart(event)
	case "content_block_delta":
		return p.contentBlockDelta(event)
	case "content_block_stop":
		return p.contentBlockStop(event)
	case "message_delta":
		return p.contentMessageDelta(event)
	case "message_stop":
		return p.contentMessageStop(event)
	default:
		return nil
	}
}

// contentMessageStart 处理消息开始事件
func (p *Provider) contentMessageStart(_ anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	// 消息开始，无需特殊处理，已在 ChatWithSystem 中发送 StreamTypeStart
	return nil
}

// contentBlockStart 处理内容块开始事件
func (p *Provider) contentBlockStart(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	block := event.AsContentBlockStart()
	switch block.ContentBlock.Type {
	case "thinking":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(block.ContentBlock.Thinking),
		}}
	case "tool_use":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(block.ContentBlock.ID, block.ContentBlock.Name),
		}}
	default:
		return nil
	}
}

// contentBlockDelta 处理内容块增量事件
func (p *Provider) contentBlockDelta(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	delta := event.AsContentBlockDelta()
	switch delta.Delta.Type {
	case "text_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta(delta.Delta.Text),
		}}
	case "thinking_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(delta.Delta.Thinking),
		}}
	case "input_json_delta":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDeltaData(delta.Delta.PartialJSON),
		}}
	default:
		return nil
	}
}

// contentBlockStop 处理内容块结束事件
func (p *Provider) contentBlockStop(_ anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	// 内容块结束，无需特殊处理
	return nil
}

// contentMessageDelta 处理消息增量事件（包含 usage）
func (p *Provider) contentMessageDelta(event anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	msgDelta := event.AsMessageDelta()
	if msgDelta.Usage.InputTokens > 0 || msgDelta.Usage.OutputTokens > 0 {
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewUsageDelta(msgDelta.Usage.InputTokens, msgDelta.Usage.OutputTokens),
		}}
	}
	return nil
}

// contentMessageStop 处理消息结束事件
func (p *Provider) contentMessageStop(_ anthropic.BetaRawMessageStreamEventUnion) []provider.StreamEvent {
	return []provider.StreamEvent{{
		Type: provider.StreamTypeStop,
	}}
}

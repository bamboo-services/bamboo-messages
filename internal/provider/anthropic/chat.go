package anthropic

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

const paramTopK = "top_k"

// Chat 流式对话。
//
// 无系统提示的流式对话，内部调用 ChatWithSystem 并传入空 systemPrompt。
// 返回 StreamEvent channel，按需消费流式输出。
func (p *Provider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 将统一 provider.Message 转换为 Anthropic 协议格式，
// 通过底层 SDK 发起流式请求，返回 StreamEvent channel。
// 支持系统提示、温度、TopP、Stop 序列、工具调用、Thinking 配置、TopK、ToolChoice 等参数。
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

		if len(config.Stop) > 0 {
			params.StopSequences = config.Stop
		}
		if tools := buildTools(config.Tools); tools != nil {
			params.Tools = tools
		}

		if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
			params.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
			}
		}

	if topK, ok := provider.GetExtraFloat64(config.ProviderExtra, paramTopK); ok {
		params.TopK = param.NewOpt(int64(topK))
	}

	// ToolChoice 映射: auto→OfAuto, none→OfNone, required/forced→OfAny
	if config.ToolChoice != "" {
		switch config.ToolChoice {
		case "auto":
			params.ToolChoice.OfAuto = &anthropic.BetaToolChoiceAutoParam{}
		case "any", "required", "forced":
			params.ToolChoice.OfAny = &anthropic.BetaToolChoiceAnyParam{}
		case "none":
			noneParam := anthropic.NewBetaToolChoiceNoneParam()
			params.ToolChoice.OfNone = &noneParam
		}
	}

	if config.UserID != "" || len(config.Metadata) > 0 {
		params.Metadata = anthropic.BetaMetadataParam{}
		if config.UserID != "" {
			params.Metadata.UserID = param.NewOpt(config.UserID)
		}
		if len(config.Metadata) > 0 {
			extra := make(map[string]any, len(config.Metadata))
			for k, v := range config.Metadata {
				extra[k] = v
			}
			params.Metadata.SetExtraFields(extra)
		}
	}

		stream := p.Client.Beta.Messages.NewStreaming(ctx, params)
		defer stream.Close()

		for stream.Next() {
			event := stream.Current()
			events := p.handleStreamEvent(event)
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
				Err:  xError.NewError(ctx, xError.OperationFailed, "Anthropic 流式对话失败", false, err),
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

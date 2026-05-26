package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
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

		params := responses.ResponseNewParams{
			Model: config.Model,
			Input: p.buildInput(systemPrompt, messages),
		}

		if config.MaxTokens > 0 {
			params.MaxOutputTokens = openai.Int(config.MaxTokens)
		}

		if config.Temperature != nil {
			params.Temperature = openai.Float(*config.Temperature)
		}

		if config.TopP != nil {
			params.TopP = openai.Float(*config.TopP)
		}

		if tools := buildTools(config.Tools); tools != nil {
			params.Tools = tools
		}

		// 透传 Reasoning 参数 (Effort + Summary)
		if config.ThinkingConfig != nil && (config.ThinkingConfig.ReasoningEffort != "" || config.ThinkingConfig.Summary != "") {
			reasoning := shared.ReasoningParam{}
			if config.ThinkingConfig.ReasoningEffort != "" {
				reasoning.Effort = shared.ReasoningEffort(config.ThinkingConfig.ReasoningEffort)
			}
			if config.ThinkingConfig.Summary != "" {
				reasoning.Summary = shared.ReasoningSummary(config.ThinkingConfig.Summary)
			}
			params.Reasoning = reasoning
		}

		// 透传 ToolChoice 参数
		if tc, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyToolChoice); ok {
			if tcStr, ok := tc.(string); ok {
				// 简单字符串模式,如 "auto", "none", "required"
				params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
					OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptions(tcStr)),
				}
			}
		}

		// 透传 ResponseFormat 参数 (通过 Text.Format)
		if rf, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyResponseFormat); ok {
			if rfStr, ok := rf.(string); ok {
				if rfStr == "text" {
					params.Text = responses.ResponseTextConfigParam{
						Format: responses.ResponseFormatTextConfigUnionParam{
							OfText: openai.Ptr(shared.NewResponseFormatTextParam()),
						},
					}
				} else if rfStr == "json_object" {
					params.Text = responses.ResponseTextConfigParam{
						Format: responses.ResponseFormatTextConfigUnionParam{
							OfJSONObject: openai.Ptr(shared.NewResponseFormatJSONObjectParam()),
						},
					}
				}
			}
		}

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
				Err:  xError.NewError(ctx, xError.OperationFailed, "OpenAI 流式对话失败", false, err),
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

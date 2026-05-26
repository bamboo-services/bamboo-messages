package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// Complete 非流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Responses 格式，
// 通过底层 SDK 发起同步请求，返回完整响应结果和错误信息。
func (p *ResponsesProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 在消息前插入系统提示，然后调用 OpenAI Responses API 发起同步请求，
// 返回包含文本内容、工具调用、Token 用量等信息的完整结果。
func (p *ResponsesProvider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	if config == nil {
		config = &provider.ChatConfig{}
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
			// 支持简单字符串格式,如 "text", "json_object"
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

	response, err := p.Client.Responses.New(ctx, params)
	if err != nil {
		return nil, xError.NewError(ctx, xError.OperationFailed, "OpenAI Responses 非流式对话失败", false, err)
	}

	// 解析响应结果
	result := &provider.CompletionResult{}

	for _, item := range response.Output {
		switch item.Type {
		case "message":
			msg := item.AsMessage()
			for _, content := range msg.Content {
				if content.Type == "output_text" {
					result.Content += content.Text
				}
			}
		case "function_call":
			fc := item.AsFunctionCall()
			result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
				ID:   fc.CallID,
				Type: "function",
				Function: provider.FunctionCall{
					Name:      fc.Name,
					Arguments: fc.Arguments,
				},
			})
		}
	}

	// 设置完成原因
	if response.Status == "incomplete" {
		result.FinishReason = provider.FinishReasonLength
	} else if len(result.ToolCalls) > 0 {
		result.FinishReason = provider.FinishReasonToolCalls
	} else {
		result.FinishReason = provider.FinishReasonStop
	}

	// 设置用量统计
	result.Usage = provider.UsageData{
		InputTokens:  response.Usage.InputTokens,
		OutputTokens: response.Usage.OutputTokens,
	}

	return result, nil
}

package completions

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// Complete 非流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Chat Completions 格式，
// 通过底层 SDK 发起同步请求，返回完整的 CompletionResult。
func (p *CompletionsProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 在消息列表前插入系统提示，然后发起同步对话。
func (p *CompletionsProvider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	if config == nil {
		config = &provider.ChatConfig{}
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

	if len(config.Stop) > 0 {
		params.Stop = buildStop(config.Stop)
	}

	if tools := buildTools(config.Tools); tools != nil {
		params.Tools = tools
	}

	if config.ThinkingConfig != nil && config.ThinkingConfig.ReasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(config.ThinkingConfig.ReasoningEffort)
	}

	if fp, ok := provider.GetExtraFloat64(config.ProviderExtra, provider.ProviderExtraKeyFrequencyPenalty); ok {
		params.FrequencyPenalty = openai.Float(fp)
	}

	if pp, ok := provider.GetExtraFloat64(config.ProviderExtra, provider.ProviderExtraKeyPresencePenalty); ok {
		params.PresencePenalty = openai.Float(pp)
	}

	if seed, ok := provider.GetExtraInt64(config.ProviderExtra, provider.ProviderExtraKeySeed); ok {
		params.Seed = openai.Int(seed)
	}

	if tc, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyToolChoice); ok {
		if toolChoice, ok := tc.(openai.ChatCompletionToolChoiceOptionUnionParam); ok {
			params.ToolChoice = toolChoice
		}
	}

	if rf, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyResponseFormat); ok {
		if responseFormat, ok := rf.(openai.ChatCompletionNewParamsResponseFormatUnion); ok {
			params.ResponseFormat = responseFormat
		}
	}

	// 调用非流式 SDK 方法
	response, err := p.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, xError.NewError(ctx, xError.OperationFailed, "OpenAI Completions 非流式对话失败", false, err)
	}

	// 检查响应
	if len(response.Choices) == 0 {
		return nil, xError.NewError(ctx, xError.OperationFailed, "OpenAI Completions 返回空响应", false, nil)
	}

	choice := response.Choices[0]

	// 解析响应内容
	result := &provider.CompletionResult{
		Content:      choice.Message.Content,
		FinishReason: mapFinishReason(choice.FinishReason),
		Usage: provider.UsageData{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CompletionTokens,
		},
	}

	// 解析工具调用
	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: provider.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return result, nil
}

// mapFinishReason 将 OpenAI 停止原因映射为统一的 FinishReason。
//
// 将 OpenAI 的 stop/length/tool_calls 映射为 provider.FinishReason 的对应值。
func mapFinishReason(reason string) provider.FinishReason {
	switch reason {
	case "stop":
		return provider.FinishReasonStop
	case "length":
		return provider.FinishReasonLength
	case "tool_calls":
		return provider.FinishReasonToolCalls
	default:
		return provider.FinishReasonStop
	}
}

package completions

import (
	"context"

	"github.com/openai/openai-go/v3"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Complete 非流式对话
func (p *CompletionsProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话
func (p *CompletionsProvider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
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

// mapFinishReason 将 OpenAI 停止原因映射为统一的 FinishReason
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

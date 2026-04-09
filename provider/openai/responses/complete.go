package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Complete 非流式对话
func (p *ResponsesProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话
func (p *ResponsesProvider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	params := responses.ResponseNewParams{
		Model: config.Model,
		Input: p.buildInput(systemPrompt, messages),
	}

	if config.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(config.MaxTokens)
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
	if len(result.ToolCalls) > 0 {
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

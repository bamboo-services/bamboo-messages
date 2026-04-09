package anthropic

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Complete 非流式对话
func (p *Provider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话
func (p *Provider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
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

	// 调用非流式 SDK 方法
	response, err := p.Client.Beta.Messages.New(ctx, params)
	if err != nil {
		return nil, xError.NewError(ctx, xError.OperationFailed, "Anthropic 非流式对话失败", false, err)
	}

	// 解析响应内容
	result := &provider.CompletionResult{
		FinishReason: mapFinishReason(response.StopReason),
		Usage: provider.UsageData{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
		},
	}

	// 遍历响应内容块
	for _, block := range response.Content {
		switch block.Type {
		case "text":
			result.Content += block.AsText().Text
		case "tool_use":
			toolUse := block.AsToolUse()
			inputBytes, _ := json.Marshal(toolUse.Input)
			result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
				ID:   toolUse.ID,
				Type: "function",
				Function: provider.FunctionCall{
					Name:      toolUse.Name,
					Arguments: string(inputBytes),
				},
			})
		}
	}

	return result, nil
}

// mapFinishReason 将 Anthropic 停止原因映射为统一的 FinishReason
func mapFinishReason(reason anthropic.BetaStopReason) provider.FinishReason {
	switch reason {
	case anthropic.BetaStopReasonEndTurn:
		return provider.FinishReasonStop
	case anthropic.BetaStopReasonMaxTokens:
		return provider.FinishReasonLength
	case anthropic.BetaStopReasonToolUse:
		return provider.FinishReasonToolCalls
	default:
		return provider.FinishReasonStop
	}
}

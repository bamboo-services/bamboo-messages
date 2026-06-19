package completions

import (
	"context"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
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
	// 构建请求参数（共享逻辑提取至 buildParams）
	params := p.buildParams(systemPrompt, messages, config)

	// 调用非流式 SDK 方法
	response, err := p.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, xError.NewError(ctx, nil, "OpenAI Completions 非流式对话失败", false, err)
	}

	// 检查响应
	if len(response.Choices) == 0 {
		return nil, xError.NewError(ctx, nil, "OpenAI Completions 返回空响应", false, nil)
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

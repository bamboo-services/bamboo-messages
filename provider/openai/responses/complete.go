package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
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

	params := p.buildResponseNewParams(config.Model, p.buildInput(systemPrompt, messages), config)

	provider.DebugRequest(
		"openai-responses",
		"POST /responses (non-stream, model="+config.Model+")",
		nil,
		params,
	)

	response, err := p.Client.Responses.New(ctx, params)
	if err != nil {
		return nil, xError.NewError(ctx, nil, "OpenAI Responses 非流式对话失败", false, err)
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
		case "reasoning":
			rc := item.AsReasoning()
			for _, sum := range rc.Summary {
				if sum.Text != "" {
					result.Thinking += sum.Text
				}
			}
			if result.Thinking == "" {
				for _, content := range rc.Content {
					if content.Text != "" {
						result.Thinking += content.Text
					}
				}
			}
			// encrypted_content 是 OpenAI 服务端加密的不透明 token，
			// Responses → Responses 直连时原样传回可保持推理上下文连续性。
			if rc.EncryptedContent != "" {
				result.ThinkingSignature = rc.EncryptedContent
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
		if len(result.ToolCalls) > 0 {
			result.FinishReason = provider.FinishReasonToolCalls
		} else {
			result.FinishReason = provider.FinishReasonLength
		}
	} else if len(result.ToolCalls) > 0 {
		result.FinishReason = provider.FinishReasonToolCalls
	} else {
		result.FinishReason = provider.FinishReasonStop
	}

	// 设置用量统计
	result.Usage = provider.UsageData{
		InputTokens:          response.Usage.InputTokens,
		OutputTokens:         response.Usage.OutputTokens,
		CacheReadInputTokens: response.Usage.InputTokensDetails.CachedTokens,
	}

	return result, nil
}

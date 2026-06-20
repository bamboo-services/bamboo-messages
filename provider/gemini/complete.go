package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Complete 非流式对话。
//
// 无系统提示的非流式对话，内部调用 CompleteWithSystem 并传入空 systemPrompt。
// 同步返回完整响应和可能的错误。
func (p *Provider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 将统一 provider.Message 转换为 Gemini 协议格式，
// 通过底层 SDK 发起同步请求（GenerateContent），返回 CompletionResult。
// 支持系统提示、温度、TopP、MaxTokens、Stop 序列、工具调用、Thinking 配置、ToolChoice 等参数。
func (p *Provider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	contents := p.buildMessages(messages)
	gc := p.buildContentConfig(systemPrompt, config)

	provider.DebugRequest(
		"gemini",
		"GenerateContent (model="+config.Model+")",
		nil,
		map[string]any{
			"contents": contents,
			"config":   gc,
		},
	)

	resp, err := p.Client.Models.GenerateContent(ctx, config.Model, contents, gc)
	if err != nil {
		return nil, xError.NewError(ctx, nil, "Gemini 非流式对话失败", false, err)
	}

	result := &provider.CompletionResult{
		FinishReason: provider.FinishReasonStop,
	}

	// 提取 Token 用量
	if resp.UsageMetadata != nil {
		result.Usage = provider.UsageData{
			InputTokens:  int64(resp.UsageMetadata.PromptTokenCount),
			OutputTokens: int64(resp.UsageMetadata.CandidatesTokenCount),
		}
	}

	// 遍历响应内容
	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		result.FinishReason = mapFinishReason(candidate.FinishReason)

		if candidate.Content != nil {
			for i, part := range candidate.Content.Parts {
				// 文本内容（忽略 Thought==true 的推理内容）
				if !part.Thought && part.Text != "" {
					result.Content += part.Text
				}
				// 工具调用
				if part.FunctionCall != nil {
					id := part.FunctionCall.ID
					if id == "" {
						id = fmt.Sprintf("gemini_call_%s_%d", part.FunctionCall.Name, i)
					}
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
						ID:   id,
						Type: "function",
						Function: provider.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsBytes),
						},
					})
					// 如果有工具调用且 FinishReason 未明确指定，设为 ToolCalls
					if result.FinishReason == provider.FinishReasonStop {
						result.FinishReason = provider.FinishReasonToolCalls
					}
				}
			}
		}
	}

	return result, nil
}
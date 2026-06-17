package anthropic

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
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
// 将统一 provider.Message 转换为 Anthropic 协议格式，
// 通过底层 SDK 发起同步请求，返回 CompletionResult。
// 支持系统提示、温度、TopP、Stop 序列、工具调用、Thinking 配置、TopK、ToolChoice 等参数。
func (p *Provider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	if config == nil {
		config = &provider.ChatConfig{}
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

	response, err := p.Client.Beta.Messages.New(ctx, params)
	if err != nil {
		return nil, xError.NewError(ctx, nil, "Anthropic 非流式对话失败", false, err)
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

// mapFinishReason 将 Anthropic 停止原因映射为统一的 FinishReason。
//
// Anthropic: end_turn → Stop, max_tokens → Length, tool_use → ToolCalls，其他默认为 Stop。
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

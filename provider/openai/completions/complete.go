package completions

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
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

	if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(config.ThinkingConfig.Effort)
	}

	if fp, ok := provider.GetExtraFloat64(config.ProviderExtra, "frequency_penalty"); ok {
		params.FrequencyPenalty = openai.Float(fp)
	}

	if pp, ok := provider.GetExtraFloat64(config.ProviderExtra, "presence_penalty"); ok {
		params.PresencePenalty = openai.Float(pp)
	}

	if seed, ok := provider.GetExtraInt64(config.ProviderExtra, "seed"); ok {
		params.Seed = openai.Int(seed)
	}

	// 用户标识
	if config.UserID != "" {
		params.User = openai.String(config.UserID)
	}

	// 预测内容（用于加速已知内容的生成）
	if pred, ok := provider.GetExtraAny(config.ProviderExtra, "prediction"); ok {
		if prediction, ok := pred.(openai.ChatCompletionPredictionContentParam); ok {
			params.Prediction = prediction
		}
	}

	// 是否启用并行工具调用
	params.ParallelToolCalls = openai.Bool(config.ParallelToolCalls)

	// 附加元数据
	if len(config.Metadata) > 0 {
		params.Metadata = shared.Metadata(config.Metadata)
	}

	// 工具选择策略
	if config.ToolChoice != "" {
		tc := config.ToolChoice
		if tc == "forced" {
			tc = "required" // map forced→required for OpenAI
		}
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(tc)}
	}

	// 响应格式
	if config.ResponseFormat != "" {
		if config.ResponseFormat == "json_object" {
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: openai.Ptr(shared.NewResponseFormatJSONObjectParam()),
			}
		} else if config.ResponseFormat == "text" {
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfText: openai.Ptr(shared.NewResponseFormatTextParam()),
			}
		}
	}

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

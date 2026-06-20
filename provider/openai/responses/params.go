package responses

import (
	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// buildResponseNewParams 从 ChatConfig 构建 OpenAI Responses API 请求参数。
//
// 将统一的 ChatConfig 中的模型配置、温度、采样参数、工具定义、
// 思考/推理配置以及 ProviderExtra 透传参数映射到 OpenAI Responses SDK 的
// ResponseNewParams 结构体。
//
// 参数:
//   - model - 模型名称（直接设置到 params.Model）
//   - input - 已构建的输入消息联合体
//   - config - 统一的聊天请求配置
//
// 此方法由 ChatWithSystem 和 CompleteWithSystem 共享调用，
// 确保流式和非流式路径使用一致的参数构建逻辑。
func (p *ResponsesProvider) buildResponseNewParams(model string, input responses.ResponseNewParamsInputUnion, config *provider.ChatConfig) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: model,
		Input: input,
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

	// Stop 参数 — Responses SDK 无原生 Stop 字段，通过 ExtraFields 传递
	if len(config.Stop) > 0 {
		params.SetExtraFields(map[string]any{"stop": config.Stop})
	}

	// UserID — 统一字段，标识终端用户，用于缓存优化和安全审计
	if config.UserID != "" {
		params.User = openai.Opt(config.UserID)
	}

	// PromptCacheKey — OpenAI prompt cache 路由粘性键
	if config.PromptCacheKey != "" {
		params.PromptCacheKey = openai.Opt(config.PromptCacheKey)
	} else if key, ok := provider.GetExtraString(config.ProviderExtra, "prompt_cache_key"); ok && key != "" {
		params.PromptCacheKey = openai.Opt(key)
	}

	// ParallelToolCalls — 是否允许并行工具调用，通过 ExtraFields 传递
	if config.ParallelToolCalls {
		params.SetExtraFields(map[string]any{"parallel_tool_calls": true})
	}

	// ProviderExtra: store — 是否持久化存储响应
	if store, ok := provider.GetExtraBool(config.ProviderExtra, "store"); ok {
		params.Store = openai.Opt(store)
	}

	// ProviderExtra: truncation — 上下文截断策略 ("auto" / "disabled")
	if truncation, ok := provider.GetExtraString(config.ProviderExtra, "truncation"); ok {
		params.Truncation = responses.ResponseNewParamsTruncation(truncation)
	}

	// ProviderExtra: previous_response_id — 关联上一轮响应实现多轮对话
	if prevID, ok := provider.GetExtraString(config.ProviderExtra, "previous_response_id"); ok {
		params.PreviousResponseID = openai.Opt(prevID)
	}

	// Metadata — 附加键值对元数据
	if len(config.Metadata) > 0 {
		params.Metadata = shared.Metadata(config.Metadata)
	}

	// ProviderExtra: modalities — 输出模态（ResponseNewParams 无原生字段，通过 ExtraFields 传递）
	if modalities, ok := provider.GetExtraAny(config.ProviderExtra, "modalities"); ok {
		params.SetExtraFields(map[string]any{"modalities": modalities})
	}

	if tools := buildTools(config.Tools); tools != nil {
		params.Tools = tools
	}

	// ThinkingConfig.Effort → Reasoning 参数 (Effort + Summary 自动推导)
	if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
		reasoning := shared.ReasoningParam{
			Effort: shared.ReasoningEffort(config.ThinkingConfig.Effort),
		}
		// 根据 Effort 自动映射 Summary
		switch config.ThinkingConfig.Effort {
		case "none":
			// 无需 summary
		case "low":
			reasoning.Summary = "concise"
		case "medium":
			reasoning.Summary = "auto"
		case "high":
			reasoning.Summary = "detailed"
		}
		params.Reasoning = reasoning
	}

	// ToolChoice — 统一字段，工具选择策略
	if config.ToolChoice != "" {
		tc := config.ToolChoice
		if tc == "forced" {
			tc = "required"
		}
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptions(tc)),
		}
	}

	// ResponseFormat — 统一字段，响应格式 (text / json_object)
	if config.ResponseFormat != "" {
		if config.ResponseFormat == "text" {
			params.Text = responses.ResponseTextConfigParam{
				Format: responses.ResponseFormatTextConfigUnionParam{
					OfText: openai.Ptr(shared.NewResponseFormatTextParam()),
				},
			}
		} else if config.ResponseFormat == "json_object" {
			params.Text = responses.ResponseTextConfigParam{
				Format: responses.ResponseFormatTextConfigUnionParam{
					OfJSONObject: openai.Ptr(shared.NewResponseFormatJSONObjectParam()),
				},
			}
		}
	}

	return params
}

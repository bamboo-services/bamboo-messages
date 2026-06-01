package responses

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
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

	// 透传 Stop 参数 (Responses SDK 无原生 Stop 字段，通过 ProviderExtra 传递)
	if len(config.Stop) > 0 {
		if config.ProviderExtra == nil {
			config.ProviderExtra = make(map[string]any)
		}
		config.ProviderExtra["stop"] = config.Stop
	}

	// 透传 User 参数 — 标识终端用户，用于缓存优化和安全审计
	if user, ok := provider.GetExtraString(config.ProviderExtra, provider.ProviderExtraKeyUser); ok {
		params.User = openai.Opt(user)
	}

	// 透传 Store 参数 — 是否持久化存储响应
	if store, ok := provider.GetExtraBool(config.ProviderExtra, provider.ProviderExtraKeyStore); ok {
		params.Store = openai.Opt(store)
	}

	// 透传 Truncation 参数 — 上下文截断策略 ("auto" / "disabled")
	if truncation, ok := provider.GetExtraString(config.ProviderExtra, provider.ProviderExtraKeyTruncation); ok {
		params.Truncation = responses.ResponseNewParamsTruncation(truncation)
	}

	// 透传 PreviousResponseID 参数 — 关联上一轮响应实现多轮对话
	if prevID, ok := provider.GetExtraString(config.ProviderExtra, provider.ProviderExtraKeyPreviousResponseID); ok {
		params.PreviousResponseID = openai.Opt(prevID)
	}

	// 透传 Metadata 参数 — 附加键值对元数据
	if len(config.Metadata) > 0 {
		params.Metadata = shared.Metadata(config.Metadata)
	}

	// 透传 Modalities 参数 (ResponseNewParams 无原生 Modalities 字段，通过 SetExtraFields 传递)
	if modalities, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyModalities); ok {
		params.SetExtraFields(map[string]any{"modalities": modalities})
	}

	if tools := buildTools(config.Tools); tools != nil {
		params.Tools = tools
	}

	// 透传 Reasoning 参数 (Effort + Summary)
	if config.ThinkingConfig != nil && (config.ThinkingConfig.ReasoningEffort != "" || config.ThinkingConfig.Summary != "") {
		reasoning := shared.ReasoningParam{}
		if config.ThinkingConfig.ReasoningEffort != "" {
			reasoning.Effort = shared.ReasoningEffort(config.ThinkingConfig.ReasoningEffort)
		}
		if config.ThinkingConfig.Summary != "" {
			reasoning.Summary = shared.ReasoningSummary(config.ThinkingConfig.Summary)
		}
		params.Reasoning = reasoning
	}

	// 透传 ToolChoice 参数
	if tc, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyToolChoice); ok {
		if tcStr, ok := tc.(string); ok {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptions(tcStr)),
			}
		}
	}

	// 透传 ResponseFormat 参数 (通过 Text.Format)
	if rf, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyResponseFormat); ok {
		if rfStr, ok := rf.(string); ok {
			if rfStr == "text" {
				params.Text = responses.ResponseTextConfigParam{
					Format: responses.ResponseFormatTextConfigUnionParam{
						OfText: openai.Ptr(shared.NewResponseFormatTextParam()),
					},
				}
			} else if rfStr == "json_object" {
				params.Text = responses.ResponseTextConfigParam{
					Format: responses.ResponseFormatTextConfigUnionParam{
						OfJSONObject: openai.Ptr(shared.NewResponseFormatJSONObjectParam()),
					},
				}
			}
		}
	}

	return params
}

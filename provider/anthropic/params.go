package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/bamboo-services/bamboo-messages/provider"
)

const paramTopK = "top_k"

// buildParams 构建 Anthropic Messages 请求参数。
//
// 将统一的 provider.ChatConfig 转换为 anthropic.BetaMessageNewParams，
// 包含所有参数构建逻辑（Model/Messages/MaxTokens/Temperature/TopP/Stop/Tools/
// ThinkingConfig/TopK/ToolChoice/Metadata 等）。
//
// 此方法由 ChatWithSystem 和 CompleteWithSystem 共享调用，
// 确保流式和非流式路径使用一致的参数构建逻辑。
func (p *Provider) buildParams(systemPrompt string, messages []provider.Message, config *provider.ChatConfig) anthropic.BetaMessageNewParams {
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

	return params
}

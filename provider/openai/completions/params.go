package completions

import (
	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// buildParams 构建 OpenAI Chat Completions 请求参数。
//
// 将统一的 provider.ChatConfig 转换为 openai.ChatCompletionNewParams，
// 包含所有共享参数构建逻辑（Model/Messages/MaxTokens/Temperature 等），
// 并根据 p.legacyCompat 做条件分支处理 Legacy 兼容场景。
//
// Legacy 模式差异：
//   - MaxTokens → 使用旧字段名 max_tokens（而非 max_completion_tokens）
//   - ParallelToolCalls → 仅在有工具时设置（而非无条件设置）
//   - ReasoningEffort → 跳过自动映射（不设置 reasoning_effort）
//   - thinking 透传 → 从 ProviderExtra 提取 thinking 值，通过 SetExtraFields 注入
func (p *CompletionsProvider) buildParams(systemPrompt string, messages []provider.Message, config *provider.ChatConfig) openai.ChatCompletionNewParams {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	params := openai.ChatCompletionNewParams{
		Model:    config.Model,
		Messages: p.buildMessages(systemPrompt, messages),
	}

	// === MaxTokens — Legacy 使用旧字段名 max_tokens ===
	if config.MaxTokens > 0 {
		if p.legacyCompat {
			params.MaxTokens = openai.Int(config.MaxTokens)
		} else {
			params.MaxCompletionTokens = openai.Int(config.MaxTokens)
		}
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

	// === ReasoningEffort — Legacy 跳过自动映射 ===
	if !p.legacyCompat {
		if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
			params.ReasoningEffort = shared.ReasoningEffort(config.ThinkingConfig.Effort)
		}
	}

	// === thinking 透传 — Legacy only，从 ProviderExtra 提取并注入 JSON extra fields ===
	if p.legacyCompat {
		if thinking, ok := provider.GetExtraAny(config.ProviderExtra, "thinking"); ok {
			params.SetExtraFields(map[string]any{"thinking": thinking})
		}
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

	// PromptCacheKey — OpenAI prompt cache 路由粘性键
	if config.PromptCacheKey != "" {
		params.PromptCacheKey = openai.String(config.PromptCacheKey)
	} else if key, ok := provider.GetExtraString(config.ProviderExtra, "prompt_cache_key"); ok && key != "" {
		params.PromptCacheKey = openai.String(key)
	}

	// 预测内容（用于加速已知内容的生成）
	if pred, ok := provider.GetExtraAny(config.ProviderExtra, "prediction"); ok {
		if prediction, ok := pred.(openai.ChatCompletionPredictionContentParam); ok {
			params.Prediction = prediction
		}
	}

	// === ParallelToolCalls — Legacy 仅在显式启用且存在工具时设置 ===
	// 注意：ChatConfig.ParallelToolCalls 为 bool 类型，零值 false 表示未启用。
	// 智谱 GLM 等第三方 OpenAI 兼容端点不支持 parallel_tool_calls 参数，
	// 即使发送 false 也会返回 400 code:1210，因此 Legacy 模式仅在显式 true 时才设置。
	if !p.legacyCompat {
		params.ParallelToolCalls = openai.Bool(config.ParallelToolCalls)
	} else if len(config.Tools) > 0 && config.ParallelToolCalls {
		params.ParallelToolCalls = openai.Bool(config.ParallelToolCalls)
	}

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

	return params
}

// buildStreamOptions 构建流式请求的 StreamOptions。
//
// 智谱 GLM 等第三方 OpenAI 兼容端点不支持 stream_options 参数，
// 发送该参数会导致 400 code:1210 参数错误，因此 Legacy 模式下返回零值（序列化时省略）。
func (p *CompletionsProvider) buildStreamOptions() openai.ChatCompletionStreamOptionsParam {
	if p.legacyCompat {
		return openai.ChatCompletionStreamOptionsParam{}
	}
	return openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: openai.Bool(true),
	}
}

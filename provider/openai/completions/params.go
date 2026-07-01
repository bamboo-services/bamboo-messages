package completions

import (
	"context"
	"encoding/json"
	"fmt"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildParams 构建 OpenAI Chat Completions 请求参数。
//
// 将统一的 provider.ChatConfig 转换为 map[string]any 形式的请求体，
// 包含所有共享参数构建逻辑（Model/Messages/MaxTokens/Temperature 等），
// 并根据 p.legacyCompat 做条件分支处理 Legacy 兼容场景。
//
// Legacy 模式差异：
//   - MaxTokens → 使用旧字段名 max_tokens（而非 max_completion_tokens）
//   - ParallelToolCalls → 仅在有工具时设置（而非无条件设置）
//   - ReasoningEffort → 跳过自动映射（不设置 reasoning_effort）
//   - StreamOptions → 省略（部分第三方端点不支持）
//   - thinking 透传 → 从 ProviderExtra 提取 thinking 值，注入为顶层字段
func (p *CompletionsProvider) buildParams(systemPrompt string, messages []provider.Message, config *provider.ChatConfig) map[string]any {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	params := map[string]any{
		"model":    config.Model,
		"messages": p.buildMessages(systemPrompt, messages),
	}

	// === MaxTokens — Legacy 使用旧字段名 max_tokens ===
	if config.MaxTokens > 0 {
		if p.legacyCompat {
			params["max_tokens"] = config.MaxTokens
		} else {
			params["max_completion_tokens"] = config.MaxTokens
		}
	}

	if config.Temperature != nil {
		params["temperature"] = *config.Temperature
	}

	if config.TopP != nil {
		params["top_p"] = *config.TopP
	}

	if len(config.Stop) > 0 {
		params["stop"] = config.Stop
	}

	if tools := buildTools(config.Tools); len(tools) > 0 {
		params["tools"] = tools
	}

	// === ReasoningEffort ===
	// 默认模式和 Legacy 模式都映射 reasoning_effort，直接透传标准值域。
	// GLM-5.2 等第三方端点原生支持 none/minimal/low/medium/high/xhigh/max 全部值，
	// 服务端会做兼容映射（xhigh→max、low/medium→high），无需客户端降级。
	if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
		params["reasoning_effort"] = config.ThinkingConfig.Effort
	}

	// === thinking 透传 — Legacy only ===
	// GLM 等第三方端点需要 thinking: {type:"enabled"} 启用思考，reasoning_effort 控制级别。
	// 数据来源优先级：
	//   1. ProviderExtra["thinking"] — 原始 thinking JSON（跨协议场景，如 Anthropic 入口）
	//   2. ThinkingConfig.Effort — 合成 {type:"enabled"} 或 {type:"disabled"}
	if p.legacyCompat {
		if thinking, ok := provider.GetExtraAny(config.ProviderExtra, "thinking"); ok {
			params["thinking"] = normalizeLegacyThinking(thinking)
		} else if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
			if synthesized := effortToLegacyThinking(config.ThinkingConfig.Effort); synthesized != nil {
				params["thinking"] = synthesized
			}
		}
	}

	if fp, ok := provider.GetExtraFloat64(config.ProviderExtra, "frequency_penalty"); ok {
		params["frequency_penalty"] = fp
	}

	if pp, ok := provider.GetExtraFloat64(config.ProviderExtra, "presence_penalty"); ok {
		params["presence_penalty"] = pp
	}

	if seed, ok := provider.GetExtraInt64(config.ProviderExtra, "seed"); ok {
		params["seed"] = seed
	}

	// 用户标识
	if config.UserID != "" {
		params["user"] = config.UserID
	}

	// PromptCacheKey — OpenAI prompt cache 路由粘性键。
	// Legacy 模式跳过 prompt_cache_key（第三方端点可能不支持）。
	// 非 Legacy 模式：显式 PromptCacheKey 优先；其次回退到 ProviderExtra；
	// SystemCacheControl（Anthropic 语义）在 OpenAI 中无原生对应字段，
	// OpenAI 会自动处理 prompt 缓存，此处仅记录 debug 提示。
	if !p.legacyCompat {
		if config.PromptCacheKey != "" {
			params["prompt_cache_key"] = config.PromptCacheKey
		} else if key, ok := provider.GetExtraString(config.ProviderExtra, "prompt_cache_key"); ok && key != "" {
			params["prompt_cache_key"] = key
		} else if config.SystemCacheControl != nil && provider.DebugEnabled {
			xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
				fmt.Sprintf("SystemCacheControl (type=%s) 由 OpenAI 自动缓存处理", config.SystemCacheControl.Type))
		}
	}

	// 预测内容（用于加速已知内容的生成）
	// 去 SDK 化后直接透传原始值（map[string]any 或其他可序列化类型）
	if pred, ok := provider.GetExtraAny(config.ProviderExtra, "prediction"); ok {
		params["prediction"] = pred
	}

	// ParallelToolCalls：bool 零值无法区分"未设置"与"显式 false"。
	// 为兼容智谱 GLM / Kimi 等第三方端点（发送 false 可能触发 400/空响应），
	// 默认与 Legacy 模式统一：仅当 tools 非空且显式 true 时才发送。
	if len(config.Tools) > 0 && config.ParallelToolCalls {
		params["parallel_tool_calls"] = config.ParallelToolCalls
	}

	// 附加元数据
	if len(config.Metadata) > 0 {
		params["metadata"] = config.Metadata
	}

	// 工具选择策略
	if config.ToolChoice != "" {
		tc := config.ToolChoice
		if tc == "forced" {
			tc = "required" // 映射 forced→required，OpenAI 标准
		}
		params["tool_choice"] = tc
	}

	// 响应格式
	if config.ResponseFormat != "" {
		if rf := buildResponseFormat(config.ResponseFormat); rf != nil {
			params["response_format"] = rf
		}
	}

	return params
}

// effortToLegacyThinking 将统一 Effort 值合成为 GLM 等第三方端点的 thinking 参数。
//
// GLM-5.2 需要 thinking: {type:"enabled"} 启用思考，reasoning_effort 控制级别。
// 此函数仅负责合成 thinking 开关，effort 值本身通过 reasoning_effort 字段传递。
//
//	none          → {type:"disabled"}
//	其他任意非空值 → {type:"enabled"}
func effortToLegacyThinking(effort string) map[string]any {
	if effort == "none" {
		return map[string]any{"type": "disabled"}
	}
	return map[string]any{"type": "enabled"}
}

// normalizeLegacyThinking 将原始 thinking 配置归一化为 legacy 端点可识别的格式。
//
// 跨协议场景下（如 Anthropic 入口），原始 thinking JSON 是 Anthropic 语义（type:"adaptive"），
// 但 legacy 端点（GLM/Kimi 等）仅识别 type:"enabled"/"disabled"，adaptive 会被静默忽略。
// 此函数将 adaptive 映射为 enabled（保留 budget_tokens 等其他字段），其他值原样返回。
//
// 保守策略：任何类型断言或 JSON 解析失败时，均原样返回原始值。
func normalizeLegacyThinking(thinking any) any {
	switch v := thinking.(type) {
	case map[string]any:
		typeVal, _ := v["type"].(string)
		if typeVal != "adaptive" {
			return v
		}
		// 复制 map 以避免修改原始数据
		normalized := make(map[string]any, len(v))
		for k, val := range v {
			normalized[k] = val
		}
		normalized["type"] = "enabled"
		return normalized

	case json.RawMessage:
		var m map[string]any
		if err := json.Unmarshal(v, &m); err != nil {
			return thinking
		}
		return normalizeLegacyThinking(m)

	default:
		return thinking
	}
}

// buildStreamOptions 构建流式请求的 StreamOptions。
//
// 默认模式始终发送 include_usage=true。
// Legacy 模式下默认省略（部分第三方端点不支持 stream_options 参数），
// 但当 includeUsage 标志为 true 时强制发送（用于 GLM/Kimi Coding 等支持 stream_options 的端点）。
func (p *CompletionsProvider) buildStreamOptions() map[string]any {
	if p.legacyCompat && !p.includeUsage {
		return nil
	}
	return map[string]any{
		"include_usage": true,
	}
}

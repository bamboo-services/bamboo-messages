package responses

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildParams 从 ChatConfig 构建 OpenAI Responses API 请求参数。
//
// 将统一的 ChatConfig 中的模型配置、温度、采样参数、工具定义、
// 思考/推理配置以及 ProviderExtra 透传参数映射到 OpenAI Responses API 的
// 请求体（map[string]any 格式），后续由 chat.go / complete.go 序列化为 JSON
// 并通过 HTTP 发送。
//
// 参数:
//   - model - 模型名称
//   - systemPrompt - 系统提示词（映射到 instructions 字段）
//   - messages - 统一消息数组
//   - config - 统一的聊天请求配置
//   - stream - 是否使用流式响应
//
// 此方法由 ChatWithSystem 和 CompleteWithSystem 共享调用，
// 确保流式和非流式路径使用一致的参数构建逻辑。
func (p *ResponsesProvider) buildParams(model string, systemPrompt string, messages []provider.Message, config *provider.ChatConfig, stream bool) map[string]any {
	params := map[string]any{
		"model":  model,
		"input":  p.buildInput(messages),
		"stream": stream,
	}

	// Instructions — 系统提示词（优先使用显式 systemPrompt，回退到 ProviderExtra）
	if systemPrompt != "" {
		params["instructions"] = systemPrompt
	} else if instructions, ok := provider.GetExtraString(config.ProviderExtra, "instructions"); ok && instructions != "" {
		params["instructions"] = instructions
	}

	// MaxOutputTokens — 最大输出 token 数
	if config.MaxTokens > 0 {
		params["max_output_tokens"] = config.MaxTokens
	}

	// Temperature — 采样温度
	if config.Temperature != nil {
		params["temperature"] = *config.Temperature
	}

	// TopP — 核采样参数
	if config.TopP != nil {
		params["top_p"] = *config.TopP
	}

	// Stop — 停止词（Responses API 无原生字段，直接透传）
	if len(config.Stop) > 0 {
		params["stop"] = config.Stop
	}

	// User — 终端用户标识，用于缓存优化和安全审计
	if config.UserID != "" {
		params["user"] = config.UserID
	}

	// PromptCacheKey — OpenAI prompt cache 路由粘性键
	if config.PromptCacheKey != "" {
		params["prompt_cache_key"] = config.PromptCacheKey
	} else if key, ok := provider.GetExtraString(config.ProviderExtra, "prompt_cache_key"); ok && key != "" {
		params["prompt_cache_key"] = key
	}

	// ParallelToolCalls — 是否允许并行工具调用
	if config.ParallelToolCalls {
		params["parallel_tool_calls"] = true
	}

	// ProviderExtra: store — 是否持久化存储响应
	if store, ok := provider.GetExtraBool(config.ProviderExtra, "store"); ok {
		params["store"] = store
	}

	// ProviderExtra: truncation — 上下文截断策略 ("auto" / "disabled")
	if truncation, ok := provider.GetExtraString(config.ProviderExtra, "truncation"); ok && truncation != "" {
		params["truncation"] = truncation
	}

	// ProviderExtra: previous_response_id — 关联上一轮响应实现多轮对话
	if prevID, ok := provider.GetExtraString(config.ProviderExtra, "previous_response_id"); ok && prevID != "" {
		params["previous_response_id"] = prevID
	}

	// ProviderExtra: include — 指定响应中需包含的附加数据（如 reasoning.encrypted_content）
	if include, ok := provider.GetExtraAny(config.ProviderExtra, "include"); ok {
		if items, ok := include.([]string); ok && len(items) > 0 {
			params["include"] = items
		}
	}

	// ProviderExtra: modalities — 输出模态
	if modalities, ok := provider.GetExtraAny(config.ProviderExtra, "modalities"); ok {
		params["modalities"] = modalities
	}

	// Metadata — 附加键值对元数据（map[string]string → map[string]any）
	if len(config.Metadata) > 0 {
		meta := make(map[string]any, len(config.Metadata))
		for k, v := range config.Metadata {
			meta[k] = v
		}
		params["metadata"] = meta
	}

	// Tools — 工具定义
	if tools := buildTools(config.Tools); tools != nil {
		params["tools"] = tools
	}

	// Reasoning — 思考/推理配置（effort + summary 自动推导）
	// effort 为 "none" 时不输出 reasoning 字段。
	// 透传前经 NormalizeReasoningEffort 归一化（max→xhigh），与 Completions 出口保持一致：
	// qwen 等上游拒绝 "max"，归一化后语义等价（最高推理强度）且兼容所有上游。
	if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" && config.ThinkingConfig.Effort != "none" {
		effort := provider.NormalizeReasoningEffort(config.ThinkingConfig.Effort)
		reasoning := map[string]any{
			"effort": effort,
		}
		// Summary 按归一化后的 effort 自动推导（max 归一化为 xhigh，落入 detailed 档）
		switch effort {
		case "minimal", "low":
			reasoning["summary"] = "concise"
		case "medium":
			reasoning["summary"] = "auto"
		case "high", "xhigh":
			reasoning["summary"] = "detailed"
		}
		params["reasoning"] = reasoning
	}

	// ToolChoice — 工具选择策略（"forced" 统一映射为 "required"）
	if config.ToolChoice != "" {
		tc := config.ToolChoice
		if tc == "forced" {
			tc = "required"
		}
		params["tool_choice"] = tc
	}

	// ResponseFormat — 响应格式 (text / json_object)
	// Responses API 通过 text.format.type 字段指定响应格式
	if config.ResponseFormat != "" {
		switch config.ResponseFormat {
		case "text":
			params["text"] = map[string]any{
				"format": map[string]any{"type": "text"},
			}
		case "json_object":
			params["text"] = map[string]any{
				"format": map[string]any{"type": "json_object"},
			}
		}
	}

	return params
}

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
	if items := extraStringSlice(config.ProviderExtra, "include"); len(items) > 0 {
		params["include"] = items
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

	// Reasoning — 思考/推理配置。
	// effort 为 "none" 时不输出 reasoning 字段。
	// 透传前经 NormalizeReasoningEffort 归一化（max→xhigh）。
	// summary 只透传客户端显式给出的值：Grok 不接受 reasoning.summary，
	// 自动推导会触发上游校验失败；OpenAI 官方则要求显式 opt-in 才返回摘要。
	if config != nil {
		reasoning := map[string]any{}
		if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" && config.ThinkingConfig.Effort != "none" {
			reasoning["effort"] = provider.NormalizeReasoningEffort(config.ThinkingConfig.Effort)
		}
		if summary, ok := provider.GetExtraString(config.ProviderExtra, "reasoning_summary"); ok && summary != "" {
			reasoning["summary"] = summary
		}
		if mode, ok := provider.GetExtraString(config.ProviderExtra, "reasoning_mode"); ok && mode != "" {
			reasoning["mode"] = mode
		}
		if ctxMode, ok := provider.GetExtraString(config.ProviderExtra, "reasoning_context"); ok && ctxMode != "" {
			reasoning["context"] = ctxMode
		}
		if len(reasoning) > 0 {
			params["reasoning"] = reasoning
		}
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

// extraStringSlice 从 ProviderExtra 读取字符串数组。
// JSON 解进 map[string]any 后数组类型是 []any，不能直接断言 []string。
func extraStringSlice(extra map[string]any, key string) []string {
	v, ok := provider.GetExtraAny(extra, key)
	if !ok || v == nil {
		return nil
	}
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok || s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

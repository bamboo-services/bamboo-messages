package responses

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// OpenAI Responses API 的缓存相关取值白名单。
//
// 与 Chat Completions 的差异（依据官方 Responses API 参考文档）：
//
//   - 请求级 prompt_cache_options.ttl："30m" / "12h"（Chat Completions 仅支持 "30m"）；
//   - 块级 prompt_cache_breakpoint.ttl："30m" / "12h"；
//   - prompt_cache_options 还支持 prewarm 与 comparison_response_id 两个
//     Responses 专属字段（通过 ProviderExtra 传入）。
//
// 超出白名单的取值一律丢弃，退化为该层级的默认行为，
// 避免向上游发送非法值导致 400。
const (
	responsesCacheTTL30m = provider.CacheTTL30m
	responsesCacheTTL12h = provider.CacheTTL12h
)

// requestCacheTTL 返回请求级 prompt_cache_options.ttl 的合法取值。
//
// 不支持时返回空字符串，调用方据此省略 ttl 字段。
func requestCacheTTL(ttl provider.CacheTTL) string {
	switch ttl {
	case responsesCacheTTL30m, responsesCacheTTL12h:
		return string(ttl)
	}
	return ""
}

// breakpointCacheTTL 返回块级 prompt_cache_breakpoint.ttl 的合法取值。
//
// 不支持时返回空字符串，调用方据此省略 ttl 字段（继承请求级 ttl）。
func breakpointCacheTTL(ttl provider.CacheTTL) string {
	switch ttl {
	case responsesCacheTTL30m, responsesCacheTTL12h:
		return string(ttl)
	}
	return ""
}

// cacheBreakpointEnabled 判断是否允许发射块级缓存断点。
//
// prompt_cache_breakpoint 是 gpt-5.6+ 的新字段，对更早的模型发送未知字段
// 会被上游拒绝（400）。因此块级断点默认不发射，需要使用者通过
// PromptCacheOptions.EnableBlockBreakpoints 显式确认目标模型支持后再开启；
// 显式声明 Mode 为 explicit 时同样自动开启。
//
// 未开启时 OpenAI 会自动在最新一条合格消息末尾放置断点，缓存行为不受影响。
func cacheBreakpointEnabled(config *provider.ChatConfig) bool {
	if config == nil {
		return false
	}
	return config.PromptCacheOptions.BlockBreakpointsEnabled()
}

// systemCacheControl 安全读取配置中的 system 级缓存标记。
func systemCacheControl(config *provider.ChatConfig) *provider.CacheControl {
	if config == nil {
		return nil
	}
	return config.SystemCacheControl
}

// buildPromptCacheOptions 构建 OpenAI 请求级缓存选项 prompt_cache_options。
//
// 返回 nil 表示不发送该字段：未显式配置 mode/ttl 时保持上游默认行为，
// 避免老模型因收到不支持的参数而报 400。
func buildPromptCacheOptions(config *provider.ChatConfig) map[string]any {
	if config == nil || config.PromptCacheOptions == nil {
		return nil
	}
	opts := config.PromptCacheOptions
	result := map[string]any{}
	switch opts.Mode {
	case provider.PromptCacheModeImplicit, provider.PromptCacheModeExplicit:
		result["mode"] = string(opts.Mode)
	}
	if ttl := requestCacheTTL(opts.TTL); ttl != "" {
		result["ttl"] = ttl
	}
	// prewarm — Responses 专属：只预热 prompt 缓存而不生成输出。
	if prewarm, ok := provider.GetExtraBool(config.ProviderExtra, "prewarm"); ok {
		result["prewarm"] = prewarm
	}
	// comparison_response_id — Responses 专属：指定用于对比的响应 ID，
	// 请求上游返回 prompt cache 复用诊断信息。
	if id, ok := provider.GetExtraString(config.ProviderExtra, "comparison_response_id"); ok && id != "" {
		result["comparison_response_id"] = id
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// buildPromptCacheRetention 构建 OpenAI 请求级缓存保留策略 prompt_cache_retention。
//
// 该字段用于 gpt-5.6 之前的模型（官方已标记 deprecated，新模型改用
// prompt_cache_options.ttl）。仅在显式配置时发送。
//
// 返回空字符串表示不发送该字段。
func buildPromptCacheRetention(config *provider.ChatConfig) string {
	if config == nil || config.PromptCacheOptions == nil {
		return ""
	}
	switch config.PromptCacheOptions.Retention {
	case provider.PromptCacheRetentionInMemory, provider.PromptCacheRetention24h:
		return string(config.PromptCacheOptions.Retention)
	}
	return ""
}

// buildCacheBreakpoint 构建 OpenAI 块级缓存断点 prompt_cache_breakpoint。
//
// OpenAI 的 prompt_cache_breakpoint 标记「可复用 prompt 前缀的精确末尾」，
// 与 Anthropic 的块级 cache_control 断点语义一致，因此由统一的
// provider.CacheControl 映射而来。
//
// 返回 nil 表示不附加断点：未提供缓存标记，或未启用 explicit 模式。
func buildCacheBreakpoint(cc *provider.CacheControl, config *provider.ChatConfig) map[string]any {
	if cc == nil || !cacheBreakpointEnabled(config) {
		return nil
	}
	bp := map[string]any{"mode": "explicit"}
	if ttl := breakpointCacheTTL(cc.TTL); ttl != "" {
		bp["ttl"] = ttl
	}
	return bp
}

// applyCacheBreakpoint 把缓存断点挂到内容块的最后一个输入文本块上。
//
// Responses 的断点只能挂在输入类内容块上（input_text / input_image /
// input_file），assistant 的 output_text 不支持断点，因此这里只处理
// input_text 块。
//
// parts 为空或未启用 explicit 模式时不做任何处理。
func applyCacheBreakpoint(parts []map[string]any, cc *provider.CacheControl, config *provider.ChatConfig) {
	bp := buildCacheBreakpoint(cc, config)
	if bp == nil || len(parts) == 0 {
		return
	}
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i]["type"] == "input_text" {
			parts[i]["prompt_cache_breakpoint"] = bp
			return
		}
	}
}

// buildSystemCacheItem 构建承载可缓存系统提示的 developer 消息项。
//
// Responses 的顶层 instructions 是纯字符串，无法携带缓存断点；官方要求
// 「把可复用的 developer 指令放进 developer 消息的 input_text 块里」。
//
// 返回 nil 表示当前配置下不使用 developer 消息承载系统提示，
// 调用方应继续使用顶层 instructions 字段。
func buildSystemCacheItem(text string, cc *provider.CacheControl, config *provider.ChatConfig) map[string]any {
	bp := buildCacheBreakpoint(cc, config)
	if bp == nil || text == "" {
		return nil
	}
	return map[string]any{
		"role": "developer",
		"content": []map[string]any{
			{
				"type":                    "input_text",
				"text":                    text,
				"prompt_cache_breakpoint": bp,
			},
		},
	}
}

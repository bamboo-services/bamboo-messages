package bamboo

import "github.com/bamboo-services/bamboo-messages/provider" // 保留：ClientOption 系统依赖 provider.Provider

// ClientOption 客户端配置选项函数。
//
// 通过 Functional Options 模式设置客户端配置，
// 如 WithProvider、WithDefaultModel 等。
type ClientOption func(*clientConfig)

// clientConfig 客户端内部配置，通过 ClientOption 函数设置。
type clientConfig struct {
	provider     provider.Provider
	defaultModel string
}

// WithProvider 设置底层协议适配器。
//
// 用于指定使用哪个协议的 Provider 实例（Anthropic/OpenAI 等）。
func WithProvider(p provider.Provider) ClientOption {
	return func(c *clientConfig) {
		c.provider = p
	}
}

// WithDefaultModel 设置默认模型名称。
//
// 当 RequestConfig 未指定 Model 时使用此默认值。
func WithDefaultModel(model string) ClientOption {
	return func(c *clientConfig) {
		c.defaultModel = model
	}
}

// NewClientWithOptions 通过 Functional Options 创建客户端。
//
// 至少需要提供 WithProvider 选项，否则 panic。
func NewClientWithOptions(opts ...ClientOption) BambooClient {
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.provider == nil {
		panic("bamboo: provider 不能为空")
	}
	return &client{
		provider:     cfg.provider,
		defaultModel: cfg.defaultModel,
	}
}

// RequestOption 请求配置选项函数，用于灵活配置 RequestConfig。
type RequestOption func(*RequestConfig)

// WithExtra 设置 Provider 特有的扩展参数。
//
// 用于传递任何额外的配置参数，
// 直接写入 ProviderExtra map 中。
func WithExtra(key string, value any) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ProviderExtra == nil {
			cfg.ProviderExtra = make(map[string]any)
		}
		cfg.ProviderExtra[key] = value
	}
}

// WithToolChoice 设置工具选择策略（如 "auto"/"none"/"required"）。
func WithToolChoice(choice string) RequestOption {
	return func(cfg *RequestConfig) { cfg.ToolChoice = choice }
}

// WithResponseFormat 设置响应格式（如 "json_object"/"text"）。
func WithResponseFormat(format string) RequestOption {
	return func(cfg *RequestConfig) { cfg.ResponseFormat = format }
}

// WithUserID 设置用户标识。
func WithUserID(userID string) RequestOption {
	return func(cfg *RequestConfig) { cfg.UserID = userID }
}

// WithParallelToolCalls 设置是否允许并行工具调用。
func WithParallelToolCalls(enabled bool) RequestOption {
	return func(cfg *RequestConfig) { cfg.ParallelToolCalls = enabled }
}

// WithSystemCacheControl 设置 system prompt 的缓存控制标记。
//
// 用于 Anthropic prompt caching，在 system prompt 上设置缓存断点。
func WithSystemCacheControl(cc *provider.CacheControl) RequestOption {
	return func(cfg *RequestConfig) { cfg.SystemCacheControl = cc }
}

// WithPromptCacheKey 设置 OpenAI prompt cache 路由粘性键。
//
// 相同 key 的请求会尽量复用同一缓存路径，提升缓存命中率。
func WithPromptCacheKey(key string) RequestOption {
	return func(cfg *RequestConfig) { cfg.PromptCacheKey = key }
}

// WithThinkingDisplay 设置思考内容的显示模式。
//
// 支持 "summarized"（摘要显示）和 "omitted"（完全隐藏）两种模式。
func WithThinkingDisplay(display string) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ThinkingConfig == nil {
			cfg.ThinkingConfig = &ThinkingConfig{}
		}
		cfg.ThinkingConfig.Display = display
	}
}

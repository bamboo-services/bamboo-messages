package bamboo

import "github.com/bamboo-services/bamboo-messages/internal/provider"

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
		panic("bamboo: provider must not be nil")
	}
	return &client{
		provider:     cfg.provider,
		defaultModel: cfg.defaultModel,
	}
}

// RequestOption 请求配置选项函数，用于灵活配置 RequestConfig。
//
// 通过 Functional Options 模式设置请求参数，
// 如 WithTopK、WithFrequencyPenalty 等。
type RequestOption func(*RequestConfig)

// WithTopK 设置 Top-K 采样参数。
//
// 仅部分 Provider 支持（如 Anthropic），
// 参数值通过 ProviderExtra 透传到底层适配器。
func WithTopK(topK float64) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ProviderExtra == nil {
			cfg.ProviderExtra = make(map[string]any)
		}
		cfg.ProviderExtra[provider.ProviderExtraKeyTopK] = topK
	}
}

// WithFrequencyPenalty 设置频率惩罚参数。
//
// 仅部分 Provider 支持（如 OpenAI），
// 参数值通过 ProviderExtra 透传到底层适配器。
func WithFrequencyPenalty(penalty float64) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ProviderExtra == nil {
			cfg.ProviderExtra = make(map[string]any)
		}
		cfg.ProviderExtra[provider.ProviderExtraKeyFrequencyPenalty] = penalty
	}
}

// WithPresencePenalty 设置存在惩罚参数。
//
// 仅部分 Provider 支持（如 OpenAI），
// 参数值通过 ProviderExtra 透传到底层适配器。
func WithPresencePenalty(penalty float64) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ProviderExtra == nil {
			cfg.ProviderExtra = make(map[string]any)
		}
		cfg.ProviderExtra[provider.ProviderExtraKeyPresencePenalty] = penalty
	}
}

// WithSeed 设置随机种子，用于控制生成结果的确定性。
//
// 仅部分 Provider 支持（如 OpenAI Completions），
// 参数值通过 ProviderExtra 透传到底层适配器。
func WithSeed(seed int64) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ProviderExtra == nil {
			cfg.ProviderExtra = make(map[string]any)
		}
		cfg.ProviderExtra[provider.ProviderExtraKeySeed] = seed
	}
}

// WithToolChoice 设置工具选择策略。
//
// 仅部分 Provider 支持（如 Anthropic/OpenAI），
// 参数值通过 ProviderExtra 透传到底层适配器。
func WithToolChoice(choice any) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ProviderExtra == nil {
			cfg.ProviderExtra = make(map[string]any)
		}
		cfg.ProviderExtra[provider.ProviderExtraKeyToolChoice] = choice
	}
}

// WithResponseFormat 设置响应格式。
//
// 仅部分 Provider 支持（如 OpenAI），
// 参数值通过 ProviderExtra 透传到底层适配器。
func WithResponseFormat(format any) RequestOption {
	return func(cfg *RequestConfig) {
		if cfg.ProviderExtra == nil {
			cfg.ProviderExtra = make(map[string]any)
		}
		cfg.ProviderExtra[provider.ProviderExtraKeyResponseFormat] = format
	}
}

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
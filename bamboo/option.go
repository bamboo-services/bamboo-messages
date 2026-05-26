package bamboo

import "github.com/bamboo-services/bamboo-messages/internal/provider"

// ClientOption 客户端配置选项函数。
type ClientOption func(*clientConfig)

// clientConfig 客户端内部配置，通过 ClientOption 函数设置。
type clientConfig struct {
	provider     provider.Provider
	defaultModel string
}

// WithProvider 设置底层协议适配器。
func WithProvider(p provider.Provider) ClientOption {
	return func(c *clientConfig) {
		c.provider = p
	}
}

// WithDefaultModel 设置默认模型名称。
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
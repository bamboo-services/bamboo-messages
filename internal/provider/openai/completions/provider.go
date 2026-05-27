package completions

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// CompletionsProvider OpenAI Chat Completions 协议适配器实现。
//
// 基于泛型基座 provider.BaseProvider，封装 OpenAI Chat Completions SDK Client。
type CompletionsProvider provider.BaseProvider[openai.Client]

// ============================================
// Options 模式 — Functional Options
// ============================================

// Option 配置 CompletionsProvider 的函数选项。
//
// 通过 WithAPIKey、WithBaseURL、WithHeader 等选项函数应用配置。
type Option func(*config)

// config Provider 运行时配置。
//
// 存储 API Key、BaseURL、Headers 等配置项。
type config struct {
	apiKey  string
	baseURL string
	headers map[string]string
}

// WithAPIKey 设置 API 密钥。
//
// 用于向 OpenAI API 进行身份验证。
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithBaseURL 设置自定义基础 URL
//
// 用于将请求指向非官方端点，例如：
//   - 自建 API 网关 / 代理服务
//   - Azure OpenAI、one-api、new-api 等 OpenAI 兼容服务
//   - 本地模型部署（Ollama、vLLM 等 OpenAI 兼容端点）
//
// 留空则使用 SDK 默认端点。
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithHeader 添加自定义 HTTP 请求头
func WithHeader(key, value string) Option {
	return func(c *config) {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers[key] = value
	}
}

// ============================================
// 构造函数
// ============================================

// NewCompletionsProvider 创建 OpenAI Chat Completions 协议适配器实例（最简形式）。
//
// 仅指定 API Key，默认连接 SDK 默认端点。
func NewCompletionsProvider(apiKey string) *CompletionsProvider {
	return NewCompletionsProviderWithOptions(WithAPIKey(apiKey))
}

// NewCompletionsProviderWithOptions 创建 OpenAI Chat Completions 协议适配器实例（Options 模式）。
//
// 支持完整的配置选项，包括自定义 BaseURL、Headers 等。
func NewCompletionsProviderWithOptions(opts ...Option) *CompletionsProvider {
	cfg := applyOptions(opts...)

	sdkOpts := []option.RequestOption{
		option.WithHeader("User-Agent", provider.GetUserAgent()),
	}
	if cfg.apiKey != "" {
		sdkOpts = append(sdkOpts, option.WithAPIKey(cfg.apiKey))
	}
	if cfg.baseURL != "" {
		sdkOpts = append(sdkOpts, option.WithBaseURL(cfg.baseURL))
	}
	for k, v := range cfg.headers {
		sdkOpts = append(sdkOpts, option.WithHeader(k, v))
	}

	client := openai.NewClient(sdkOpts...)

	return &CompletionsProvider{
		Client: client,
	}
}

// applyOptions 将选项列表应用到默认配置
func applyOptions(opts ...Option) *config {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// GetProviderType 获取协议类型标识。
//
// 返回 provider.ProviderOpenAICompletions 常量，标识当前使用的协议类型。
func (p *CompletionsProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAICompletions
}

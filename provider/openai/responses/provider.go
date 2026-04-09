package responses

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ResponsesProvider OpenAI Responses 协议适配器实现
type ResponsesProvider provider.BaseProvider[openai.Client]

// ============================================
// Options 模式 — Functional Options
// ============================================

// Option 配置 ResponsesProvider 的函数选项
type Option func(*config)

// config Provider 运行时配置
type config struct {
	apiKey  string
	baseURL string
headers map[string]string
}

// WithAPIKey 设置 API 密钥
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

// NewResponsesProvider 创建 OpenAI Responses 协议适配器实例（最简形式）
//
// 仅指定 API Key，默认连接 SDK 默认端点。
func NewResponsesProvider(apiKey string) *ResponsesProvider {
	return NewResponsesProviderWithOptions(WithAPIKey(apiKey))
}

// NewResponsesProviderWithOptions 创建 OpenAI Responses 协议适配器实例（Options 模式）
//
// 支持完整的配置选项，包括自定义 BaseURL、Headers 等。
func NewResponsesProviderWithOptions(opts ...Option) *ResponsesProvider {
	cfg := applyOptions(opts...)

	sdkOpts := []option.RequestOption{}
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

	return &ResponsesProvider{
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

// GetProviderType 获取协议类型标识
func (p *ResponsesProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAIResponses
}

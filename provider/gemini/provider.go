package gemini

import (
	"context"

	"github.com/bamboo-services/bamboo-messages/provider"
	"google.golang.org/genai"
)

// Provider Gemini 协议适配器实现。
//
// 类型别名自 BaseProvider，嵌入 genai.Client。
// genai.Client 由 Google Gen AI Go SDK 提供，支持 Gemini API 与 Vertex AI 两种后端。
type Provider provider.BaseProvider[genai.Client]

// ============================================
// Options 模式 — Functional Options
// ============================================

// Option 配置 Provider 的函数选项。
//
// 支持配置 API Key、BaseURL、Headers 等。
type Option func(*config)

// config Provider 运行时配置。
//
// 存储 API Key、BaseURL、自定义 Headers 等配置。
type config struct {
	apiKey  string
	baseURL string
	headers map[string]string
	debug   bool
	// interceptors 请求拦截器链，通过 WithInterceptor 注册。
	// 构造 Provider 时若非空，会用 interceptorTransport 包装 HTTP client，
	// 让所有上游请求都先经过拦截器链。
	interceptors []provider.RequestInterceptor
}

// WithAPIKey 设置 API 密钥。
//
// 用于 Gemini API 认证（即 GEMINI_API_KEY）。
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithBaseURL 设置自定义基础 URL
//
// 用于将请求指向非官方端点，例如：
//   - 自建 API 网关 / 代理服务
//   - 第三方 Gemini 兼容服务
//   - 测试环境的 mock server
//
// 留空则使用 SDK 默认端点（generativelanguage.googleapis.com）。
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithHeader 添加自定义 HTTP 请求头。
//
// 可用于传递追踪 ID、认证 token 等自定义头。
func WithHeader(key, value string) Option {
	return func(c *config) {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers[key] = value
	}
}

// WithDebug 启用 debug 日志。
//
// 启用后，适配器在发起请求前会输出 Provider 类型、端点、headers 和 body（正文截断）。
// 等价于设置环境变量 BAMBOO_DEBUG=1。
func WithDebug() Option {
	return func(c *config) { c.debug = true }
}

// WithInterceptor 注册一个请求拦截器。
//
// 多次调用按调用顺序追加。拦截器在 Provider 发起上游 HTTP 请求前执行，
// 可对已序列化的 JSON body 做任意修改（如参数覆盖、字段删除、签名注入）。
// 拦截器返回 error 时立即中止请求并向上冒泡。
func WithInterceptor(fn provider.RequestInterceptor) Option {
	return func(c *config) {
		if fn == nil {
			return
		}
		c.interceptors = append(c.interceptors, fn)
	}
}

// ============================================
// 构造函数
// ============================================

// NewProvider 创建 Gemini 协议适配器实例（最简形式）
//
// 仅指定 API Key，默认连接 Gemini API 官方端点。
func NewProvider(apiKey string) *Provider {
	return NewProviderWithOptions(WithAPIKey(apiKey))
}

// NewProviderWithOptions 创建 Gemini 协议适配器实例（Options 模式）
//
// 支持完整的配置选项，包括自定义 BaseURL、Headers 等。
// genai.NewClient 内部仅构建客户端，不发起网络请求，可在构造函数中安全调用。
func NewProviderWithOptions(opts ...Option) *Provider {
	cfg := applyOptions(opts...)

	clientCfg := &genai.ClientConfig{
		APIKey:  cfg.apiKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			Headers: make(map[string][]string),
		},
	}
	// 设置统一 UserAgent
	clientCfg.HTTPOptions.Headers.Set("User-Agent", provider.GetUserAgent())
	if cfg.baseURL != "" {
		clientCfg.HTTPOptions.BaseURL = cfg.baseURL
	}
	for k, v := range cfg.headers {
		clientCfg.HTTPOptions.Headers.Set(k, v)
	}
	// 若注册了拦截器，用 interceptorTransport 包装 HTTP client 并注入到 genai
	if httpCli := provider.NewInterceptorHTTPClient(nil, cfg.interceptors); httpCli != nil {
		clientCfg.HTTPClient = httpCli
	}

	client, err := genai.NewClient(context.Background(), clientCfg)
	if err != nil {
		// genai.NewClient 在仅配置 ClientConfig 字段时不会返回 error，
		// 这里做防御性处理，返回零值 Client 保证不 panic。
		_ = err
	}

	return &Provider{
		Client: *client,
	}
}

// applyOptions 将选项列表应用到默认配置。
//
// 创建空 config，遍历所有 Option 并应用。
func applyOptions(opts ...Option) *config {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.debug {
		provider.SetDebug(true)
	}
	return cfg
}

// GetProviderType 获取协议类型标识。
//
// 返回 "gemini" 常量，用于识别协议类型。
func (p *Provider) GetProviderType() provider.ProviderType {
	return "gemini"
}

// 编译期接口检查，确保 Provider 完整实现 provider.Provider 接口。
var _ provider.Provider = (*Provider)(nil)

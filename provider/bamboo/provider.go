package bamboo

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Provider bamboo 原生协议适配器实现。
//
// 统一持有 *provider.HTTPClient 进行 HTTP 通信，认证使用 Authorization Bearer 模式。
// 该适配器面向 bamboo 原生协议端点，是 bamboo/codec 层的中继目标之一。
type Provider struct {
	httpClient *provider.HTTPClient
}

// ============================================
// Options 模式 — Functional Options
// ============================================

// Option 配置 Provider 的函数选项。
//
// 支持配置 API Key、BaseURL、Headers、请求拦截器等。
type Option func(*config)

// config Provider 运行时配置。
//
// 存储 API Key、BaseURL、自定义 Headers 和拦截器等配置。
type config struct {
	apiKey       string
	baseURL      string
	headers      map[string]string
	interceptors []provider.RequestInterceptor
}

// WithAPIKey 设置 API 密钥。
//
// 用于 bamboo 原生协议端点的 Bearer 认证。
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithBaseURL 设置自定义基础 URL。
//
// 用于连接自建网关、代理或第三方 bamboo 兼容端点。
// 留空则不在构造时设置默认端点，失败安全地推迟到请求时处理。
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithHeader 添加自定义 HTTP 请求头。
//
// 可用于传递追踪 ID、自定义认证头等。
func WithHeader(key, value string) Option {
	return func(c *config) {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers[key] = value
	}
}

// WithInterceptor 注册一个请求拦截器。
//
// 多次调用按调用顺序追加。拦截器在 Provider 发起上游 HTTP 请求前执行，
// 可对已序列化的 JSON body 做任意修改。
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

const defaultBaseURL = ""

// NewProvider 创建 bamboo 原生协议适配器实例（最简形式）。
//
// 仅指定 API Key，其余配置使用默认值。
func NewProvider(apiKey string) *Provider {
	return NewProviderWithOptions(WithAPIKey(apiKey))
}

// NewProviderWithOptions 创建 bamboo 原生协议适配器实例（Options 模式）。
//
// 支持完整的配置选项，包括自定义 BaseURL、Headers 等。
// 认证使用 Authorization: Bearer 模式。
func NewProviderWithOptions(opts ...Option) *Provider {
	cfg := applyOptions(opts...)

	baseURL := cfg.baseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	httpClient := provider.NewHTTPClient(
		baseURL,
		cfg.apiKey,
		"Authorization",
		"Bearer ",
		cfg.headers,
		cfg.interceptors,
	)

	return &Provider{httpClient: httpClient}
}

func applyOptions(opts ...Option) *config {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// GetProviderType 获取协议类型标识。
//
// 返回 provider.ProviderBamboo 常量。
func (p *Provider) GetProviderType() provider.ProviderType {
	return provider.ProviderBamboo
}

// 编译期接口满足检查。
var _ provider.Provider = (*Provider)(nil)

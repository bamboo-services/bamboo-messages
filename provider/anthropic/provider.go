package anthropic

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Provider Anthropic Messages 协议适配器实现。
//
// 在去 SDK 化重构后，不再嵌入 anthropic-sdk-go Client，
// 而是统一持有 *provider.HTTPClient 进行 HTTP 通信。
// legacyCompat 标志控制旧版端点兼容行为（如 GLM/Kimi 等 Anthropic 兼容端点）。
type Provider struct {
	httpClient     *provider.HTTPClient
	legacyCompat   bool
	degradedReason provider.DegradedReasonStrategy
}

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
	apiKey         string
	baseURL        string
	headers        map[string]string
	legacyCompat   bool
	interceptors   []provider.RequestInterceptor
	degradedReason provider.DegradedReasonStrategy
}

// WithAPIKey 设置 API 密钥。
//
// 用于 Anthropic API 认证。
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithBaseURL 设置自定义基础 URL
//
// 用于将请求指向非官方端点，例如：
//   - 自建 API 网关 / 代理服务
//   - 第三方 Anthropic 兼容服务
//   - 测试环境的 mock server
//
// 留空则使用 SDK 默认端点。
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

// WithLegacyCompat 启用旧版兼容模式。
//
// 适用于 GLM/Kimi 等第三方 Anthropic 兼容端点，这些端点仅识别 thinking.type:"enabled"，
// 不支持 Anthropic 官方的 thinking.type:"adaptive" 自适应模式。
//
// 启用后的行为差异：
//   - thinking 参数: type 强制为 "enabled"（adaptive → enabled），保留 budget_tokens
//   - 优先读取 ProviderExtra["thinking"] 原始配置（跨协议场景保留 budget_tokens 等）
//   - 无原始配置时从 ThinkingConfig.Effort 合成 {type:"enabled"}
func WithLegacyCompat() Option {
	return func(c *config) { c.legacyCompat = true }
}

// WithInterceptor 注册一个请求拦截器。
//
// 多次调用按调用顺序追加。拦截器在 Provider 发起上游 HTTP 请求前执行，
// 可对已序列化的 JSON body 做任意修改（如参数覆盖、字段删除、签名注入）。
// 拦截器返回 error 时立即中止请求并向上冒泡。
//
// 这是 provider.WithInterceptor 的 anthropic 包级 wrapper，
// 内部直接转发到 provider.RequestInterceptor 类型。
func WithInterceptor(fn provider.RequestInterceptor) Option {
	return func(c *config) {
		if fn == nil {
			return
		}
		c.interceptors = append(c.interceptors, fn)
	}
}

// WithDegradedReason 设置流式中断降级时补发的完成原因策略。
//
// 默认 DegradedReasonStop。Agent Loop 场景启用 DegradedReasonToolUse 后，
// 带 tools 请求中断时返回 tool_calls 让 agent 继续轮询。
func WithDegradedReason(strategy provider.DegradedReasonStrategy) Option {
	return func(c *config) { c.degradedReason = strategy }
}

// ============================================
// 构造函数
// ============================================

const defaultBaseURL = "https://api.anthropic.com"

// NewProvider 创建 Anthropic Messages 协议适配器实例（最简形式）
//
// 仅指定 API Key，默认连接 SDK 默认端点。
func NewProvider(apiKey string) *Provider {
	return NewProviderWithOptions(WithAPIKey(apiKey))
}

// NewProviderWithOptions 创建 Anthropic Messages 协议适配器实例（Options 模式）
//
// 支持完整的配置选项，包括自定义 BaseURL、Headers 等。
// 构造时注入 anthropic-version 请求头，认证使用 x-api-key 模式。
func NewProviderWithOptions(opts ...Option) *Provider {
	cfg := applyOptions(opts...)

	// Anthropic 必须的版本头
	if cfg.headers == nil {
		cfg.headers = make(map[string]string)
	}
	cfg.headers["anthropic-version"] = "2023-06-01"

	baseURL := cfg.baseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	httpClient := provider.NewHTTPClient(
		baseURL,
		cfg.apiKey,
		"x-api-key",
		"",
		cfg.headers,
		cfg.interceptors,
	)

	return &Provider{
		httpClient:     httpClient,
		legacyCompat:   cfg.legacyCompat,
		degradedReason: cfg.degradedReason,
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
	return cfg
}

// GetProviderType 获取协议类型标识。
//
// 返回 ProviderAnthropic 常量，用于识别协议类型。
func (p *Provider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

package completions

import (
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type CompletionsProvider struct {
	provider.BaseProvider[openai.Client]
	legacyCompat      bool
	streamDrainTimeout time.Duration
}

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
	apiKey             string
	baseURL            string
	headers            map[string]string
	legacyCompat       bool
	debug              bool
	streamDrainTimeout time.Duration
	interceptors       []provider.RequestInterceptor
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

// WithLegacyCompat 启用旧版兼容模式。
//
// 开启后，CompletionsProvider 将使用旧版行为模式，
// 用于兼容早期 API 响应格式或特定第三方端点的非标准行为。
func WithLegacyCompat() Option {
	return func(c *config) { c.legacyCompat = true }
}

func WithStreamDrainTimeout(d time.Duration) Option {
	return func(c *config) { c.streamDrainTimeout = d }
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
	// 若注册了拦截器，用 interceptorTransport 包装 HTTP client 并注入到 SDK
	if httpCli := provider.NewInterceptorHTTPClient(nil, cfg.interceptors); httpCli != nil {
		sdkOpts = append(sdkOpts, option.WithHTTPClient(httpCli))
	}

	client := openai.NewClient(sdkOpts...)

	return &CompletionsProvider{
		BaseProvider: provider.BaseProvider[openai.Client]{
			Client: client,
		},
		legacyCompat:       cfg.legacyCompat,
		streamDrainTimeout: cfg.streamDrainTimeout,
	}
}

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
// 返回 provider.ProviderOpenAICompletions 常量，标识当前使用的协议类型。
func (p *CompletionsProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAICompletions
}

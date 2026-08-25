package responses

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ResponsesProvider OpenAI Responses 协议适配器实现。
//
// 在去 SDK 化重构后，不再嵌入 openai-go Client，
// 而是统一持有 *provider.HTTPClient 进行 HTTP 通信。
type ResponsesProvider struct {
	httpClient              *provider.HTTPClient
	degradedReason          provider.DegradedReasonStrategy
	includeReasoningContent bool
	ignoreEncryptedContent  bool
}

// ============================================
// Options 模式 — Functional Options
// ============================================

// Option 配置 ResponsesProvider 的函数选项。
//
// 用于 Functional Options 模式构造 ResponsesProvider 实例，
// 支持设置 API 密钥、自定义端点、附加请求头等配置。
type Option func(*config)

// config Provider 运行时配置。
//
// 保存 ResponsesProvider 的配置信息，
// 包括 API 密钥、自定义基础 URL 和附加请求头。
type config struct {
	apiKey                  string
	baseURL                 string
	headers                 map[string]string
	interceptors            []provider.RequestInterceptor
	degradedReason          provider.DegradedReasonStrategy
	includeReasoningContent bool
	ignoreEncryptedContent  bool
}

// WithAPIKey 设置 API 密钥。
//
// 用于 OpenAI 官方 API 认证，或兼容端点的密钥认证。
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

// WithDegradedReason 设置流式中断降级时补发的完成原因策略。
//
// 默认 DegradedReasonStop。Agent Loop 场景启用 DegradedReasonToolUse 后，
// 带 tools 请求中断时返回 tool_calls 让 agent 继续轮询。
func WithDegradedReason(strategy provider.DegradedReasonStrategy) Option {
	return func(c *config) { c.degradedReason = strategy }
}

// WithIncludeReasoningContent 设置是否在 input reasoning item 中携带 content 数组。
//
// 默认为 false（严格遵守 OpenAI 官方 OpenAPI Schema，content.maxItems = 0）。
// 当上游为支持明文思考回传的第三方 Open Responses 网关（如 Grok / vLLM / SGLang）时可按需开启。
func WithIncludeReasoningContent(include bool) Option {
	return func(c *config) { c.includeReasoningContent = include }
}

// WithIgnoreEncryptedContent 设置是否在构建 input 时忽略 encrypted_content。
//
// 默认为 false（保留密文）。当存在多 Key 轮询或跨组织调用、防止解密失败导致 400 时可开启。
func WithIgnoreEncryptedContent(ignore bool) Option {
	return func(c *config) { c.ignoreEncryptedContent = ignore }
}

// ============================================
// 构造函数
// ============================================

const defaultBaseURL = "https://api.openai.com/v1"

// NewResponsesProvider 创建 OpenAI Responses 协议适配器实例（最简形式）
//
// 仅指定 API Key，默认连接 SDK 默认端点。
func NewResponsesProvider(apiKey string) *ResponsesProvider {
	return NewResponsesProviderWithOptions(WithAPIKey(apiKey))
}

// NewResponsesProviderWithOptions 创建 OpenAI Responses 协议适配器实例（Options 模式）
//
// 支持完整的配置选项，包括自定义 BaseURL、Headers 等。
// 认证使用 Authorization: Bearer 模式。
func NewResponsesProviderWithOptions(opts ...Option) *ResponsesProvider {
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

	return &ResponsesProvider{
		httpClient:              httpClient,
		degradedReason:          cfg.degradedReason,
		includeReasoningContent: cfg.includeReasoningContent,
		ignoreEncryptedContent:  cfg.ignoreEncryptedContent,
	}
}

// applyOptions 将选项列表应用到默认配置。
//
// 按顺序应用所有 Option 函数到初始配置，返回最终配置实例。
func applyOptions(opts ...Option) *config {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// GetProviderType 获取协议类型标识。
//
// 返回 provider.ProviderOpenAIResponses，标识此 Provider 使用 OpenAI Responses 协议。
func (p *ResponsesProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAIResponses
}

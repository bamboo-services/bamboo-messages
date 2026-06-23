// Package option 提供通用的 Functional Options 模式实现。
//
// 该包定义了 Provider 适配器通用的配置选项模式，包括：
//   - WithAPIKey: 设置 API 密钥
//   - WithBaseURL: 设置自定义基础 URL
//   - WithHeader: 添加自定义 HTTP 请求头
//   - WithDebug: 启用 debug 日志
//
// 所有 Provider 适配器都可以使用此包来实现一致的配置模式，
// 减少重复代码，提高可维护性。
package option

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Option 配置 Provider 的函数选项。
//
// 用于设置 API Key、BaseURL、Headers 等通用配置。
// 各适配器可以扩展此类型以添加特有配置。
type Option func(*Config)

// Config Provider 运行时配置。
//
// 存储 API Key、BaseURL、自定义 Headers 等通用配置。
// 各适配器可以嵌入此结构体以添加特有配置。
type Config struct {
	// APIKey API 密钥，用于身份验证。
	APIKey string

	// BaseURL 自定义基础 URL。
	//
	// 用于将请求指向非官方端点，例如：
	//   - 自建 API 网关 / 代理服务
	//   - 第三方兼容服务
	//   - 测试环境的 mock server
	//
	// 留空则使用 SDK 默认端点。
	BaseURL string

	// Headers 自定义 HTTP 请求头。
	//
	// 可用于传递追踪 ID、认证 token 等自定义头。
	Headers map[string]string

	// Debug 是否启用 debug 日志。
	//
	// 启用后，适配器在发起请求前会输出 Provider 类型、端点、headers 和 body（正文截断）。
	// 等价于设置环境变量 BAMBOO_DEBUG=1。
	Debug bool
}

// WithAPIKey 设置 API 密钥。
//
// 用于 API 认证。
func WithAPIKey(key string) Option {
	return func(c *Config) { c.APIKey = key }
}

// WithBaseURL 设置自定义基础 URL。
//
// 用于将请求指向非官方端点，例如：
//   - 自建 API 网关 / 代理服务
//   - 第三方兼容服务
//   - 测试环境的 mock server
//
// 留空则使用 SDK 默认端点。
func WithBaseURL(url string) Option {
	return func(c *Config) { c.BaseURL = url }
}

// WithHeader 添加自定义 HTTP 请求头。
//
// 可用于传递追踪 ID、认证 token 等自定义头。
func WithHeader(key, value string) Option {
	return func(c *Config) {
		if c.Headers == nil {
			c.Headers = make(map[string]string)
		}
		c.Headers[key] = value
	}
}

// WithDebug 启用 debug 日志。
//
// 启用后，适配器在发起请求前会输出 Provider 类型、端点、headers 和 body（正文截断）。
// 等价于设置环境变量 BAMBOO_DEBUG=1。
func WithDebug() Option {
	return func(c *Config) { c.Debug = true }
}

// ApplyOptions 将选项列表应用到默认配置。
//
// 创建空 Config，遍历所有 Option 并应用。
// 如果启用了 Debug，会自动调用 provider.SetDebug(true)。
func ApplyOptions(opts ...Option) *Config {
	cfg := &Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.Debug {
		provider.SetDebug(true)
	}
	return cfg
}

// GetAPIKey 获取 API 密钥。
func (c *Config) GetAPIKey() string {
	return c.APIKey
}

// GetBaseURL 获取自定义基础 URL。
func (c *Config) GetBaseURL() string {
	return c.BaseURL
}

// GetHeaders 获取自定义 HTTP 请求头。
func (c *Config) GetHeaders() map[string]string {
	return c.Headers
}

// IsDebug 是否启用 debug 日志。
func (c *Config) IsDebug() bool {
	return c.Debug
}

// SetAPIKey 设置 API 密钥。
func (c *Config) SetAPIKey(key string) {
	c.APIKey = key
}

// SetBaseURL 设置自定义基础 URL。
func (c *Config) SetBaseURL(url string) {
	c.BaseURL = url
}

// SetHeader 设置自定义 HTTP 请求头。
func (c *Config) SetHeader(key, value string) {
	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}
	c.Headers[key] = value
}

// SetDebug 设置 debug 日志开关。
func (c *Config) SetDebug(debug bool) {
	c.Debug = debug
}

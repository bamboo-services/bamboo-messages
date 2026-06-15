// Package relay 提供函数式协议互转 API。
//
// relay 层是 codec 与 bamboo client 的薄包装，通过 Relay() 和 RelayStream()
// 两个函数实现不同 AI 对话协议（OpenAI / Anthropic / Responses / Gemini）之间的
// 请求-响应互转，无需上层业务关心协议差异。
//
// 使用方式：
//
//	// 非流式互转：OpenAI 格式请求 → Anthropic 格式响应
//	out, err := relay.Relay(ctx, provider, body, codec.FormatOpenAI, codec.FormatAnthropic)
//
//	// 流式互转
//	ch, err := relay.RelayStream(ctx, provider, body, codec.FormatOpenAI, codec.FormatAnthropic)
//	for data := range ch {
//	    write(data) // SSE 数据帧
//	}
package relay

import "github.com/bamboo-services/bamboo-messages/bamboo"

// Config relay 运行时配置，携带可选的回调函数。
//
// 通过 Option 函数式选项配置，零值 Config 即可正常工作。
type Config struct {
	// OnUsage Token 用量回调。
	// 在收到 message_delta 事件（携带 Usage）时触发。
	OnUsage func(usage bamboo.Usage)

	// OnError 错误回调。
	// 在 relay 过程中发生任何错误时触发（不影响错误返回，仅通知）。
	OnError func(err error)
}

// Option relay 配置选项函数。
//
// 采用 Functional Options 模式，通过 WithUsageCallback / WithErrorCallback
// 等函数配置 Config。
type Option func(*Config)

// WithUsageCallback 设置 Token 用量回调。
//
// 当流式或非流式响应中包含 Usage 信息时触发该回调。
// 适用于计费、监控等场景。
func WithUsageCallback(fn func(bamboo.Usage)) Option {
	return func(c *Config) {
		c.OnUsage = fn
	}
}

// WithErrorCallback 设置错误回调。
//
// 当 relay 过程中发生错误时触发该回调，错误仍会正常返回给调用方，
// 回调仅用于异步通知（如日志记录、告警等）。
func WithErrorCallback(fn func(error)) Option {
	return func(c *Config) {
		c.OnError = fn
	}
}

// applyOptions 应用所有选项并返回配置实例。
//
// 未传入任何选项时返回零值 Config，所有回调字段为 nil（安全跳过）。
func applyOptions(opts ...Option) *Config {
	cfg := &Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// triggerUsage 安全触发 Usage 回调。
func (c *Config) triggerUsage(usage bamboo.Usage) {
	if c != nil && c.OnUsage != nil {
		c.OnUsage(usage)
	}
}

// triggerError 安全触发 Error 回调。
func (c *Config) triggerError(err error) {
	if c != nil && c.OnError != nil && err != nil {
		c.OnError(err)
	}
}

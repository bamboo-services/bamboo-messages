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

import (
	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// Config relay 运行时配置，携带可选的回调函数和 debug 开关。
//
// 通过 Option 函数式选项配置，零值 Config 即可正常工作。
type Config struct {
	// OnUsage Token 用量回调。
	// 在收到 message_delta 事件（携带 Usage）时触发。
	OnUsage func(usage bamboo.Usage)

	// OnError 错误回调。
	// 在 relay 过程中发生任何错误时触发（不影响错误返回，仅通知）。
	OnError func(err error)

	// EstimateOnMissingUsage 当上游流中断导致 usage 数据缺失时，
	// 是否用累积的流内容估算 token 用量并触发 OnUsage 回调。
	//
	// 适用场景：部分上游（如智谱 GLM Coding-MAX）存在 SSE 帧截断 Bug，
	// 流式请求在 finish_reason 到达后、usage chunk 到达前中断，
	// 导致 OnUsage 回调永远不被触发，上层业务报 "上游没有返回计费信息"。
	//
	// 启用后，RelayStream 在 goroutine 退出时检测 usage 是否缺失，
	// 若缺失则基于已输出的文本内容（CJK 1:1, Latin 4:1 估算规则）
	// 和请求 messages 估算 input/output tokens。
	//
	// 估算精度的偏差约 ±20%，适用于按次计费等容错场景。
	// 通过 WithUsageEstimation(true) 启用。
	EstimateOnMissingUsage bool
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

// WithUsageEstimation 启用 usage 缺失时的估算回退。
//
// 当上游流中断导致 usage 数据未到达时，RelayStream 在退出时
// 基于已输出的文本内容估算 token 用量并触发 OnUsage 回调。
func WithUsageEstimation(enabled bool) Option {
	return func(c *Config) {
		c.EstimateOnMissingUsage = enabled
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

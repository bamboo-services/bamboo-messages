// Package anthropic 实现 Anthropic Messages 协议格式的 Codec 与流式序列化器。
//
// 本包负责将 Anthropic Messages 请求（含 system、messages、tools、thinking 等字段）解析为统一的
// [codec.RelayRequest]，并将内部响应与流式事件序列化为 Anthropic 标准的 SSE 帧（message_start、
// content_block_start、content_block_delta、content_block_stop、message_delta、message_stop）。
//
// 本包在 init() 阶段自动注册到 [codec.Register](codec.FormatAnthropic)。
package anthropic

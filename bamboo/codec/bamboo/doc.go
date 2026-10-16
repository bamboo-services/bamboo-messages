// Package bamboo 实现 Bamboo 原生协议格式的 Codec 恒等编解码器。
//
// 本包是跨协议中继中的恒等变换层，直接复用 [bamboo.BambooMessage]、[bamboo.RequestConfig]
// 与 [bamboo.Response] 完成解析与序列化，提供最高效的内部直通与 SSE 帧转发。
//
// 本包在 init() 阶段自动注册到 [codec.Register](codec.FormatBamboo)。
package bamboo

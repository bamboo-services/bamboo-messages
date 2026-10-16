// Package responses 实现 OpenAI Responses API 协议格式的 Codec 与有状态流式状态机。
//
// 本包负责将下一代 Responses 协议请求（含 input 数组、instructions、previous_response_id、
// stream_id、tools、reasoning 等字段）解析为统一中间表示，并通过内置状态机序列化为
// 标准的 response.created、response.output_item.added、response.output_text.delta、
// response.completed 等事件流。
//
// 本包在 init() 阶段自动注册到 [codec.Register](codec.FormatResponses)。
package responses

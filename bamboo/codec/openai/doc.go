// Package openai 实现 OpenAI Chat Completions 协议格式的 Codec 与流式序列化器。
//
// 本包负责将 OpenAI Chat Completions 格式的请求 JSON（含 developer/system、messages、tools、
// response_format、reasoning_effort 等字段）解析为 [codec.RelayRequest]，
// 并将响应转换为 chat.completion / chat.completion.chunk 格式的 JSON 与 SSE 帧序列。
//
// 本包在 init() 阶段自动注册到 [codec.Register](codec.FormatOpenAI)。
package openai

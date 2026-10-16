// Package gemini 实现 Google Gemini 原生协议格式的 Codec 与流式序列化器。
//
// 本包负责将 Gemini generateContent / streamGenerateContent 格式的请求体（含 contents、
// systemInstruction、generationConfig、tools 等）解析为统一模型，同时将内部事件
// 序列化为符合 Gemini 规范的 SSE chunk 帧流。
//
// 本包在 init() 阶段自动注册到 [codec.Register](codec.FormatGemini)。
package gemini

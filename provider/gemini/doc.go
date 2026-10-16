// Package gemini 实现面向 Google Gemini 原生 API 的 Provider 适配器。
//
// 本包通过 [provider.HTTPClient] 对接 Google AI Studio 或 Vertex AI 的 /v1beta 端点，
// 使用 x-goog-api-key 认证，支持 Gemini 1.5、2.0、2.5、3 以及下一代 Gemini 4 系列模型。
//
// # 协议特性支持
//
//   - 思考与签名保护：完整透传 Gemini 3/4 的 thinkingLevel（low/medium/high）与 thoughtSignature 原生加密签名，确保多轮工具调用时签名不丢失；
//   - 并发同名工具单次有序消费：内置 toolResultPool，根据 call_id 与有序索引单次严格消费工具结果，杜绝同名工具调用被多次兜底覆盖；
//   - 结构化工具策略：支持 AUTO、NONE、ANY 以及 Gemini 特有的 VALIDATED（配合 allowedFunctionNames 显式白名单）。
//
// # 构造入口
//
//   - [NewProvider]：最简构造函数；
//   - [NewProviderWithOptions]：完整构造函数，支持 BaseURL、请求头与拦截器。
package gemini

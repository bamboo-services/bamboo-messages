// Package completions 实现面向 OpenAI Chat Completions 协议的 Provider 适配器。
//
// 本包通过 [provider.HTTPClient] 与兼容 OpenAI 的 /chat/completions 端点通信，使用 Bearer Token 认证。
// 适配包括 OpenAI 官方（GPT-4o、o1、o3 等）、智谱 GLM、月之暗面 Kimi、字节豆包等主流兼容服务商。
//
// # 协议特性支持
//
//   - 推理模型与思考：原生支持 o-series 的 reasoning_effort 映射，并支持 Google OpenAI 兼容层通过 extra_content.google.thought_signature 携带的加密签名；
//   - GLM 标签剥离：内置 thinkTagStripper，自动分流流式 content 中混杂的 <think>...</think> 文本到独立 thinking 事件；
//   - Legacy 兼容模式：通过 WithLegacyCompat 自动适配旧版代理端点（如使用 max_tokens 替代 max_completion_tokens）。
//
// # 构造入口
//
//   - [NewCompletionsProvider]：最简构造函数；
//   - [NewCompletionsProviderWithOptions]：完整构造函数，支持配置 BaseURL、自定义头、Legacy 兼容模式与拦截器。
package completions

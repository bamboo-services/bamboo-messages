// Package anthropic 实现面向 Anthropic Messages 协议的原生 Provider 适配器。
//
// 本包通过 [provider.HTTPClient] 与官方 /v1/messages 端点通信，注入 x-api-key 认证头与
// anthropic-version（2023-06-01），完整支持 Claude 3 / 3.5 / 3.7 / 4 系列模型。
//
// # 协议特性支持
//
//   - Prompt Caching：支持在 system 提示词、messages 历史与 tools 列表中显式设置 CacheControl（ephemeral）缓存断点，并透传缓存命中 token 统计；
//   - Thinking 模式：支持 Claude 3.7+ 的扩展思考（extended thinking），通过 ThinkingConfig.Effort 自动映射 budget_tokens；
//   - 工具调用：支持原生 tool_use 与 tool_result 事件，并完整兼容 disable_parallel_tool_use 约束。
//
// # 构造入口
//
//   - [NewProvider]：最简构造函数，接收 apiKey；
//   - [NewProviderWithOptions]：完整构造函数，支持自定义 BaseURL、HTTP 头与请求拦截器注入。
package anthropic

// Package provider 定义 AI 对话协议适配器的核心接口、通用数据模型与通信基座。
//
// 本包是所有下游适配器（Anthropic、OpenAI Completions、OpenAI Responses、Gemini、Bamboo）
// 共同遵循的抽象核心。通过规范化的接口和类型，屏蔽各 AI 提供商在身份认证、端点规范、
// 消息格式、流式帧格式和工具调用模式上的异构差异，保持核心包零外部 SDK 重依赖。
//
// # 核心接口
//
// [Provider] 是协议适配器的核心抽象，约定 6 个方法：
//
//   - [Provider.Chat] 与 [Provider.ChatWithSystem]：流式对话，返回只读事件 channel；
//   - [Provider.Complete] 与 [Provider.CompleteWithSystem]：非流式完整对话，同步等待返回 [*CompletionResult]；
//   - [Provider.GetProviderType]：返回当前提供商类型标识；
//   - [Provider.GetAvailableModels]：返回该提供商支持的已知模型标识列表。
//
// # 消息与流事件模型
//
// 对话数据流采用值类型进行通道传输，传递后不可变：
//
//   - [Message]：统一消息载体，承载角色、文本、多模态块、工具调用（[ToolCall]）、工具返回以及思维链属性（ThinkingContent/ThinkingSignature/ReasoningID）；
//   - [StreamEvent]：流式事件信封，通过 Type 标识阶段（Start、Delta、Stop、Done、Error），携带值类型增量 [StreamDelta]；
//   - [StreamDelta]：泛型增量类型，支持多种具体 Delta 数据（如 [TextData]、[ThinkingData]、[ToolCallData]、[UsageData] 等）。
//
// # 通信与解析基座
//
// 本包提供经工业级验证的统一通信组件：
//
//   - [HTTPClient]：统一管理 HTTP 认证头（Bearer/x-api-key）、版本化 User-Agent（BM-SDK/{version}）、URL 拼接与请求拦截器链；
//   - [SSEScanner]：高效且高容错的 SSE 帧扫描器，内置 JSON 校验与截断断线自动恢复；
//   - [TimingCollector]：零侵入的流式耗时收集器，通过物理时间锚点真实测量 TTFT、思维阶段、正文阶段耗时与输出速率。
//
// # 并发与生命周期
//
// 所有 [Provider] 实例均要求实现为并发安全，可由多个 goroutine 并发发起调用。
// 流式请求由传入的 [context.Context] 驱动，当上下文被取消或产生网络故障时，
// Provider 负责及时关闭流 channel 并清理相关 HTTP 连接与扫描器资源。
package provider

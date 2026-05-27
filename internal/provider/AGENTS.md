# Provider Layer — 核心抽象 + 协议适配

## OVERVIEW

`provider/` 定义统一的 AI 对话接口和类型系统，`provider/anthropic/`、`provider/openai/completions/`、`provider/openai/responses/` 分别实现具体协议适配。

## ADAPTER TEMPLATE (7 文件结构)

每个适配器 **必须** 严格遵循以下文件分工：

| 文件 | 职责 | 必须实现 |
|------|------|---------|
| `provider.go` | 构造函数 + Options 模式 + config + GetProviderType | `New*Provider(apiKey)`, `New*WithOptions(opts...)`, `Option`, `config`, `applyOptions` |
| `chat.go` | 流式对话实现 | `Chat(ctx, messages, config)`, `ChatWithSystem(ctx, systemPrompt, messages, config)` — 返回 `<-chan StreamEvent` |
| `complete.go` | 非流式对话实现 | `Complete(ctx, messages, config)`, `CompleteWithSystem(ctx, systemPrompt, messages, config)` — 返回 `(*CompletionResult, error)` |
| `stream.go` | SSE → StreamEvent 转换 | 解析底层 SDK 的流式迭代器，映射为统一的 StreamEvent 发送到 channel；OpenAI 适配器需在首个文本增量前合成 BlockStart 事件 |
| `message.go` | 消息类型双向转换 | 协议特定消息格式 ↔ `provider.Message` |
| `models.go` | 模型常量 + GetAvailableModels | 定义该协议支持的模型名称列表 |
| `provider_test.go` | 集成测试 | 测试流式/非流式/带 System 的 4 个核心方法 |

## WHERE TO LOOK

| 任务 | 文件 |
|------|------|
| 新增适配器 | 完整复制任意现有适配器的 7 文件结构，修改包名和 SDK 调用 |
| 修改流式解析 | `*/stream.go` — SSE 事件循环 + StreamEvent 发送 |
| 修改消息转换 | `*/message.go` — toSDK / fromSDK 映射函数 |
| 修改请求构建 | `*/chat.go` + `*/complete.go` — 构建底层 SDK 请求参数 |
| 添加新 Delta 类型 | `provider/stream.go` — 新增 StreamDeltaType 常量 + DeltaData 类型 + 构造函数 |
| 添加 ProviderExtra 参数透传 | `*/chat.go` + `*/complete.go` — 使用 GetExtra* 从 config.ProviderExtra 提取参数设置到 SDK params |
| 理解 UserAgent/版本 | `provider/version.go` | SDKName 常量 + GetUserAgent() + GetSDKVersion() |
| 添加 ThinkingConfig 支持 | `*/chat.go` + `*/complete.go` — 从 config.ThinkingConfig 映射到 Anthropic/OpenAI 各自的推理参数 |

## CONVENTIONS

- **Channel 模式** — 流式通过 `make(chan StreamEvent)` 返回，在 goroutine 中发送，发送完 close
- **WithSystem 实现** — 在 messages 前插入 `{Role: RoleSystem, Content: systemPrompt}`，然后调用对应的 Chat/Complete
- **Options 三件套** — 每个适配器的 `WithAPIKey` / `WithBaseURL` / `WithHeader` 签名完全一致
- **Provider 类型别名** — `type Provider = BaseProvider[SDK.Client]`，嵌入后通过 `.Client` 访问底层 SDK
- **BlockStart 合成** — Anthropic 适配器直接从原生 `content_block_start` 事件提取；OpenAI 适配器（Completions/Responses）没有原生事件，通过 `textBlockStarted *bool` 参数在首个文本/推理增量前自动合成 `NewBlockStartDelta("text")`
- **handleChunk/handleStreamEvent 签名** — OpenAI 适配器的流处理函数新增 `textBlockStarted *bool` 参数用于追踪 BlockStart 状态
- **参数透传** — 适配器 chat.go/complete.go 从 `config.ThinkingConfig` 和 `config.ProviderExtra` 提取参数，使用 `GetExtraFloat64`/`GetExtraInt64`/`GetExtraAny` 等类型安全 helper，不使用裸类型断言
- **ThinkingConfig 映射** — Anthropic: Enabled + BudgetTokens → BetaThinkingParam；OpenAI Completions: ReasoningEffort → Reasoning 参数；OpenAI Responses: ReasoningEffort + Summary → Reasoning 参数
- **统一 UserAgent** — 所有适配器在构造函数中通过 `option.WithHeader("User-Agent", provider.GetUserAgent())` 设置统一 UserAgent，格式为 `BM-SDK/{version}`，版本号通过 `runtime/debug.ReadBuildInfo()` 动态读取

## ANTI-PATTERNS

- **禁止** 在 `stream.go` 中不关闭 channel — 必须在 goroutine 结束时 `close(ch)`
- **禁止** 跳过 `StreamTypeStart` 事件 — 流开始必须先发送 Start 事件
- **禁止** 在 message.go 中遗漏 ToolCalls / ToolCallID 映射 — 完整的双向转换
- **禁止** 在适配器中使用裸类型断言访问 ProviderExtra（如 `extra["key"].(float64)`） — 必须使用 `provider.GetExtraFloat64()` 等 helper

## NOTES

- `anthropic` 使用独立的 `anthropic-sdk-go`，`openai/completions` 和 `openai/responses` 共享 `openai-go/v3` 同一 SDK
- 流式实现中，Anthropic 和 OpenAI 的 SSE 迭代方式不同 — Anthropic 用 `client.Messages.NewStreaming()`，OpenAI 用 `client.Chat.Completions.NewStreaming()` / `client.Responses.NewStreaming()`
- 测试均为集成测试（需要 API Key + 网络），无 mock
- Anthropic `stream.go` 的 `contentBlockStart` 方法在 text block 时发出 `NewBlockStartDelta("text")`（之前返回 nil）
- OpenAI Completions `handleChunk` 和 OpenAI Responses `handleStreamEvent` 签名新增 `textBlockStarted *bool` 参数，调用方需传入
- 新增 `StreamDeltaTypeBlockStart` delta 类型和 `BlockStartData` 数据类型，以及 `NewBlockStartDelta`/`NewBlockStartDeltaWithID` 构造函数
- `version.go` 使用 `sync.Once` 保证 `GetUserAgent()` 并发安全，版本读取失败时回退到 `"dev"`

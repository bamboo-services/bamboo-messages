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
| `stream.go` | SSE → StreamEvent 转换 | 解析底层 SDK 的流式迭代器，映射为统一的 StreamEvent 发送到 channel |
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

## CONVENTIONS

- **Channel 模式** — 流式通过 `make(chan StreamEvent)` 返回，在 goroutine 中发送，发送完 close
- **WithSystem 实现** — 在 messages 前插入 `{Role: RoleSystem, Content: systemPrompt}`，然后调用对应的 Chat/Complete
- **Options 三件套** — 每个适配器的 `WithAPIKey` / `WithBaseURL` / `WithHeader` 签名完全一致
- **Provider 类型别名** — `type Provider = BaseProvider[SDK.Client]`，嵌入后通过 `.Client` 访问底层 SDK

## ANTI-PATTERNS

- **禁止** 在 `stream.go` 中不关闭 channel — 必须在 goroutine 结束时 `close(ch)`
- **禁止** 跳过 `StreamTypeStart` 事件 — 流开始必须先发送 Start 事件
- **禁止** 在 message.go 中遗漏 ToolCalls / ToolCallID 映射 — 完整的双向转换

## NOTES

- `anthropic` 使用独立的 `anthropic-sdk-go`，`openai/completions` 和 `openai/responses` 共享 `openai-go/v3` 同一 SDK
- 流式实现中，Anthropic 和 OpenAI 的 SSE 迭代方式不同 — Anthropic 用 `client.Messages.NewStreaming()`，OpenAI 用 `client.Chat.Completions.NewStreaming()` / `client.Responses.NewStreaming()`
- 测试均为集成测试（需要 API Key + 网络），无 mock

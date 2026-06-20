# bamboo 知识库

## 概述

Bamboo Messages SDK 的公共 API 层（门面层），面向业务开发者提供统一的消息模型、流事件、工具定义和客户端接口。该包零外部 SDK 依赖，仅依赖 Go 标准库和 `provider` 核心抽象层。

## 目录结构

```text
bamboo/
├── bamboo.go        # BambooClient 接口 + client 实现
├── message.go       # BambooMessage + ContentBlock 消息模型
├── response.go      # Response / Usage 非流式响应类型
├── stream.go        # StreamEvent / StreamDelta 流事件模型
├── tool.go          # Tool / ToolInputSchema 工具定义
├── config.go        # RequestConfig + ThinkingConfig + 指针辅助函数
├── option.go        # ClientOption + RequestOption + WithXxx() 配置函数
├── convert.go       # 类型转换 (provider ↔ bamboo) + StreamConverter
├── content.go       # ContentBlock 构造函数 + ToolResultBlock(含 ToolName)
├── errors.go        # BambooError 错误类型
├── codec/           # N-to-N 协议编解码层
├── relay/           # 跨协议中继层 (Relay / RelayStream)
└── *_test.go        # 单元测试 + 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解公共 API 入口 | `bamboo.go` | `BambooClient` 接口定义 `Chat` 和 `Complete` |
| 创建客户端 | `bamboo.go` | `NewClient(p)` 或 `NewClientWithOptions(opts...)` |
| 构建消息 | `message.go` + `content.go` | `NewUserMessage`, `NewTextBlock`, `NewToolResultBlock` 等 |
| 配置请求参数 | `config.go` + `option.go` | `RequestConfig` 结构体 + `WithToolChoice`/`WithResponseFormat`/`WithUserID`/`WithParallelToolCalls`/`WithSystemCacheControl`/`WithPromptCacheKey` 等 |
| 处理流事件 | `stream.go` | `StreamEvent` 结构体 + 事件类型常量 |
| 理解类型转换 | `convert.go` | `messagesToProvider`（含 ThinkingBlock 保留）, `configToProvider`, `resultToResponse`（含 ThinkingBlock 生成）, `StreamConverter.Convert`（使用实际 FinishReason） |
| 理解 ThinkingBlock 处理 | `convert.go` | `messagesToProvider` 保留 ThinkingBlock 到 provider.Message；`resultToResponse` 从 CompletionResult.Thinking 生成 ThinkingBlock |
| 添加 ContentBlock 类型 | `content.go` + `stream.go` | 扩展 `ContentBlockType` 和 `StreamDeltaType` |
| 自定义错误处理 | `errors.go` | `BambooError` 类型 + 错误类型常量 |
| 配置 Prompt Caching | `option.go` + `content.go` | `WithSystemCacheControl()` / `WithPromptCacheKey()` / ContentBlock 的 `CacheControl` 字段 |
| 协议互转 | `relay/` | `relay.Relay()` / `relay.RelayStream()` |
| 外部协议编解码 | `codec/` | Anthropic / OpenAI / Responses / Gemini 格式的请求解析与响应序列化 |

## 约定

- **指针区分未设置/零值** — `Temperature`/`TopP` 等可选字段使用 `*float64`，通过 `PtrFloat64()` 辅助函数设置
- **双 Options 体系** — `ClientOption`（配置客户端）和 `RequestOption`（配置单次请求）完全独立
- **类型化字段优先** — `RequestConfig` 的 `UserID`/`ToolChoice`/`ResponseFormat`/`ParallelToolCalls`/`SystemCacheControl`/`PromptCacheKey`/`Metadata` 等通用参数使用类型化字段，不再通过 ProviderExtra 传递
- **ProviderExtra 兜底** — `WithExtra()` 用于传递任何未覆盖的扩展参数，直接写入 `RequestConfig.ProviderExtra`
- **StreamConverter 防御性设计** — 若 provider 未发送 BlockStart，在首个文本/推理增量时自动合成
- **StreamConverter 检查类型断言** — `StreamConverter.Convert` 中所有 `delta.Data` 类型断言均使用 `data, ok := ...` 模式，断言失败时安全返回 nil 而非 panic
- **StreamConverter 使用实际 FinishReason** — `handleStop` 使用适配器在 `StreamTypeStop` 事件中提供的 `FinishReason`（通过 `mapFinishReason` 映射），而非硬编码 `FinishReasonEndTurn`；适配器未提供时默认 `FinishReasonEndTurn`
- **ThinkingBlock 内容保留** — `messagesToProvider` 将 `ThinkingBlock` 的内容保留到 `provider.Message.ThinkingContent` / `ThinkingSignature` 字段，用于多轮对话中向 provider 回传 thinking block 内容；多个 ThinkingBlock 时拼接内容，保留最后一个签名
- **ThinkingBlock 生成** — `resultToResponse` 中，若 `CompletionResult.Thinking` 非空，会在 Content 最前面生成 `ThinkingBlock`
- **ContentBlock 数组风格** — 一条消息可包含多个不同类型的内容块（文本、图片、工具调用等）
- **工具结果拆分** — `convert.go` 的 `messagesToProvider` 将单条消息的 tool_result 拆分为独立的 `RoleTool` 消息，`ToolResultBlock.ToolName` 会传递到 `provider.Message.ToolName`
- **CacheControl 提升策略** — `messagesToProvider` 中，同一条消息内多个 ContentBlock 的 `CacheControl` 标记会被收集，最后一个提升为 `Message.CacheControl`；多于 1 个时输出 warning 日志；ThinkingBlock 的 CacheControl 也参与提升
- **system 角色降级** — `providerRole` 中 "system" 角色记录警告并降级为 user（应通过 system 参数传递）；其他未知角色同样降级为 user 并记录警告
- **Usage 缓存字段透传** — `Usage` 结构体包含 `CacheCreationInputTokens` / `CacheReadInputTokens`，在 `resultToResponse` 和 `StreamConverter` 中完整透传
- **ToolResultBlock.ToolName** — `ToolResultBlock` 新增 `ToolName` 字段，用于在跨协议转换中保留工具名称信息（Gemini 等协议的 functionResponse 需要）

## 反模式

- **禁止** 在 `convert.go` 中遗漏 ContentBlock 类型处理 — 新增类型必须在 `messagesToProvider` 和 `resultToResponse` 中同步处理
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 在 `bamboo` 包中引入具体 SDK 依赖 — 必须面向 `provider.Provider` 接口编程
- **禁止** 裸类型断言访问 `StreamEvent.Delta` — 应通过事件类型判断后安全断言
- **禁止** 在 `StreamConverter` 中使用裸类型断言 — `delta.Data` 的所有断言必须使用 `data, ok := ...` 模式，断言失败时安全返回
- **禁止** 忽略 ContentBlock 的 `CacheControl` 字段 — 新增 block 类型时必须在 `messagesToProvider` 中处理缓存标记提升（包括 ThinkingBlock）

## 调试路径

1. 消息转换错误 → 检查 `convert.go` 的 `messagesToProvider` 是否正确处理 ContentBlock 类型
2. 流事件类型不匹配 → 检查 `convert.go` 的 `StreamConverter.Convert` 是否正确映射 Delta 类型
3. 配置参数不生效 → 检查 `configToProvider` 是否遗漏了新字段（特别是 `SystemCacheControl` / `PromptCacheKey` / `Metadata`）
4. 工具调用结果丢失 → 检查 `messagesToProvider` 中 tool_result 的拆分逻辑
5. 客户端初始化失败 → 确认 `NewClient` 传入了非 nil 的 provider
6. 协议互转异常 → 先查 `codec/` 对应格式子包的解析/序列化，再查 `relay/` 的调用链
7. Prompt caching 未命中 → 检查 `SystemCacheControl` / ContentBlock `CacheControl` 是否正确设置，查看 Usage 的 `CacheCreationInputTokens` / `CacheReadInputTokens` 是否为 0
8. 请求参数不确定 → 启用 relay 层 `WithDebug(true)` 或环境变量 `BAMBOO_DEBUG=1`
9. ThinkingBlock 内容丢失 → 检查 `messagesToProvider` 是否正确将 ThinkingBlock 内容写入 `provider.Message.ThinkingContent`；检查 `resultToResponse` 是否从 `CompletionResult.Thinking` 生成 ThinkingBlock
10. FinishReason 不正确 → 检查适配器是否在 `StreamTypeStop` 事件中提供了正确的 `FinishReason`；`StreamConverter` 会使用该值，未提供时默认 `FinishReasonEndTurn`
11. system 角色被降级 → "system" 消息角色会记录警告并降级为 user，应通过 `Chat`/`Complete` 的 system 参数传递系统提示

## 引用

- [codec](./codec/AGENTS.md) — N-to-N 协议编解码层
- [relay](./relay/AGENTS.md) — 跨协议中继层

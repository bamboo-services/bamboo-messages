# bamboo 知识库

## 概述

Bamboo Messages SDK 的公共 API 层（门面层），面向业务开发者提供统一的消息模型、流事件、工具定义和客户端接口。该包零外部 SDK 依赖，仅依赖 Go 标准库和 `provider` 核心抽象层。

## 目录结构

```text
bamboo/
├── bamboo.go        # BambooClient 接口 + client 实现 + NewClientWithOptions
├── message.go       # BambooMessage (含 ReasoningID) + ContentBlock 消息模型 + UnmarshalJSON
├── response.go      # Response (含 ResponseID) / Usage 非流式响应类型 + UnmarshalJSON
├── stream.go        # StreamEvent / StreamDelta 流事件模型 + EventPing 常量
├── tool.go          # Tool / ToolInputSchema 工具定义
├── config.go        # RequestConfig + ThinkingConfig + 指针辅助函数
├── option.go        # ClientOption + RequestOption + WithToolChoice/WithResponseFormat/WithUserID/WithProvider/WithDefaultModel + WithXxx() 配置函数
├── convert.go       # 类型转换 (provider ↔ bamboo) + StreamConverter (含 IndexedToolCallDeltaData 支持、优先级 FinishReason、自动 flush)
├── content.go       # ContentBlock 构造函数 + WithCache 变体 + RegisterBlockType + ContentBlocks 反序列化
├── errors.go        # BambooError 错误类型
├── codec/           # N-to-N 协议编解码层
├── relay/           # 跨协议中继层 (Relay / RelayStream)
└── *_test.go        # 单元测试 + 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解公共 API 入口 | `bamboo.go` | `BambooClient` 接口定义 `Chat` 和 `Complete` |
| 创建客户端 | `bamboo.go` | `NewClient(p)` 或 `NewClientWithOptions(opts...)`（WithProvider/WithDefaultModel） |
| 构建消息 | `message.go` + `content.go` | `NewUserMessage`, `NewTextBlock`, `NewToolResultBlock` 等 |
| 带 CacheControl 的 Block | `content.go` | `NewTextBlockWithCache` / `NewToolUseBlockWithCache` 等 6 种 WithCache 变体 |
| 自定义 Block 反序列化 | `content.go` | `RegisterBlockType(ct, factory)` — 注册自定义 ContentBlock 类型 |
| 配置请求参数 | `config.go` + `option.go` | `RequestConfig` 结构体 + `WithToolChoice`/`WithResponseFormat`/`WithUserID`/`WithParallelToolCalls`/`WithSystemCacheControl`/`WithPromptCacheKey` 等 |
| 处理流事件 | `stream.go` | `StreamEvent` 结构体 + 事件类型常量（含 `EventPing`） |
| 理解类型转换 | `convert.go` | `messagesToProvider`, `configToProvider`, `resultToResponse`, `StreamConverter.Convert` |
| 理解优先级 FinishReason | `convert.go` | `recordFinishReason` — tool_use(2) > max_tokens(1) > end_turn(0) 优先级策略 |
| 理解 Error 自动 flush | `convert.go` | `handleError` — 流式错误时自动补发 stop 事件（Vercel AI SDK flush 模式） |
| 理解跨类型 Block 切换 | `convert.go` | `stopForNewBlock` — text→thinking / thinking→text 过渡时自动关闭前一个 block |
| 添加 ContentBlock 类型 | `content.go` + `stream.go` | 扩展 `ContentBlockType` 和 `StreamDeltaType`；通过 `RegisterBlockType` 注册反序列化 |
| 自定义错误处理 | `errors.go` | `BambooError` 类型 + 错误类型常量 |
| 配置 Prompt Caching | `option.go` + `content.go` | `WithSystemCacheControl()` / `WithPromptCacheKey()` / ContentBlock 的 `CacheControl` 字段 / WithCache 构造函数 |
| 协议互转 | `relay/` | `relay.Relay()` / `relay.RelayStream()` |
| 外部协议编解码 | `codec/` | Anthropic / OpenAI / Responses / Gemini 格式的请求解析与响应序列化 |

## 代码地图

### 消息与内容模型

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `MessageRole` | 类型 | message.go | 消息角色类型 (string) |
| `RoleUser` / `RoleAssistant` | 常量 | message.go | `"user"` / `"assistant"` 消息角色 |
| `BambooMessage` | 结构体 | message.go | 上层消息模型 (Role + ContentBlock 数组 + ReasoningID) |
| `BambooMessage.UnmarshalJSON` | 方法 | message.go | 使用 `ContentBlocks` 包装实现类型多态反序列化 |
| `NewUserMessage` | 函数 | message.go | 创建用户文本消息 |
| `NewUserMessageBlocks` | 函数 | message.go | 创建用户多 ContentBlock 消息 |
| `NewAssistantMessage` | 函数 | message.go | 创建助手文本消息 |
| `NewAssistantMessageBlocks` | 函数 | message.go | 创建助手多 ContentBlock 消息 |
| `ContentBlockType` | 类型 | content.go | 内容块类型标识 (string) |
| `ContentBlockText` / `Thinking` / `ToolUse` / `ToolResult` / `Image` / `Document` | 常量 | content.go | 6 种 ContentBlockType 常量 |
| `ContentSource` | 结构体 | content.go | 统一来源类型 (Type + MediaType + Data + URL + Content) |
| `ContentBlock` | 接口 | content.go | 内容块接口 (`BlockType() ContentBlockType`) |
| `TextBlock` | 结构体 | content.go | 文本内容块 |
| `ThinkingBlock` | 结构体 | content.go | 思考过程内容块 (Thinking + Signature) |
| `ToolUseBlock` | 结构体 | content.go | 工具调用内容块 (ID + Name + Input) |
| `ToolResultBlock` | 结构体 | content.go | 工具结果内容块 (ToolUseID + ToolName + Content + IsError + CacheControl) |
| `ImageBlock` | 结构体 | content.go | 图片内容块 (Source: ContentSource) |
| `DocumentBlock` | 结构体 | content.go | 文档内容块 (Source: ContentSource) |
| `ContentBlocks` | 类型 | content.go | `[]ContentBlock` 别名，带自定义 `UnmarshalJSON` 按 `type` 字段分派 |
| `RegisterBlockType` | 函数 | content.go | 注册自定义 ContentBlock 类型用于 JSON 反序列化 |

### 内容块构造函数

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `NewTextBlock` / `NewTextBlockWithCache` | 函数 | content.go | 文本内容块（普通 / 带 CacheControl） |
| `NewThinkingBlock` / `NewThinkingBlockWithCache` | 函数 | content.go | 思考过程内容块（普通 / 带 CacheControl） |
| `NewToolUseBlock` / `NewToolUseBlockWithCache` | 函数 | content.go | 工具调用内容块（普通 / 带 CacheControl） |
| `NewToolUseBlockWithRawInput` | 函数 | content.go | 工具调用内容块（原始 JSON 字符串输入，避免双重编码） |
| `NewToolResultBlock` / `NewToolResultBlockWithCache` | 函数 | content.go | 工具结果内容块（普通 / 带 CacheControl） |
| `NewImageBlock` / `NewImageBlockWithCache` | 函数 | content.go | 图片内容块（普通 / 带 CacheControl） |
| `NewDocumentBlock` / `NewDocumentBlockWithCache` | 函数 | content.go | 文档内容块（普通 / 带 CacheControl） |

### 配置与选项

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `RequestConfig` | 结构体 | config.go | 请求配置 (Model, Temperature, MaxTokens, ThinkingConfig, SystemCacheControl, PromptCacheKey, Metadata, ProviderExtra 等) |
| `ThinkingConfig` | 类型别名 | config.go | = provider.ThinkingConfig |
| `PtrFloat64/PtrBool/PtrInt64` | 函数 | config.go | 指针辅助函数 |
| `ClientOption` | 函数类型 | option.go | 客户端配置选项 func(*clientConfig) |
| `RequestOption` | 函数类型 | option.go | 请求配置选项 func(*RequestConfig) |
| `WithProvider` | 函数 | option.go | ClientOption: 设置底层协议适配器 |
| `WithDefaultModel` | 函数 | option.go | ClientOption: 设置默认模型名称 |
| `NewClientWithOptions` | 函数 | bamboo.go | 通过 Functional Options 创建客户端 |
| `WithToolChoice/WithResponseFormat/WithUserID/WithParallelToolCalls` | 函数 | option.go | 类型化请求配置函数 |
| `WithSystemCacheControl/WithPromptCacheKey` | 函数 | option.go | Prompt Caching 请求配置函数 |
| `WithExtra` | 函数 | option.go | 兜底扩展参数传递 |

### 响应与流事件

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `FinishReason` | 类型 | response.go | 响应完成原因 (string) |
| `FinishReasonEndTurn` / `MaxTokens` / `ToolUse` / `StopSequence` | 常量 | response.go | 4 种完成原因常量 |
| `Response` | 结构体 | response.go | 非流式响应 (Content + StopReason + Usage + ProviderType + RequestID + ResponseID + CreatedAt) |
| `Response.UnmarshalJSON` | 方法 | response.go | 使用 `ContentBlocks` 包装实现类型多态反序列化 |
| `Usage` | 结构体 | response.go | Token 用量 (InputTokens + OutputTokens + CacheCreationInputTokens + CacheReadInputTokens) |
| `StreamEventType` | 类型 | stream.go | 流事件类型标识 (string) |
| `EventMessageStart` / `ContentBlockStart` / `ContentBlockDelta` / `ContentBlockStop` / `MessageDelta` / `MessageStop` / `Ping` / `Error` | 常量 | stream.go | 8 种流事件类型常量 |
| `StreamDeltaType` | 类型 | stream.go | 流增量数据类型标识 (string) |
| `DeltaTextDelta` / `DeltaThinkingDelta` / `DeltaInputJSON` / `DeltaSignature` | 常量 | stream.go | 4 种流增量类型常量 |
| `StreamDelta` | 结构体 | stream.go | 流增量数据 (Type + Text + Thinking + Signature + PartialJSON) |
| `MessageDelta` | 结构体 | stream.go | 消息增量 (StopReason + StopSequence) |
| `StreamEvent` | 结构体 | stream.go | 流事件 (Type + Message + Index + ContentBlock + Delta + Usage + Error) |

### 错误类型

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `BambooError` | 类型别名 | errors.go | = `pkgErrors.BambooError`，Bamboo SDK 统一错误类型 (Category + Message + StatusCode) |
| `NewBambooError` | var 别名 | errors.go | = `pkgErrors.NewBambooError`，签名 `(category, message string, statusCode int) *BambooError` |

### 类型转换

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `StreamConverter` | 结构体 | convert.go | 将 provider.StreamEvent 序列转换为 Anthropic 风格事件序列 |
| `NewStreamConverter` | 构造函数 | convert.go | StreamConverter 实例化 |

## 约定

- **指针区分未设置/零值** — `Temperature`/`TopP` 等可选字段使用 `*float64`，通过 `PtrFloat64()` 辅助函数设置
- **双 Options 体系** — `ClientOption`（配置客户端）和 `RequestOption`（配置单次请求）完全独立
- **类型化字段优先** — `RequestConfig` 的 `UserID`/`ToolChoice`/`ResponseFormat`/`ParallelToolCalls`/`SystemCacheControl`/`PromptCacheKey`/`Metadata` 等通用参数使用类型化字段，不再通过 ProviderExtra 传递
- **ProviderExtra 兜底** — `WithExtra()` 用于传递任何未覆盖的扩展参数，直接写入 `RequestConfig.ProviderExtra`
- **StreamConverter 防御性设计** — 若 provider 未发送 BlockStart，在首个文本/推理增量时自动合成
- **StreamConverter 类型安全** — `StreamConverter.Convert` 中所有 `delta.Data` 类型断言均使用 `data, ok := ...` 模式，断言失败时安全返回 nil 而非 panic
- **优先级 FinishReason** — `recordFinishReason` 使用优先级策略：tool_use(2) > max_tokens(1) > end_turn(0)，防止 stop 覆盖 tool_use
- **Error 自动 flush** — `handleError` 在流式错误时自动补发 stop 事件（Vercel AI SDK flush 模式），确保下游接收到完整的 block 生命周期
- **跨类型 Block 自动切换** — `stopForNewBlock` 在 text→thinking / thinking→text 过渡时自动关闭前一个 block
- **Usage 通过 EventPing relay** — UsageDelta 现在通过 `EventPing` 事件类型输出（不再用 `EventMessageDelta`），避免过早终止语义
- **双键工具 Block 追踪** — `toolBlockByProviderIndex` 和 `toolBlockByID` 支持带索引和不带索引两种工具调用模式
- **ThinkingBlock 内容保留** — `messagesToProvider` 将 `ThinkingBlock` 的内容保留到 `provider.Message.ThinkingContent` / `ThinkingSignature` 字段；`ReasoningID` 保留到 `provider.Message.ReasoningID`
- **ThinkingBlock 生成** — `resultToResponse` 中，若 `CompletionResult.Thinking` 非空，会在 Content 最前面生成 `ThinkingBlock`
- **ContentBlock 数组风格** — 一条消息可包含多个不同类型的内容块（文本、图片、工具调用等）
- **工具结果拆分** — `convert.go` 的 `messagesToProvider` 将单条消息的 tool_result 拆分为独立的 `RoleTool` 消息
- **CacheControl 提升策略** — `messagesToProvider` 中，同一条消息内多个 ContentBlock 的 `CacheControl` 标记会被收集，最后一个提升为 `Message.CacheControl`
- **system 角色降级** — `providerRole` 中 "system" 角色记录警告并降级为 user
- **Usage 缓存字段透传** — `Usage` 结构体包含 `CacheCreationInputTokens` / `CacheReadInputTokens`
- **Block 类型注册表** — `RegisterBlockType` + `ContentBlocks.UnmarshalJSON` 实现 JSON 多态反序列化；所有 6 种标准类型在 `init()` 中自动注册
- **Chat 首事件 peek 模式** — `Chat` 同步 peek 首个 provider 事件，若为 Error（无 Start 前缀）立即返回 `(nil, error)`，防止空流挂起
- **Chat channel 缓冲** — Chat channel 缓冲为 64（吸收短突发流量，Preto.ai 5000+ req/s 生产验证）；终止事件不依赖 buffer 容量，通过超时保障
- **终止事件超时保障** — `terminateWriteTimeout` (5s) 确保终止事件（message_stop 等）写入 out channel 时既不被 `default` 丢弃，也不会因消费端卡死而无限阻塞 goroutine
- **ctx 取消合成错误** — 流式中途 ctx 取消时，发送合成 `xerr.Error` 到 converter 后退出
- **Complete 部分成功** — `Complete` 在 provider 返回 error + 部分结果时返回 `(resp, error)`
- **错误透传简化** — `wrapProviderError` 使用 `errors.As` 提取 `*BambooError`，若已存在则直接透传避免重复包装；否则降级为 `NewBambooError("SDK", err.Error(), 0)`

## 反模式

- **禁止** 在 `convert.go` 中遗漏 ContentBlock 类型处理 — 新增类型必须在 `messagesToProvider` 和 `resultToResponse` 中同步处理
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 在 `bamboo` 包中引入具体 SDK 依赖 — 必须面向 `provider.Provider` 接口编程
- **禁止** 裸类型断言访问 `StreamEvent.Delta` — 应通过事件类型判断后安全断言
- **禁止** 在 `StreamConverter` 中使用裸类型断言 — `delta.Data` 的所有断言必须使用 `data, ok := ...` 模式
- **禁止** 忽略 ContentBlock 的 `CacheControl` 字段 — 新增 block 类型时必须在 `messagesToProvider` 中处理缓存标记提升
- **禁止** 新增 ContentBlock 类型时不注册反序列化 — 必须通过 `RegisterBlockType` 注册

## 调试路径

1. 消息转换错误 → 检查 `convert.go` 的 `messagesToProvider` 是否正确处理 ContentBlock 类型
2. 流事件类型不匹配 → 检查 `convert.go` 的 `StreamConverter.Convert` 是否正确映射 Delta 类型
3. 配置参数不生效 → 检查 `configToProvider` 是否遗漏了新字段
4. 工具调用结果丢失 → 检查 `messagesToProvider` 中 tool_result 的拆分逻辑
5. 客户端初始化失败 → 确认 `NewClient` 传入了非 nil 的 provider；或检查 `NewClientWithOptions` 的 `WithProvider` 是否设置
6. 协议互转异常 → 先查 `codec/` 对应格式子包的解析/序列化，再查 `relay/` 的调用链
7. Prompt caching 未命中 → 检查 `SystemCacheControl` / ContentBlock `CacheControl` 是否正确设置
8. 请求参数不确定 → 启用环境变量 `BAMBOO_DEBUG=1`
9. ThinkingBlock 内容丢失 → 检查 `messagesToProvider` 是否正确将 ThinkingBlock 内容写入 `provider.Message.ThinkingContent`
10. FinishReason 不正确 → 检查 `recordFinishReason` 优先级策略是否被覆盖
11. system 角色被降级 → "system" 消息角色会记录警告并降级为 user，应通过 `Chat`/`Complete` 的 system 参数传递
12. JSON 反序列化 ContentBlock 失败 → 检查自定义类型是否通过 `RegisterBlockType` 注册
13. Chat 空流挂起 → 检查 provider 是否返回了 Error 事件但未发送 Start 前缀（首事件 peek 模式应捕获）

## 引用

- [codec](./codec/AGENTS.md) — N-to-N 协议编解码层
- [relay](./relay/AGENTS.md) — 跨协议中继层

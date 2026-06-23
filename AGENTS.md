# 项目知识库

**生成日期:** 2026-06-23
**提交:** 09b1c3d
**分支:** master

## 概述

Bamboo Messages — AI 对话协议标准化适配层，纯 Go SDK 库。通过统一 `Provider` 接口屏蔽 Anthropic Messages / OpenAI Chat Completions / OpenAI Responses / Google Gemini 等协议差异，上层业务零改动切换 AI 后端。支持 N-to-N 协议互转（通过 codec + relay 层），可对接任意兼容端点。

## 目录结构

```text
bamboo-messages/
├── provider/                       # 核心抽象层（公共包）— 接口 + 通用类型 + 流模型 + Debug
│   ├── provider.go                # Provider 接口 (6 methods) + BaseProvider[T] 泛型基座
│   ├── type.go                    # Message / ChatConfig / ThinkingConfig / Tool / CompletionResult / CacheControl / ContentBlock / ProviderExtra helpers
│   ├── stream.go                  # StreamEvent / StreamDelta[E] + 7 种 Delta 构造函数 (含 NewUsageDeltaWithCache)
│   ├── debug.go                   # Debug 全局开关 + DebugRequest/FormatDebugRequest + 敏感字段脱敏 + 长文本截断
│   ├── version.go                 # SDKName + GetUserAgent() + GetSDKVersion()
│   ├── stream_test.go             # 流模型单元测试
│   ├── type_test.go               # 类型单元测试
│   ├── anthropic/                 # Anthropic Messages 协议适配器
│   │   └── params_audit_test.go   # 参数映射审计测试
│   ├── openai/
│   │   ├── completions/           # OpenAI Chat Completions 协议适配器
│   │   │   ├── message_test.go    # 消息转换单元测试
│   │   │   └── params_audit_test.go # 参数映射审计测试
│   │   └── responses/             # OpenAI Responses 协议适配器
│   │       └── params_audit_test.go # 参数映射审计测试
│   └── gemini/                    # Google Gemini 协议适配器
│       ├── audit_test.go          # 流事件审计测试
│       └── params_audit_test.go   # 参数映射审计测试
│
├── bamboo/                        # 公共 SDK 层 — 面向上层业务的统一 API
│   ├── bamboo.go                  # BambooClient 接口 + Chat/Complete 实现
│   ├── message.go                 # BambooMessage + ContentBlock 消息模型
│   ├── response.go                # Response / Usage 非流式响应类型
│   ├── stream.go                  # StreamEvent / StreamDelta 流事件模型
│   ├── tool.go                    # Tool / ToolInputSchema 工具定义
│   ├── config.go                  # RequestConfig + ThinkingConfig + PtrFloat64/PtrBool/PtrInt64
│   ├── option.go                  # ClientOption + RequestOption + WithToolChoice/WithResponseFormat/WithUserID/WithParallelToolCalls/WithSystemCacheControl/WithPromptCacheKey/WithExtra
│   ├── convert.go                 # 类型转换 (provider ↔ bamboo) + StreamConverter
│   ├── content.go                 # ContentBlock 构造函数
│   ├── errors.go                  # BambooError 错误类型
│   ├── codec/                     # N-to-N 协议编解码层（anthropic/openai/responses/gemini 格式）
│   ├── relay/                     # 跨协议中继层 (Relay / RelayStream + SmoothPacer 平滑缓冲 + Debug)
│   └── *_test.go                  # 单元测试 + 集成测试
│
├── internal/
│   └── xerr/                      # 内部最小错误类型（替代 bamboo-base-go/common/error）
│       └── error.go               # xerr.Error — err + Message
│
├── example/                       # 使用示例
│   └── main.go                    # 完整示例代码
│
├── docs/                          # 设计文档
│   ├── new-api-feasibility.md
│   └── new-api-integration.md
│
└── go.mod                         # Go 1.25, anthropic-sdk-go v1.27, openai-go v3.30, genai v1.60
```

## 导航指南

| 想做什么 | 去哪里 | 备注 |
|----------|--------|------|
| 理解核心接口 | `provider/provider.go` | 6 个方法，Chat/Complete 各有带 System 变体 |
| 理解通用类型 | `provider/type.go` | Message, ChatConfig, Tool, CompletionResult, ThinkingConfig, CacheControl, ContentBlock, ProviderExtra |
| 理解流式模型 | `provider/stream.go` | StreamEvent channel + 6 种 DeltaData 类型 + 含缓存的 UsageData |
| 理解 Debug 机制 | `provider/debug.go` | `DebugEnabled` 全局开关 + `SetDebug()` + `DebugRequest()` + 环境变量 `BAMBOO_DEBUG` |
| 理解 UserAgent/版本 | `provider/version.go` | SDKName + GetUserAgent() + GetSDKVersion() |
| 添加新协议适配器 | 参见 `provider/AGENTS.md` | 结构完全模板化 |
| 理解消息转换 | `*/message.go` | 各适配器的 协议类型 ↔ provider 类型映射 |
| 理解参数构建 | `*/params.go` | 各适配器的 buildParams / buildContentConfig 共享入口 |
| 理解流式解析 | `*/stream.go` | 各适配器的 SSE 事件 → StreamEvent 转换 |
| 查看模型定义 | `*/models.go` | 各协议的模型常量 |
| 运行测试 | `*/provider_test.go` | 每个适配器独立测试 |
| 理解参数透传 | `bamboo/option.go` + `bamboo/convert.go` | ThinkingConfig + ProviderExtra + CacheControl 映射 |
| 理解 BlockStart 事件 | `provider/stream.go` | BlockStartData + 构造函数 |
| 理解 Prompt Caching | `provider/type.go` + `bamboo/option.go` | CacheControl / SystemCacheControl / PromptCacheKey 三层方案 |
| 查看使用示例 | `example/main.go` | 完整示例代码 |
| 理解 N-to-N 协议互转 | `bamboo/codec/` + `bamboo/relay/` | codec 编解码 + relay 中继 |
| 理解流式平滑缓冲 | `bamboo/relay/smooth*.go` | SmoothPacer EMA 自适应 + CJK 切分 + 三阶段模式 |
| 理解内部错误类型 | `internal/xerr/error.go` | 最小错误包装，替代外部依赖 |

## 代码地图

### 核心抽象层 (`provider/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `BaseProvider[T]` | 泛型结构体 | provider.go:14 | 适配器基座，嵌入底层 SDK Client |
| `Provider` | 接口 | provider.go:23 | 6 方法：Chat, ChatWithSystem, Complete, CompleteWithSystem, GetProviderType, GetAvailableModels |
| `Message` | 结构体 | type.go:96 | 统一消息模型 (Role + Content + ContentBlocks + ToolCalls + ToolCallID + ToolName + IsError + CacheControl + ThinkingContent + ThinkingSignature) |
| `ChatConfig` | 结构体 | type.go:221 | 请求配置 (Model, Temperature, MaxTokens, Tools, Metadata, UserID, ToolChoice, ResponseFormat, ParallelToolCalls, ThinkingConfig, SystemCacheControl, PromptCacheKey, ProviderExtra) |
| `ThinkingConfig` | 结构体 | type.go:212 | 思考/推理配置 (Effort: none/low/medium/high) |
| `CacheControl` | 结构体 | type.go:60 | 缓存控制标记 (Type + TTL)，Anthropic prompt caching 使用 |
| `NewEphemeralCacheControl` | 函数 | type.go:68 | 创建 ephemeral CacheControl 标记 |
| `ProviderExtra` | map[string]any | type.go (ChatConfig 字段) | Provider 特有参数透传 |
| `GetExtraFloat64/Int64/String/Bool/Any` | 函数 | type.go:246-310 | ProviderExtra 安全取值 helpers |
| `ContentBlock` | 接口 | type.go:155 | 多媒体内容块接口 (BlockType() string) |
| `ImageContentBlock` | 结构体 | type.go:162 | 图片内容块 (Source: ImageSource) |
| `DocumentContentBlock` | 结构体 | type.go:183 | 文档内容块 (Source: DocumentSource) |
| `StreamEvent` | 结构体 | stream.go:10 | 流事件 (Type + Delta + Err + FinishReason)，值类型，Err 为 `*xerr.Error` |
| `StreamDelta[E]` | 泛型结构体 | stream.go:17 | 流增量 (Type + Data)，泛型确保类型安全 |
| `UsageData` | 结构体 | stream.go:82 | Token 用量（含 CacheCreationInputTokens / CacheReadInputTokens） |
| `CompletionResult` | 结构体 | type.go:80 | 非流式完整响应 (Content + ToolCalls + FinishReason + Usage + Thinking) |
| `BlockStartData` | 结构体 | stream.go:93 | 内容块开始数据 (BlockType + ID + Name) |
| `DebugEnabled` | 变量 | debug.go:17 | Debug 全局开关，通过 `BAMBOO_DEBUG` 环境变量初始化 |
| `SetDebug` | 函数 | debug.go:23 | 全局开启/关闭 Provider 层 debug 日志 |
| `DebugRequest` | 函数 | debug.go:50 | 输出请求 debug 日志（headers 敏感脱敏、body 长文本截断） |
| `FormatDebugRequest` | 函数 | debug.go:67 | 返回格式化 debug 字符串（不受开关限制） |
| `GetUserAgent` | 函数 | version.go | 生成统一 User-Agent 字符串 |
| `GetSDKVersion` | 函数 | version.go | 读取 SDK 版本号 (runtime/debug) |
| `NewBlockStartDelta` / `NewBlockStartDeltaWithID` | 构造函数 | stream.go | BlockStart Delta 工厂函数 |
| `NewUsageDeltaWithCache` | 构造函数 | stream.go:176 | 带缓存统计的 Usage Delta 工厂函数 |
| `NewTextDelta` 等 | 构造函数 | stream.go | 7 种 Delta 工厂函数 |

### 适配器层 (结构完全一致)

| 适配器 | 包路径 | 底层 SDK | Provider 类型名 |
|--------|--------|----------|----------------|
| Anthropic Messages | `provider/anthropic` | `anthropic-sdk-go` | `Provider` |
| OpenAI Completions | `provider/openai/completions` | `openai-go/v3` | `CompletionsProvider` |
| OpenAI Responses | `provider/openai/responses` | `openai-go/v3` | `ResponsesProvider` |
| Google Gemini | `provider/gemini` | `google.golang.org/genai` | `Provider` |

### 公共 SDK 层 (`bamboo/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `BambooClient` | 接口 | bamboo.go | 公共接口：Chat + Complete |
| `BambooMessage` | 结构体 | message.go | 上层消息模型 (Role + ContentBlock 数组) |
| `RequestConfig` | 结构体 | config.go | 请求配置 (Model, Temperature, ThinkingConfig, SystemCacheControl, PromptCacheKey, Metadata, ProviderExtra 等) |
| `ThinkingConfig` | 类型别名 | config.go | = provider.ThinkingConfig |
| `PtrFloat64/PtrBool/PtrInt64` | 函数 | config.go | 指针辅助函数 |
| `ClientOption` | 函数类型 | option.go | 客户端配置选项 func(*clientConfig) |
| `RequestOption` | 函数类型 | option.go | 请求配置选项 func(*RequestConfig) |
| `WithToolChoice/WithResponseFormat/WithUserID/WithParallelToolCalls` | 函数 | option.go | 类型化请求配置函数 |
| `WithSystemCacheControl/WithPromptCacheKey` | 函数 | option.go | Prompt Caching 请求配置函数 |
| `WithExtra` | 函数 | option.go | 兜底扩展参数传递 |
| `ContentBlock` | 接口 | content.go | 内容块接口 (`BlockType() ContentBlockType`) |
| `TextBlock` | 结构体 | content.go | 文本内容块 |
| `ThinkingBlock` | 结构体 | content.go | 思考过程内容块 (Thinking + Signature) |
| `ToolUseBlock` | 结构体 | content.go | 工具调用内容块 (ID + Name + Input) |
| `ToolResultBlock` | 结构体 | content.go | 工具结果内容块 (ToolUseID + ToolName + Content + IsError + CacheControl) |
| `ImageBlock` | 结构体 | content.go | 图片内容块 (Source: ContentSource) |
| `DocumentBlock` | 结构体 | content.go | 文档内容块 (Source: ContentSource) |
| `StreamConverter` | 结构体 | convert.go | 将 provider.StreamEvent 序列转换为 Anthropic 风格事件序列 |
| `NewStreamConverter` | 构造函数 | convert.go | StreamConverter 实例化 |

### 编解码层 (`bamboo/codec/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Codec` | 接口 | codec.go | 协议编解码接口 (Format/ParseRequest/SerializeResponse/SerializeError/NewSerializer) |
| `StreamSerializer` | 接口 | codec.go | 流式序列化器 (Serialize/Flush) |
| `FormatType` | 类型 | codec.go | 协议格式标识 (openai/anthropic/responses/gemini) |
| `RelayRequest` | 结构体 | types.go | 解析后的统一请求中间表示 (Messages/System/Config/IsStream) |
| `Get` | 函数 | registry.go | 根据格式标识查找已注册的 Codec |
| `CodecError` | 结构体 | errors.go | Codec 层统一错误类型 (Type/Message/Cause) |

### 中继层 (`bamboo/relay/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Relay` | 函数 | relay.go | 非流式协议互转 |
| `RelayStream` | 函数 | relay.go | 流式协议互转 |
| `Config` | 结构体 | config.go | relay 运行时配置 (OnUsage/OnError/Debug/Smooth) |
| `Option` | 函数类型 | config.go | Functional Options 配置 |
| `WithUsageCallback` | 函数 | config.go | 设置 Token 用量回调 |
| `WithErrorCallback` | 函数 | config.go | 设置错误回调 |
| `WithDebug` | 函数 | config.go | 启用 relay 层 debug 日志 |
| `WithSmoothBuffer` | 函数 | smooth.go | 启用流式平滑缓冲（预设档位: gentle/smooth/typewriter） |
| `WithSmoothBufferCustom` | 函数 | smooth.go | 启用流式平滑缓冲（自定义参数） |
| `SmoothLevel` | 类型 | smooth.go | 平滑档位标识 (off/gentle/smooth/typewriter/custom) |
| `SmoothParams` | 结构体 | smooth.go | 平滑参数 (TokensPerFrame/MinInterval/MaxInterval/EMAAlpha/DrainTier*) |
| `SmoothConfig` | 结构体 | smooth.go | 平滑配置 (Level + Params) |
| `SmoothPacer` | 结构体 | smooth_pacer.go | 流式平滑缓冲器核心（EMA 自适应 + 三阶段模式） |
| `FrameParser` | 结构体 | smooth_parser.go | SSE 帧解析器（提取 data 行） |
| `TokenSplitter` | 结构体 | smooth_parser.go | CJK/Latin 文本切分器 |
| `FormatRelayInput/FormatRelayParsed` | 函数 | debug.go | 返回格式化 debug 字符串（不受开关限制） |

### 内部工具 (`internal/xerr/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Error` | 结构体 | error.go | 内部最小错误类型 (err + Message)，替代 bamboo-base-go/common/error |
| `NewError` | 函数 | error.go | 兼容原 xError.NewError 签名的构造函数 |

## 模块架构

```text
┌─────────────────────────────────────────────────────┐
│                    上层业务                           │
└──────────┬──────────────────────┬───────────────────┘
           │                      │
           ▼                      ▼
┌──────────────────┐    ┌──────────────────┐
│  bamboo (SDK)    │    │  bamboo/relay    │
│  公共 API 门面   │    │  跨协议中继      │
│  Chat/Complete   │    │  Relay/Stream    │
│                  │    │  + Debug 日志    │
└────────┬─────────┘    └────────┬─────────┘
         │                       │
         │              ┌────────▼─────────┐
         │              │  bamboo/codec    │
         │              │  N-to-N 编解码   │
         │              │  4 种格式子包    │
         │              └────────┬─────────┘
         │                       │
         ▼                       ▼
┌─────────────────────────────────────────┐
│           provider (核心抽象层)          │
│  Provider 接口 + Message + StreamEvent  │
│  + CacheControl + Debug 全局开关         │
└──────────┬──────────────────────────────┘
           │
    ┌──────┼──────┬──────────────┬─────────────┐
    ▼      ▼      ▼              ▼             ▼
┌────────┐┌────────┐┌──────────────┐┌───────────┐
│anthropic││openai/ ││openai/       ││  gemini   │
│ +Cache ││complet.││responses     ││ +params.go│
└────────┘└────────┘└──────────────┘└───────────┘
```

## 约定

1. **值类型传递** — `Message`, `StreamEvent` 为值类型，通过 channel 安全传递，传递后视为只读
2. **Functional Options** — 所有适配器统一使用 `Option func(*config)` 模式，提供 `WithAPIKey`, `WithBaseURL`, `WithHeader`, `WithDebug` 四个选项
3. **双构造函数** — 每个适配器提供 `New*(apiKey)` 最简形式 + `New*WithOptions(opts...)` 完整形式，前者调用后者
4. **参数构建集中化** — 每个适配器通过 `params.go` 的 `buildParams`（Gemini 为 `buildContentConfig`）方法统一构建请求参数，Chat 和 Complete 共享同一入口，避免重复逻辑
5. **文件分工固定** — 每个适配器固定文件：`provider.go` / `params.go` / `chat.go` / `complete.go` / `stream.go` / `message.go` / `models.go` / `option.go` / `tools.go` / `provider_test.go`
6. **中文注释** — 所有文档注释使用中文，遵循 Go doc 规范
7. **错误透传** — 使用 `internal/xerr.Error` 类型（替代原 bamboo-base-go 的 xError.Error），`StreamEvent.Err` 字段为 `*xerr.Error`，保留完整上下文
8. **泛型基座** — `BaseProvider[T any]` 通过泛型参数嵌入不同 SDK Client
9. **参数透传三层方案** — Layer 1: `ChatConfig` 类型化字段 (UserID/ToolChoice/ResponseFormat/ParallelToolCalls/SystemCacheControl/PromptCacheKey/Metadata)；Layer 2: Provider 包独立的 Options 体系 (AnthropicMessagesOption/OpenaiCompletionsOption/OpenaiResponsesOption)；Layer 3: `WithExtra()` 兜底传递任意 key-value
10. **BlockStart 事件** — 所有适配器在首个文本增量前必须发出 BlockStart delta；Anthropic 原生支持，OpenAI/Gemini 适配器通过 textBlockStarted/thinkingBlockStarted 参数合成
11. **ProviderExtra 取值** — 适配器中使用 GetExtra* 类型安全 helper，不做裸类型断言
12. **统一 UserAgent** — 所有适配器在构造函数中通过设置统一 UserAgent，格式为 `BM-SDK/{version}`
13. **Reasoning 内容独立追踪** — OpenAI/Gemini 适配器使用 `textBlockStarted` 和 `thinkingBlockStarted` 两个独立布尔标志追踪不同内容块状态
14. **StreamConverter 防御性自动补发** — 若 Provider 未发送 BlockStart，在首个文本/推理增量时自动合成对应类型的 BlockStart
15. **Legacy 兼容模式** — OpenAI Completions 适配器通过 `legacyCompat` 标志支持旧版端点兼容（max_tokens 旧字段名、条件性 ParallelToolCalls、跳过 ReasoningEffort 映射）
16. **Codec 无状态** — `Codec` 接口无状态可并发使用，有状态操作通过 `NewSerializer()` 创建独立 `StreamSerializer`
17. **Debug 三入口统一** — 环境变量 `BAMBOO_DEBUG=1/true/on` / `provider.SetDebug(true)` / 适配器 `WithDebug()` Option；三者任一启用即生效；relay 层额外提供 `WithDebug(true)` Option 控制单次调用
18. **Debug 脱敏与截断** — 敏感 header（Authorization/X-API-Key/API-Key/X-Goog-API-Key）自动脱敏（保留前 4 后 4）；长文本字段（content/text/system/thinking/reasoning_content/arguments）超过 500 字符自动截断
19. **Prompt Caching 三层方案** — Layer 1: Anthropic 显式 CacheControl 断点（system/messages/tools）；Layer 2: OpenAI PromptCacheKey 路由粘性键；Layer 3: Gemini 通过 ProviderExtra 的 `cached_content` 引用外部资源
20. **Usage 缓存统计透传** — `UsageData` / `Usage` 结构体包含 `CacheCreationInputTokens` / `CacheReadInputTokens`，从 Provider → convert → codec 完整透传
21. **Thinking 内容全链路保留** — `BambooMessage.ThinkingBlock` → `provider.Message.ThinkingContent/ThinkingSignature` → 适配器 → `provider.CompletionResult.Thinking` → `bamboo.Response.ThinkingBlock` 双向透传
22. **FinishReason 流式透传** — `provider.StreamEvent.FinishReason` 由适配器填充，`StreamConverter.handleStop` 使用实际完成原因，不再硬编码 `FinishReasonEndTurn`
23. **ToolName/ToolCallID 分离** — `ToolResultBlock` 和 `provider.Message` 同时保存 `ToolName`（函数名）和 `ToolCallID`（调用 ID），Gemini `FunctionResponse` 需要两者同时存在
24. **StreamConverter 类型安全** — `handleDelta` 中对 `delta.Data` 的断言使用 `ok` 模式，避免自定义 Provider 触发 panic
25. **未知角色降级** — `messagesToProvider` 对 `system` 角色显式 warning 并降级为 `RoleUser`
26. **流式平滑缓冲可选** — relay 层通过 `WithSmoothBuffer(level)` 或 `WithSmoothBufferCustom(params)` 启用；不设置时 `Config.Smooth` 为 nil，走原始流式路径
27. **SmoothPacer 三阶段模式** — NORMAL（EMA 自适应间隔）→ DRAIN（尾部加速阶梯递减）→ FLUSH（错误冲刷立即排空）；上游结束后自动切换到 DRAIN
28. **CJK 文本切分** — `TokenSplitter` 按 rune 切分：CJK 独立 token、Latin 连续合并、标点附着前 token、空格前缀附着后 token；跨帧残余保留在 `pendingTail`
29. **积压感知缩减** — NORMAL 模式下队列积压增长时，有效间隔从 `baseInterval` 线性缩减到 `minIntervalFloor`（2ms），避免过度积压

## 反模式

- **禁止** 直接依赖具体适配器包的业务逻辑 — 必须面向 `provider.Provider` 接口编程
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 在 `provider/` 核心包中引入任何具体 SDK 依赖 — 核心包零外部依赖
- **禁止** 适配器之间互相引用 — 每个适配器独立，零耦合
- **禁止** 裸类型断言访问 ProviderExtra — 必须使用 GetExtra* helpers
- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — OpenAI/Gemini 适配器中两者必须独立追踪
- **禁止** 在 Codec 层直接调用 Provider — Codec 只做格式转换，Provider 调用由 relay 层负责
- **禁止** 在 `chat.go` 和 `complete.go` 中重复构建参数逻辑 — 必须统一调用 `params.go` 的 `buildParams`（Gemini 为 `buildContentConfig`）
- **禁止** StreamConverter 中裸类型断言 `delta.Data` — 必须使用 `ok` 模式安全断言
- **禁止** 在 `messagesToProvider` 中静默丢弃 ThinkingBlock — 必须保留到 `provider.Message.ThinkingContent/ThinkingSignature`

## 独特风格

- `BaseProvider[T]` 泛型基座 + 类型别名 (`type Provider = BaseProvider[anthropic.Client]`) 模式，既统一又保留 SDK 特有能力
- `StreamDelta[E any]` 泛型增量，统一使用时通过 `StreamDelta[any]` + 具体 DeltaData 类型 (TextData / ThinkingData / ToolCallData / ToolCallDeltaData / UsageData) 做类型区分
- 配置可选字段用指针 (`*float64`) 区分"未设置"和"零值"
- `ThinkingConfig` 统一结构体，通过 `Effort` (none/low/medium/high) 适配所有 Provider 的思考/推理模式，各适配器自动映射为 Provider 特有参数
- `ProviderExtra map[string]any` + string key 常量模式，扩展新参数只需添加常量和 WithXxx 函数
- `GetUserAgent()` 动态读取版本号 — 通过 `runtime/debug.ReadBuildInfo()` 在运行时读取，避免硬编码版本
- `StreamConverter` 防御性自动补发 — 若 Provider 未发送 BlockStart，自动合成，兼容不完整的 Provider 实现
- N-to-N Codec 架构 — `codec` 层提供 4 种格式子包，`relay` 层提供函数式互转 API，实现任意协议间的请求-响应转换
- `internal/xerr.Error` 最小错误类型 — 替代外部 bamboo-base-go 依赖，使 SDK 内部错误处理自包含
- Debug 双层实现 — `provider/debug.go`（适配器层，打印请求参数）+ `bamboo/relay/debug.go`（relay 层，打印原始 body 和解析后的 RelayRequest），两者通过同一环境变量 `BAMBOO_DEBUG` 联动
- Prompt Caching 统一抽象 — `CacheControl` 结构体 + `NewEphemeralCacheControl()` 工厂函数，跨 Provider 表达缓存语义；Anthropic 显式断点、OpenAI 路由粘性、Gemini 外部资源引用，三种模型统一为一套 API
- Usage 缓存字段全链路透传 — `UsageData.CacheCreationInputTokens` / `CacheReadInputTokens` 从 Provider 适配器 → `convert.go` → `codec` 序列化，完整传递到上层
- Thinking 内容全链路保留 — `BambooMessage.ThinkingBlock` ↔ `provider.Message.ThinkingContent/ThinkingSignature` ↔ `provider.CompletionResult.Thinking` ↔ `bamboo.Response.ThinkingBlock` 双向透传
- FinishReason 流式透传 — 适配器在 `StreamTypeStop` 事件中填充 `FinishReason`，`StreamConverter` 使用实际停止原因而非硬编码
- N-to-N Codec 架构 — `codec` 层提供 4 种格式子包，`relay` 层提供函数式互转 API，实现任意协议间的请求-响应转换
- 流式平滑缓冲可选 — relay 层通过 `WithSmoothBuffer(level)` 或 `WithSmoothBufferCustom(params)` 启用；不设置时 `Config.Smooth` 为 nil，走原始流式路径
- SmoothPacer 三阶段模式 — NORMAL（EMA 自适应间隔）→ DRAIN（尾部加速阶梯递减）→ FLUSH（错误冲刷立即排空）；上游结束后自动切换到 DRAIN
- CJK 文本切分 — `TokenSplitter` 按 rune 切分：CJK 独立 token、Latin 连续合并、标点附着前 token、空格前缀附着后 token；跨帧残余保留在 `pendingTail`
- 积压感知缩减 — NORMAL 模式下队列积压增长时，有效间隔从 `baseInterval` 线性缩减到 `minIntervalFloor`（2ms），避免过度积压

## 常用命令

```bash
# 测试
go test ./...

# 测试单个适配器
go test ./provider/anthropic/...
go test ./provider/openai/completions/...
go test ./provider/openai/responses/...
go test ./provider/gemini/...
go test ./bamboo/codec/...
go test ./bamboo/relay/...

# 编译检查
go build ./...

# 依赖整理
go mod tidy

# 启用 Debug 日志（任选其一）
BAMBOO_DEBUG=1 go run ./example
BAMBOO_DEBUG=true go test ./provider/anthropic/...
# 或在代码中
#   provider.SetDebug(true)
#   anthropic.WithDebug()
#   relay.WithDebug(true)
```

## 备注

- 所有适配器统一使用 `User-Agent: BM-SDK/{version}`（见 `provider/version.go`），版本通过 `runtime/debug.ReadBuildInfo()` 动态读取
- `docs/` 目录包含设计文档（new-api-feasibility.md / new-api-integration.md）
- 测试需要有效的 API Key，目前测试为 integration test（需要网络）
- `internal/xerr/` 替代了原 `bamboo-base-go/common/error` 依赖，使 SDK 内部错误处理自包含
- OpenAI Completions 适配器的 Legacy 兼容模式可对接旧版第三方代理端点
- bamboo 包的 RequestOption 与 ClientOption 是两个独立的 Functional Options 体系，前者配置请求参数，后者配置客户端
- `example/main.go` 提供了完整的使用示例代码
- 架构从 `internal/provider/` 提升为公共包 `provider/`，上层业务可直接 import 具体适配器
- Debug 日志的敏感字段脱敏列表：`Authorization` / `X-API-Key` / `API-Key` / `X-Goog-API-Key`，脱敏策略为保留前 4 后 4 字符
- Debug 日志的长文本截断字段：`content` / `text` / `system` / `thinking` / `reasoning_content` / `arguments`，截断阈值 `MaxDebugBodyLen` = 500 字符
- Gemini 适配器使用独立的 `params.go` 文件（`buildContentConfig`），与其他适配器的 `buildParams` 命名不同但职责一致
- 新增 11 个 `*_audit_test.go` 文件用于回归审计：codec 请求解析、provider 参数映射、Gemini 流事件等
- `CacheCreationInputTokens` 在跨协议到 OpenAI/Responses/Gemini 时无原生字段，当前按目标协议最佳实践透传或记录限制
- relay 层新增流式平滑缓冲支持：`SmoothPacer` 实现 EMA 自适应间隔 + 三阶段模式（NORMAL/DRAIN/FLUSH）；`TokenSplitter` 支持 CJK/Latin 混合文本切分；预设档位 gentle/smooth/typewriter 可选

## 引用

- [provider](./provider/AGENTS.md) — 核心抽象层知识库
- [bamboo](./bamboo/AGENTS.md) — 公共 SDK 层知识库
- [bamboo/codec](./bamboo/codec/AGENTS.md) — N-to-N 协议编解码层知识库
- [bamboo/relay](./bamboo/relay/AGENTS.md) — 跨协议中继层知识库
- [provider/anthropic](./provider/anthropic/AGENTS.md) — Anthropic Messages 协议适配器知识库
- [provider/openai/completions](./provider/openai/completions/AGENTS.md) — OpenAI Chat Completions 协议适配器知识库
- [provider/openai/responses](./provider/openai/responses/AGENTS.md) — OpenAI Responses 协议适配器知识库
- [provider/gemini](./provider/gemini/AGENTS.md) — Google Gemini 协议适配器知识库

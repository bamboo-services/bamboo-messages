# 项目知识库

**生成日期:** 2026-07-16
**提交:** bcb1833
**分支:** master

## 概述

Bamboo Messages — AI 对话协议标准化适配层，纯 Go SDK 库。通过统一 `Provider` 接口屏蔽 Anthropic Messages / OpenAI Chat Completions / OpenAI Responses / Google Gemini 等协议差异，上层业务零改动切换 AI 后端。支持 N-to-N 协议互转（通过 codec + relay 层），可对接任意兼容端点。

## 目录结构

```text
bamboo-messages/
├── provider/                       # 核心抽象层（公共包）— 接口 + 通用类型 + 流模型 + Debug + 拦截器 + 耗时统计
│   ├── provider.go                # Provider 接口 (6 methods)
│   ├── type.go                    # Message / ChatConfig / ThinkingConfig / Tool / CompletionResult / CacheControl / ContentBlock / ProviderExtra helpers
│   ├── stream.go                  # StreamEvent / StreamDelta[E] + 11 种 Delta 构造函数 (含 NewUsageDeltaWithCache) + IndexedToolCallDeltaData
│   ├── http_client.go             # 统一 HTTP 客户端 HTTPClient + NewHTTPClient（认证/拦截器/User-Agent/URL 拼接）
│   ├── sse_scanner.go             # 共享 SSE 帧解析器 SSEScanner + GLM 截断容错
│   ├── debug.go                   # Debug 全局开关 + DebugRequest/FormatDebugRequest + 敏感字段脱敏 + 长文本截断
│   ├── version.go                 # SDKName + GetUserAgent() + GetSDKVersion()
│   ├── interceptor.go             # RequestInterceptor 函数类型 + ApplyInterceptors 链式执行
│   ├── interceptor_transport.go   # NewInterceptorHTTPClient — HTTP Transport 层拦截器注入
│   ├── options.go                 # 公共 Options 结构体 + Option + WithInterceptor + ApplyOptions
│   ├── timing.go                  # TimingCollector / TimingStats / TokenRates / RateSample — 流式耗时统计与 Token 速率测量
│   ├── stream_test.go             # 流模型单元测试
│   ├── type_test.go               # 类型单元测试
│   ├── http_client_test.go        # HTTPClient 单元测试（Do/DoWithDebug/buildURL/applyHeaders）
│   ├── sse_scanner_test.go        # SSEScanner 单元测试（帧解析/json.Valid 容错/GLM 截断恢复）
│   ├── options_test.go            # 公共 Options 单元测试
│   ├── interceptor_test.go        # 拦截器链单元测试
│   ├── interceptor_transport_test.go # 拦截器 Transport 集成测试
│   ├── timing_test.go             # 耗时收集器单元测试
│   ├── debug_test.go              # Debug 机制单元测试
│   ├── testutil_test.go           # 测试辅助工具
│   ├── anthropic/                 # Anthropic Messages 协议适配器
│   │   ├── types.go               # Anthropic 协议原生请求/响应 DTO（本地定义）
│   │   ├── mock_test.go           # httptest mock server 测试辅助工具
│   │   ├── complete_test.go       # 非流式对话单元测试
│   │   ├── params_audit_test.go   # 参数映射审计测试
│   │   ├── interceptor_test.go    # 拦截器注入测试
│   │   ├── message_test.go        # 消息转换单元测试
│   │   ├── params_test.go         # buildParams 单元测试
│   │   └── stream_test.go         # 流式事件单元测试
│   ├── openai/
│   │   ├── completions/           # OpenAI Chat Completions 协议适配器
│   │   │   ├── types.go           # OpenAI Completions 协议原生请求/响应 DTO（本地定义）
│   │   │   ├── mock_test.go       # httptest mock server 测试辅助工具
│   │   │   ├── message_test.go    # 消息转换单元测试
│   │   │   ├── params_audit_test.go # 参数映射审计测试
│   │   │   ├── legacy_compat_test.go # Legacy 兼容模式测试
│   │   │   └── integration_cross_protocol_test.go # 跨协议集成测试
│   │   └── responses/             # OpenAI Responses 协议适配器
│   │       ├── types.go           # OpenAI Responses 协议原生请求/响应 DTO（本地定义）
│   │       ├── mock_test.go       # httptest mock server 测试辅助工具
│   │       ├── message_audit_test.go # 消息转换审计测试
│   │       ├── params_audit_test.go # 参数映射审计测试
│   │       └── stream_test.go     # 流式事件单元测试
│   ├── gemini/                    # Google Gemini 协议适配器
│   │   ├── types.go               # Gemini 协议原生请求/响应 DTO（本地定义）
│   │   ├── mock_test.go           # httptest mock server 测试辅助工具
│   │   ├── audit_test.go          # 流事件审计测试
│   │   ├── params_audit_test.go   # 参数映射审计测试
│   │   └── stream_test.go         # 流式事件单元测试
│   └── bamboo/                    # bamboo 原生协议适配器
│       ├── provider.go            # Provider 构造函数 + Options 模式
│       ├── complete.go            # 非流式对话
│       ├── chat.go                # 流式对话
│       ├── stream.go              # SSE 事件 → StreamEvent 转换
│       ├── message.go             # 消息格式转换
│       ├── params.go              # 共享参数构建
│       ├── models.go              # 模型列表
│       ├── types.go               # 本地 wire DTO
│       ├── provider_test.go       # 集成测试
│       ├── complete_test.go       # 非流式测试
│       ├── stream_test.go         # 流式测试
│       └── mock_test.go           # httptest 测试辅助
│
├── bamboo/                        # 公共 SDK 层 — 面向上层业务的统一 API
│   ├── bamboo.go                  # BambooClient 接口 + Chat/Complete 实现 + NewClientWithOptions
│   ├── message.go                 # BambooMessage (含 ReasoningID) + ContentBlock 消息模型 + UnmarshalJSON
│   ├── response.go                # Response (含 ResponseID) / Usage 非流式响应类型 + UnmarshalJSON
│   ├── stream.go                  # StreamEvent / StreamDelta 流事件模型 + EventPing
│   ├── tool.go                    # Tool / ToolInputSchema 工具定义
│   ├── config.go                  # RequestConfig + ThinkingConfig + PtrFloat64/PtrBool/PtrInt64
│   ├── option.go                  # ClientOption + RequestOption + WithToolChoice/WithResponseFormat/WithUserID/WithParallelToolCalls/WithProvider/WithDefaultModel/WithSystemCacheControl/WithPromptCacheKey/WithExtra
│   ├── convert.go                 # 类型转换 (provider ↔ bamboo) + StreamConverter (优先级 FinishReason + Error 自动 flush + 双键工具 Block)
│   ├── content.go                 # ContentBlock 构造函数 + WithCache 变体 + RegisterBlockType + ContentBlocks 反序列化
│   ├── errors.go                  # BambooError 类型别名（= pkgErrors.BambooError）+ NewBambooError 变量别名
│   ├── codec/                     # N-to-N 协议编解码层（anthropic/openai/responses/gemini/bamboo 格式）
│   │   └── bamboo/                # bamboo 原生协议编解码（identity transform）
│   │       ├── codec.go           # Codec 实例 + init() 注册到 registry.go
│   │       ├── request.go         # 解析 bamboo 原生请求信封
│   │       ├── response.go        # 序列化为 bamboo 原生响应 JSON
│   │       ├── stream.go          # 流式序列化器
│   │       ├── error.go           # 序列化 bamboo 原生错误响应
│   │       └── *_test.go          # 单元测试
│   ├── relay/                     # 跨协议中继层 (Relay / RelayStream + 纯透传 + Debug)
│   └── *_test.go                  # 单元测试 + 集成测试
│
├── pkg/                            # 通用组件工具包 — 可复用的工具函数和类型
│   ├── option/                     # 通用 Functional Options 模式
│   │   └── option.go               # WithAPIKey/WithBaseURL/WithHeader + ApplyOptions + Getters/Setters
│   ├── helpers/                    # 通用工具函数
│   │   └── helpers.go              # PtrFloat64/PtrBool/PtrInt64/PtrString + GetExtra* 安全取值
│   └── errors/                     # 通用错误类型
│       ├── errors.go               # BambooError 统一错误类型 (Category + Message + StatusCode)
│
├── internal/
│   └── xerr/                      # 内部最小错误类型（替代 bamboo-base-go/common/error）
│       └── error.go               # xerr.Error — err + Message
│
├── develop/                       # 开发期设计文档（本地参考，spec / roadmap / 协议设计）
│   └── docs/
│       ├── bamboo-messages-spec.md
│       ├── message-format.md
│       ├── overview.md
│       ├── provider-interface.md
│       ├── roadmap.md
│       └── stream-design.md
│
├── example/                       # 使用示例
│   └── main.go                    # 完整示例代码
│
├── docs/                          # 设计文档
│   ├── new-api-feasibility.md
│   ├── new-api-integration.md
│   └── glm-stream-interruption-investigation.md # GLM 流式截断中断调研
│
└── go.mod                         # Go 1.25，纯标准库 + 无外部 SDK 依赖
```

## 导航指南

| 想做什么 | 去哪里 | 备注 |
|----------|--------|------|
| 理解核心接口 | `provider/provider.go` | 6 个方法，Chat/Complete 各有带 System 变体 |
| 理解通用类型 | `provider/type.go` | Message, ChatConfig, Tool, CompletionResult, ThinkingConfig, CacheControl, ContentBlock, ProviderExtra |
| 理解流式模型 | `provider/stream.go` | StreamEvent channel + 6 种 DeltaData 类型 + 含缓存的 UsageData + IndexedToolCallDeltaData |
| 理解 Debug 机制 | `provider/debug.go` | `DebugEnabled` 全局开关 + 环境变量 `BAMBOO_DEBUG` 唯一入口 |
| 理解 UserAgent/版本 | `provider/version.go` | SDKName + GetUserAgent() + GetSDKVersion() |
| 理解请求拦截器 | `provider/interceptor.go` + `interceptor_transport.go` | RequestInterceptor + ApplyInterceptors + NewInterceptorHTTPClient |
| 理解公共 Options | `provider/options.go` | Options 结构体 + WithInterceptor + ApplyOptions |
| 理解流式耗时统计 | `provider/timing.go` | TimingCollector + TimingStats + TokenRates + RateSample + 不可靠速率标记 |
| 添加新协议适配器 | 参见 `provider/AGENTS.md` | 结构完全模板化 |
| 理解消息转换 | `*/message.go` | 各适配器的 协议类型 ↔ provider 类型映射 |
| 理解参数构建 | `*/params.go` | 各适配器的 buildParams / buildContentConfig 共享入口 |
| 理解流式解析 | `*/stream.go` | 各适配器的 SSE 事件 → StreamEvent 转换 |
| 查看模型定义 | `*/models.go` | 各协议的模型常量 |
| 运行测试 | `*/provider_test.go` | 每个适配器独立测试 |
| 理解参数透传 | `bamboo/option.go` + `bamboo/convert.go` | ThinkingConfig + ProviderExtra + CacheControl 映射 |
| 理解 BlockStart 事件 | `provider/stream.go` | BlockStartData + 构造函数 |
| 理解 Prompt Caching | `provider/type.go` + `bamboo/option.go` + `bamboo/content.go` | CacheControl / SystemCacheControl / PromptCacheKey / WithCache 构造函数 |
| 查看使用示例 | `example/main.go` | 完整示例代码 |
| 查阅协议设计文档 | `develop/docs/` | spec / roadmap / message-format / stream-design / provider-interface / overview |
| 理解 N-to-N 协议互转 | `bamboo/codec/` + `bamboo/relay/` | codec 编解码 + relay 中继 |
| 理解流式纯透传 | `bamboo/relay/relay.go` | RelayStream 直接透传上游 SSE 帧到输出 channel |
| 理解内部错误类型 | `internal/xerr/error.go` | 最小错误包装，替代外部依赖 |
| 使用通用 Options 模式 | `pkg/option/option.go` | WithAPIKey/WithBaseURL/WithHeader + ApplyOptions |
| 使用通用工具函数 | `pkg/helpers/helpers.go` | PtrFloat64/PtrBool/PtrInt64/PtrString + GetExtra* 安全取值 |
| 使用通用错误类型 | `pkg/errors/errors.go` | BambooError 统一错误类型 |
| 理解 bamboo 原生协议编解码 | `bamboo/codec/bamboo/codec.go` | 恒等变换：直接使用 `bamboo.BambooMessage` / `RequestConfig` / `Response` |
| 理解 bamboo 原生 Provider 适配器 | `provider/bamboo/provider.go` | 面向 bamboo 原生端点（`/v1/bamboo`）的 Provider 实现 |

## 代码地图

### 核心抽象层 (`provider/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Provider` | 接口 | provider.go | 6 方法：Chat, ChatWithSystem, Complete, CompleteWithSystem, GetProviderType, GetAvailableModels |
| `HTTPClient` | 结构体 | http_client.go | 统一 HTTP 通信基座（认证/自定义头/拦截器/User-Agent/URL 拼接） |
| `NewHTTPClient` | 函数 | http_client.go | 创建统一 HTTP 客户端实例 |
| `HTTPClient.Do` | 方法 | http_client.go | 发起 HTTP 请求 `(ctx, method, path, body) → (*http.Response, error)` — 适配器统一入口 |
| `HTTPClient.DoWithDebug` | 方法 | http_client.go | 发起 HTTP 请求并输出 debug 日志（headers 脱敏 + body 截断） |
| `HTTPClient.GetBaseURL` | 方法 | http_client.go | 返回基础 URL 字符串 |
| `SSEScanner` | 结构体 | sse_scanner.go | 共享 SSE 帧解析器，内置 json.Valid 容错与 GLM 截断恢复 |
| `NewSSEScanner` | 函数 | sse_scanner.go | 从 io.ReadCloser 创建 SSE 帧解析器 |
| `Message` | 结构体 | type.go | 统一消息模型 (Role + Content + ContentBlocks + ToolCalls + ToolCallID + ToolName + IsError + CacheControl + ThinkingContent + ThinkingSignature + ReasoningID) |
| `ChatConfig` | 结构体 | type.go | 请求配置 (Model, Temperature, TopP, MaxTokens, Stop, Tools, Metadata, UserID, ToolChoice, ResponseFormat, ParallelToolCalls, ThinkingConfig, SystemCacheControl, PromptCacheKey, ProviderExtra) |
| `ThinkingConfig` | 结构体 | type.go | 思考/推理配置 (Effort: none/low/medium/high) |
| `CacheControl` | 结构体 | type.go | 缓存控制标记 (Type + TTL)，Anthropic prompt caching 使用 |
| `NewEphemeralCacheControl` | 函数 | type.go | 创建 ephemeral CacheControl 标记 |
| `ProviderExtra` | map[string]any | type.go (ChatConfig 字段) | Provider 特有参数透传 |
| `GetExtraFloat64/Int64/String/Bool/Any` | 函数 | type.go | ProviderExtra 安全取值 helpers |
| `ContentBlock` | 接口 | type.go | 多媒体内容块接口 (BlockType() string) |
| `ImageContentBlock` | 结构体 | type.go | 图片内容块 (Source: ImageSource) |
| `DocumentContentBlock` | 结构体 | type.go | 文档内容块 (Source: DocumentSource) |
| `StreamEvent` | 结构体 | stream.go | 流事件 (Type + Delta + Err + FinishReason)，值类型，Err 为 `*xerr.Error` |
| `StreamDelta[E]` | 泛型结构体 | stream.go | 流增量 (Type + Data)，泛型确保类型安全 |
| `UsageData` | 结构体 | stream.go | Token 用量（含 CacheCreationInputTokens / CacheReadInputTokens） |
| `CompletionResult` | 结构体 | type.go | 非流式完整响应 (Content + ToolCalls + FinishReason + Usage + Thinking + ThinkingSignature + ResponseID) |
| `BlockStartData` | 结构体 | stream.go | 内容块开始数据 (BlockType + ID + Name) |
| `IndexedToolCallDeltaData` | 结构体 | stream.go | 带 Provider 原生索引的工具调用参数增量 (PartialJSON + Index + HasIndex) |
| `RequestInterceptor` | 函数类型 | interceptor.go | `func(ctx, body []byte) ([]byte, error)` — 请求拦截器 |
| `ApplyInterceptors` | 函数 | interceptor.go | 按注册顺序链式应用拦截器 |
| `NewInterceptorHTTPClient` | 函数 | interceptor_transport.go | 构造注入拦截器的 `*http.Client`；无拦截器返回 nil |
| `Options` | 结构体 | options.go | 公共运行时选项 (`Interceptors []RequestInterceptor`) |
| `Option` | 函数类型 | options.go | `func(*Options)` — 公共配置选项 |
| `WithInterceptor` | 函数 | options.go | 注册请求拦截器 |
| `ApplyOptions` | 函数 | options.go | 应用公共选项列表 |
| `TimingCollector` | 结构体 | timing.go | 流式请求耗时收集器（零侵入 Observe 模式） |
| `NewTimingCollector` | 函数 | timing.go | 创建耗时收集器实例 |
| `TimingStats` | 结构体 | timing.go | 流式耗时统计 (TotalDuration, FirstByteDuration/TTFT, ThinkingDuration, ContentDuration, ToolDuration, ThinkingTokens, OutputTokens, ToolTokens, TotalTokens, TokenSource) |
| `TokenRates` | 结构体 | timing.go | Token 生成速率 (.2f): ThinkingTokensPerSec, OutputTokensPerSec, ToolTokensPerSec（负值=不可靠） |
| `RateSample` | 结构体 | timing.go | 速率采样点 (ElapsedSec, TokensPerSec, Kind) |
| `RateSampleKind` | 类型 | timing.go | 速率采样类型: `"thinking"` / `"output"` / `"tool"` |
| `DebugEnabled` | 变量 | debug.go | Debug 全局开关，通过环境变量 `BAMBOO_DEBUG=1/true/on` 启用（唯一入口） |
| `DebugRequest` | 函数 | debug.go | 输出请求 debug 日志（headers 敏感脱敏、body 长文本截断） |
| `FormatDebugRequest` | 函数 | debug.go | 返回格式化 debug 字符串（不受开关限制） |
| `GetUserAgent` | 函数 | version.go | 生成统一 User-Agent 字符串 |
| `GetSDKVersion` | 函数 | version.go | 读取 SDK 版本号 (runtime/debug) |
| `NewBlockStartDelta` / `NewBlockStartDeltaWithID` | 构造函数 | stream.go | BlockStart Delta 工厂函数 |
| `NewToolCallDeltaWithIndex` / `NewToolCallDeltaDataWithIndex` | 构造函数 | stream.go | 带索引的工具调用 Delta 工厂函数 |
| `NewUsageDeltaWithCache` | 构造函数 | stream.go | 带缓存统计的 Usage Delta 工厂函数 |
| `NewTextDelta` 等 | 构造函数 | stream.go | 7 种 Delta 工厂函数 |

### 适配器层 (结构完全一致)

| 适配器 | 包路径 | 底层传输 | Provider 类型名 |
|--------|--------|----------|----------------|
| Anthropic Messages | `provider/anthropic` | `*provider.HTTPClient` (x-api-key) | `Provider` |
| OpenAI Completions | `provider/openai/completions` | `*provider.HTTPClient` (Authorization Bearer) | `CompletionsProvider` |
| OpenAI Responses | `provider/openai/responses` | `*provider.HTTPClient` (Authorization Bearer) | `ResponsesProvider` |
| Google Gemini | `provider/gemini` | `*provider.HTTPClient` (x-goog-api-key) | `Provider` |
| bamboo 原生 | `provider/bamboo` | `*provider.HTTPClient` (Authorization Bearer) | `Provider` |

### bamboo 原生适配器 (`provider/bamboo/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Provider` | 结构体 | `provider.go` | 持有 `*provider.HTTPClient` 的 bamboo 原生协议适配器 |
| `Option` | 函数类型 | `provider.go` | `func(*config)` — Provider 配置选项 |
| `NewProvider` | 函数 | `provider.go` | 最简构造函数 |
| `NewProviderWithOptions` | 函数 | `provider.go` | Options 构造函数（APIKey/BaseURL/Headers/Interceptor） |
| `WithAPIKey` / `WithBaseURL` / `WithHeader` / `WithInterceptor` | 函数 | `provider.go` | 配置选项 |
| `GetProviderType` | 方法 | `provider.go` | 返回 `provider.ProviderBamboo` |
| `GetAvailableModels` | 方法 | `models.go` | 返回空列表（开放端点无固定模型白名单） |
| `CompleteWithSystem` | 方法 | `complete.go` | 带系统提示的非流式对话 |
| `ChatWithSystem` | 方法 | `chat.go` | 带系统提示的流式对话 |
| `handleStreamEvent` | 方法 | `stream.go` | SSE 事件分发（message_start/delta/stop/ping/error） |
| `mapBambooFinishReason` | 函数 | `stream.go` | 7 值停止原因映射（end_turn/max_tokens/tool_use/stop_sequence/pause_turn/refusal/server_tool_use） |
| `buildMessages` | 函数 | `message.go` | `provider.Message` → `wireMessage` |
| `buildParams` | 函数 | `params.go` | Chat/Complete 共享参数构建入口 |
| `buildTools` | 函数 | `params.go` | `provider.Tool` → `wireTool` |

### 公共 SDK 层 (`bamboo/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `BambooClient` | 接口 | bamboo.go | 公共接口：Chat + Complete |
| `NewClientWithOptions` | 函数 | bamboo.go | 通过 Functional Options 创建客户端 |
| `WithProvider` | ClientOption | option.go | 设置底层协议适配器 |
| `WithDefaultModel` | ClientOption | option.go | 设置默认模型名称 |
| `BambooMessage` | 结构体 | message.go | 上层消息模型 (Role + ContentBlock 数组 + ReasoningID) |
| `Response` | 结构体 | response.go | 非流式响应 (Content + StopReason + Usage + ProviderType + RequestID + ResponseID + CreatedAt) |
| `RequestConfig` | 结构体 | config.go | 请求配置 (Model, Temperature, ThinkingConfig, SystemCacheControl, PromptCacheKey, Metadata, ProviderExtra 等) |
| `ThinkingConfig` | 类型别名 | config.go | = provider.ThinkingConfig |
| `PtrFloat64/PtrBool/PtrInt64` | 函数 | config.go | 指针辅助函数 |
| `ClientOption` | 函数类型 | option.go | 客户端配置选项 func(*clientConfig) |
| `RequestOption` | 函数类型 | option.go | 请求配置选项 func(*RequestConfig) |
| `WithToolChoice/WithResponseFormat/WithUserID/WithParallelToolCalls` | 函数 | option.go | 类型化请求配置函数 |
| `WithSystemCacheControl/WithPromptCacheKey` | 函数 | option.go | Prompt Caching 请求配置函数 |
| `WithExtra` | 函数 | option.go | 兜底扩展参数传递 |
| `ContentBlock` | 接口 | content.go | 内容块接口 (`BlockType() ContentBlockType`) |
| `TextBlock/ThinkingBlock/ToolUseBlock/ToolResultBlock/ImageBlock/DocumentBlock` | 结构体 | content.go | 6 种内容块实现 |
| `New*Block` / `New*BlockWithCache` | 函数 | content.go | 12 种内容块构造函数（普通 + 带 CacheControl） |
| `RegisterBlockType` | 函数 | content.go | 注册自定义 ContentBlock 类型用于 JSON 反序列化 |
| `ContentBlocks` | 类型 | content.go | `[]ContentBlock` 别名，带自定义 UnmarshalJSON |
| `EventPing` | 常量 | stream.go | `"ping"` — 心跳/Usage relay 事件类型 |
| `StreamConverter` | 结构体 | convert.go | 将 provider.StreamEvent 序列转换为 Anthropic 风格事件序列 |
| `NewStreamConverter` | 构造函数 | convert.go | StreamConverter 实例化 |

### 编解码层 (`bamboo/codec/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Codec` | 接口 | codec.go | 协议编解码接口 (Format/ParseRequest/SerializeResponse/SerializeError/NewSerializer) |
| `StreamSerializer` | 接口 | codec.go | 流式序列化器 (Serialize/Flush) |
| `FormatType` | 类型 | codec.go | 协议格式标识 (openai/anthropic/responses/gemini/bamboo) |
| `RelayRequest` | 结构体 | types.go | 解析后的统一请求中间表示 (Messages/System/Config/IsStream) |
| `Get` | 函数 | registry.go | 根据格式标识查找已注册的 Codec |
| `CodecError` | 结构体 | errors.go | Codec 层统一错误类型 (Type/Message/Cause) |
| `bambooCodec` | 结构体 | `bamboo/codec/bamboo/codec.go` | bamboo 原生协议 Codec 实现（恒等变换） |
| `Codec` | 变量 | `bamboo/codec/bamboo/codec.go` | 全局 `bmcodec.Codec` 实例（包内注册到 `bmcodec.Bamboo`） |
| `parseRequest` | 函数 | `bamboo/codec/bamboo/request.go` | `json.Unmarshal` 原生信封 → `RelayRequest` |
| `serializeResponse` | 函数 | `bamboo/codec/bamboo/response.go` | `json.Marshal(*bamboo.Response)` 恒等输出 |
| `serializeError` | 函数 | `bamboo/codec/bamboo/error.go` | 提取 `*bamboo.BambooError` 字段输出原生错误 JSON |
| `bambooStreamSerializer` | 结构体 | `bamboo/codec/bamboo/stream.go` | 流式 `json.Marshal(StreamEvent)` + Anthropic SSE 帧 |
| `newStreamSerializer` | 函数 | `bamboo/codec/bamboo/stream.go` | 创建新的流式序列化器 |

### 中继层 (`bamboo/relay/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Relay` | 函数 | relay.go | 非流式协议互转（含 SerializeError 错误格式化） |
| `RelayStream` | 函数 | relay.go | 流式协议互转（含速率采样集成） |
| `Config` | 结构体 | config.go | relay 运行时配置 (OnUsage/OnError/EstimateOnMissingUsage) |
| `Option` | 函数类型 | config.go | Functional Options 配置 |
| `WithUsageCallback` | 函数 | config.go | 设置 Token 用量回调 |
| `WithErrorCallback` | 函数 | config.go | 设置错误回调 |
| `FormatRelayInput/FormatRelayParsed` | 函数 | debug.go | 返回格式化 debug 字符串（不受开关限制） |
| `FormatRelayResponse` | 函数 | debug.go | 返回非流式响应的格式化 debug 字符串 |

### 内部工具 (`internal/xerr/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Error` | 结构体 | error.go | 内部最小错误类型 (err + Message)，替代 bamboo-base-go/common/error |
| `NewError` | 函数 | error.go | 兼容原 xError.NewError 签名的构造函数 |

### 通用组件工具包 (`pkg/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Option` | 函数类型 | option/option.go | 通用配置选项 func(*Config) |
| `Config` | 结构体 | option/option.go | 通用 Provider 配置 (APIKey/BaseURL/Headers) |
| `WithAPIKey` | 函数 | option/option.go | 设置 API 密钥 |
| `WithBaseURL` | 函数 | option/option.go | 设置自定义基础 URL |
| `WithHeader` | 函数 | option/option.go | 添加自定义 HTTP 请求头 |
| `ApplyOptions` | 函数 | option/option.go | 将选项列表应用到默认配置 |
| `Config.GetAPIKey/GetBaseURL/GetHeaders` | 方法 | option/option.go | Config Getter 方法 |
| `Config.SetAPIKey/SetBaseURL/SetHeader` | 方法 | option/option.go | Config Setter 方法 |
| `PtrFloat64/PtrBool/PtrInt64/PtrString` | 函数 | helpers/helpers.go | 指针辅助函数 |
| `GetExtraFloat64/Int64/String/Bool/Any` | 函数 | helpers/helpers.go | ProviderExtra 安全取值 helpers |
| `BambooError` | 结构体 | errors/errors.go | Bamboo SDK 统一错误类型 (Category + Message + StatusCode) |
| `NewBambooError` | 函数 | errors/errors.go | 创建 BambooError 实例 `(category, message, statusCode) → *BambooError` |

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
│  + TimingCollector│    │  + Debug 日志    │
│                  │    │  + 速率采样      │
└────────┬─────────┘    └────────┬─────────┘
         │                       │
         │              ┌────────▼─────────┐
         │              │  bamboo/codec    │
         │              │  N-to-N 编解码   │
         │              │  5 种格式子包    │
         │              │  (anthropic/     │
         │              │  openai/         │
         │              │  responses/      │
         │              │  gemini/         │
         │              │  bamboo)         │
         │              └────────┬─────────┘
         │                       │
         ▼                       ▼
┌─────────────────────────────────────────┐
│           provider (核心抽象层)          │
│  Provider 接口 + Message + StreamEvent  │
│  + CacheControl + Debug 全局开关         │
│  + RequestInterceptor 拦截器链           │
│  + TimingCollector 耗时统计              │
└──────────┬──────────────────────────────┘
           │
    ┌──────┼──────┬──────────────┬─────────────┐
     ▼      ▼      ▼              ▼             ▼      ▼
┌────────┐┌────────┐┌──────────────┐┌───────────┐┌─────────┐
│anthropic││openai/ ││openai/       ││  gemini   ││ bamboo  │
│ +Cache ││complet.││responses     ││ +params.go││ +wire   │
│ +Intercept││+Intercept││+Intercept  ││+Intercept ││ +Intercept│
└────────┘└────────┘└──────────────┘└───────────┘└─────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│              pkg (通用组件工具包)         │
│  option/ + helpers/ + errors/           │
│  可复用的工具函数和类型                  │
└─────────────────────────────────────────┘
```

## 约定

1. **值类型传递** — `Message`, `StreamEvent` 为值类型，通过 channel 安全传递，传递后视为只读
2. **Functional Options** — 所有适配器统一使用 `Option func(*config)` 模式，提供 `WithAPIKey`, `WithBaseURL`, `WithHeader`, `WithInterceptor` 四个选项
3. **双构造函数** — 每个适配器提供 `New*(apiKey)` 最简形式 + `New*WithOptions(opts...)` 完整形式，前者调用后者
4. **参数构建集中化** — 每个适配器通过 `params.go` 的 `buildParams`（Gemini 为 `buildContentConfig`）方法统一构建请求参数，Chat 和 Complete 共享同一入口，避免重复逻辑
5. **文件分工固定** — 每个适配器固定文件：`provider.go` / `params.go` / `chat.go` / `complete.go` / `stream.go` / `message.go` / `models.go` / `option.go` / `tools.go` / `types.go` / `provider_test.go`
6. **本地 DTO 与统一 HTTP 传输** — 每个适配器通过 `types.go` 定义协议原生的请求/响应 DTO，通过 `*provider.HTTPClient` 发起请求，流式响应统一使用 `provider.SSEScanner` 解析
7. **中文注释** — 所有文档注释使用中文，遵循 Go doc 规范
7. **错误透传** — 使用 `internal/xerr.Error` 类型（替代原 bamboo-base-go 的 xError.Error），`StreamEvent.Err` 字段为 `*xerr.Error`，保留完整上下文
8. **统一 HTTP 基座** — 所有适配器统一持有 `*provider.HTTPClient` 字段，通过 `provider.NewHTTPClient` 创建，认证头、User-Agent、自定义头、请求拦截器均由其统一处理
9. **参数透传三层方案** — Layer 1: `ChatConfig` 类型化字段 (UserID/ToolChoice/ResponseFormat/ParallelToolCalls/TopP/Stop/SystemCacheControl/PromptCacheKey/Metadata)；Layer 2: Provider 包独立的 Options 体系 + 公共 `provider.Options` (拦截器)；Layer 3: `WithExtra()` 兜底传递任意 key-value
10. **BlockStart 事件** — 所有适配器在首个文本增量前必须发出 BlockStart delta；Anthropic 原生支持，OpenAI/Gemini 适配器通过 textBlockStarted/thinkingBlockStarted 参数合成
11. **ProviderExtra 取值** — 适配器中使用 GetExtra* 类型安全 helper，不做裸类型断言
12. **统一 UserAgent** — 所有适配器通过 `provider.HTTPClient` 自动注入统一 UserAgent，格式为 `BM-SDK/{version}`
13. **Reasoning 内容独立追踪** — OpenAI/Gemini 适配器使用 `textBlockStarted` 和 `thinkingBlockStarted` 两个独立布尔标志追踪不同内容块状态
14. **StreamConverter 防御性自动补发** — 若 Provider 未发送 BlockStart，在首个文本/推理增量时自动合成对应类型的 BlockStart
15. **Legacy 兼容模式** — OpenAI Completions 适配器通过 `legacyCompat` 标志支持旧版端点兼容（max_tokens 旧字段名、条件性 ParallelToolCalls、跳过 ReasoningEffort 映射）
16. **Codec 无状态** — `Codec` 接口无状态可并发使用，有状态操作通过 `NewSerializer()` 创建独立 `StreamSerializer`
17. **Debug 环境变量唯一入口** — 仅通过环境变量 `BAMBOO_DEBUG=1/true/on` 启用；`provider.DebugEnabled` 为全局开关变量；`SetDebug()` 函数和 `WithDebug()` Option 已移除；relay 层 debug 函数直接检查 `provider.DebugEnabled`
18. **Debug 脱敏与截断** — 敏感 header（Authorization/X-API-Key/API-Key/X-Goog-API-Key）自动脱敏（保留前 4 后 4）；长文本字段（content/text/system/thinking/reasoning_content/arguments）超过 500 字符自动截断
19. **Prompt Caching 三层方案** — Layer 1: Anthropic 显式 CacheControl 断点（system/messages/tools）；Layer 2: OpenAI PromptCacheKey 路由粘性键；Layer 3: Gemini 通过 ProviderExtra 的 `cached_content` 引用外部资源
20. **Usage 缓存统计透传** — `UsageData` / `Usage` 结构体包含 `CacheCreationInputTokens` / `CacheReadInputTokens`，从 Provider → convert → codec 完整透传
21. **Thinking 内容全链路保留** — `BambooMessage.ThinkingBlock` → `provider.Message.ThinkingContent/ThinkingSignature/ReasoningID` → 适配器 → `provider.CompletionResult.Thinking/ThinkingSignature/ResponseID` → `bamboo.Response.ThinkingBlock` 双向透传
22. **FinishReason 流式透传** — `provider.StreamEvent.FinishReason` 由适配器填充，`StreamConverter.handleStop` 使用实际完成原因，不再硬编码 `FinishReasonEndTurn`
23. **ToolName/ToolCallID 分离** — `ToolResultBlock` 和 `provider.Message` 同时保存 `ToolName`（函数名）和 `ToolCallID`（调用 ID），Gemini `FunctionResponse` 需要两者同时存在
24. **StreamConverter 类型安全** — `handleDelta` 中对 `delta.Data` 的断言使用 `ok` 模式，避免自定义 Provider 触发 panic
25. **未知角色降级** — `messagesToProvider` 对 `system` 角色显式 warning 并降级为 `RoleUser`
26. **流式纯透传** — relay 层 `RelayStream` 直接将上游 provider 产生的 SSE 帧经 codec 序列化后写入输出 channel，无中间缓冲、无调速、无 token 切分
27. **请求拦截器链** — `RequestInterceptor` 在 HTTP Transport 层对已序列化的请求 body 进行任意修改；nil 拦截器防御性跳过；空切片零开销原样返回
28. **拦截器 Transport 零包装** — `NewInterceptorHTTPClient` 无拦截器时返回 nil，Provider 保留标准库默认 client
29. **公共 Options 嵌入模式** — `provider.Options` 通过匿名嵌入为各 Provider 提供统一的拦截器注册能力
30. **TimingCollector 零侵入** — 用户代码主动创建并调用 `Observe(event)`；非并发安全，单 goroutine 使用
31. **Token 估算规则标准化** — TimingCollector: CJK 1:1, Latin 4:1, Other 2:1
32. **优先级 FinishReason** — `recordFinishReason` 优先级策略：tool_use(2) > max_tokens(1) > end_turn(0)
33. **Error 自动 flush** — `handleError` 在流式错误时自动补发 stop 事件（Vercel AI SDK flush 模式）
34. **跨类型 Block 自动切换** — `stopForNewBlock` 在 text→thinking / thinking→text 过渡时自动关闭前一个 block
35. **Block 类型注册表** — `RegisterBlockType` + `ContentBlocks.UnmarshalJSON` 实现 JSON 多态反序列化
36. **Chat 首事件 peek 模式** — `Chat` 同步 peek 首个 provider 事件，若为 Error 立即返回 `(nil, error)`
37. **Relay 失败返回协议格式错误** — `Relay` 在 provider 失败时调用 `outCodec.SerializeError(err)` 返回协议格式化错误
38. **Adapter 本地 DTO 不暴露外部类型** — 适配器内部使用本地 `types.go` 定义的请求/响应结构，禁止在公共 API 或返回类型中暴露任何外部 SDK 类型
39. **错误透传简化** — `BambooError` 简化为 `Category + Message + StatusCode` 三字段（移除 `Type`/`Code`/`ProviderType`）；`bamboo.go` 的 `wrapProviderError` 使用 `errors.As` 提取 `*BambooError` 直接透传，避免重复包装；非 BambooError 降级为 `NewBambooError("SDK", err.Error(), 0)`
41. **bamboo 原生格式 = 恒等变换 codec + 镜像 DTO provider** — `bamboo/codec/bamboo` 直接复用 `bamboo.BambooMessage` / `RequestConfig` / `Response` 做 identity transform；`provider/bamboo` 使用本地 wire DTO 镜像 facade JSON 形状，不 import 上层 facade 包，避免循环依赖

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
- **禁止** 新增 ContentBlock 类型时不注册反序列化 — 必须通过 `RegisterBlockType` 注册
- **禁止** 将 `ReasoningID` 与 `ThinkingSignature` 混用 — 前者是 OpenAI Responses reasoning item 标识，后者是加密内容

## 独特风格

- `HTTPClient` 统一 HTTP 通信基座 + `SSEScanner` 共享 SSE 帧解析器 — 替代 `BaseProvider[T]` 泛型基座，使所有适配器不依赖具体 SDK，基于 `net/http` + JSON 构建
- `StreamDelta[E any]` 泛型增量，统一使用时通过 `StreamDelta[any]` + 具体 DeltaData 类型 (TextData / ThinkingData / ToolCallData / ToolCallDeltaData / IndexedToolCallDeltaData / UsageData) 做类型区分
- 配置可选字段用指针 (`*float64`) 区分"未设置"和"零值"
- `ThinkingConfig` 统一结构体，通过 `Effort` (none/low/medium/high) 适配所有 Provider 的思考/推理模式，各适配器自动映射为 Provider 特有参数
- `ProviderExtra map[string]any` + string key 常量模式，扩展新参数只需添加常量和 WithXxx 函数
- `GetUserAgent()` 动态读取版本号 — 通过 `runtime/debug.ReadBuildInfo()` 在运行时读取，避免硬编码版本
- `StreamConverter` 防御性自动补发 — 若 Provider 未发送 BlockStart，自动合成，兼容不完整的 Provider 实现
- N-to-N Codec 架构 — `codec` 层提供 5 种格式子包（anthropic / openai / responses / gemini / bamboo），`relay` 层提供函数式互转 API，实现任意协议间的请求-响应转换
- `internal/xerr.Error` 最小错误类型 — 替代外部 bamboo-base-go 依赖，使 SDK 内部错误处理自包含
- Debug 环境变量唯一入口 — `provider/debug.go`（适配器层，打印请求参数）+ `bamboo/relay/debug.go`（relay 层，打印原始 body 和解析后的 RelayRequest），两者通过同一环境变量 `BAMBOO_DEBUG` 联动；RelayStream 流式逐帧 debug 已移除，上层可通过输出 channel 自行捕获
- Prompt Caching 统一抽象 — `CacheControl` 结构体 + `NewEphemeralCacheControl()` 工厂函数，跨 Provider 表达缓存语义
- Usage 缓存字段全链路透传 — `UsageData.CacheCreationInputTokens` / `CacheReadInputTokens` 从 Provider 适配器 → `convert.go` → `codec` 序列化，完整传递到上层
- Thinking 内容全链路保留 — `BambooMessage.ThinkingBlock` ↔ `provider.Message.ThinkingContent/ThinkingSignature/ReasoningID` ↔ `provider.CompletionResult.Thinking/ThinkingSignature/ResponseID` ↔ `bamboo.Response.ThinkingBlock` 双向透传
- FinishReason 流式透传 — 适配器在 `StreamTypeStop` 事件中填充 `FinishReason`，`StreamConverter` 使用实际停止原因而非硬编码；`recordFinishReason` 使用优先级策略防止覆盖
- 流式纯透传 — relay 层 `RelayStream` 直接将上游 provider 产生的 SSE 帧经 codec 序列化后写入输出 channel，无中间缓冲、无调速、无 token 切分
- 请求拦截器链 — `RequestInterceptor` + `ApplyInterceptors` + `interceptorTransport` 构成完整的 HTTP 层请求改写机制，正交扩展点
- TimingCollector 零侵入可观测性 — pull 模式（非 push），用户代码主动创建并在事件循环中调用 `Observe`；4 阶段状态机驱动耗时计算；耗时低于 1ms 的阶段速率用负值标记不可靠
- Block 类型注册表 — `RegisterBlockType` + `ContentBlocks.UnmarshalJSON` 实现 JSON 多态反序列化，6 种标准类型 `init()` 自动注册
- Chat 首事件 peek 模式 — 同步 peek 首个 provider 事件防止空流挂起

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

# 启用 Debug 日志
BAMBOO_DEBUG=1 go run ./example
BAMBOO_DEBUG=true go test ./provider/anthropic/...
```

## 备注

- 所有适配器统一使用 `User-Agent: BM-SDK/{version}`（见 `provider/version.go`），版本通过 `runtime/debug.ReadBuildInfo()` 动态读取
- `docs/` 目录包含设计文档（new-api-feasibility.md / new-api-integration.md）
- `develop/docs/` 目录包含开发期设计参考文档（spec / message-format / overview / provider-interface / roadmap / stream-design），用于协议设计与实现对照
- 测试需要有效的 API Key，目前测试为 integration test（需要网络）
- `internal/xerr/` 替代了原 `bamboo-base-go/common/error` 依赖，使 SDK 内部错误处理自包含
- OpenAI Completions 适配器的 Legacy 兼容模式可对接旧版第三方代理端点
- bamboo 包的 RequestOption 与 ClientOption 是两个独立的 Functional Options 体系，前者配置请求参数，后者配置客户端
- `example/main.go` 提供了完整的使用示例代码
- 架构从 `internal/provider/` 提升为公共包 `provider/`，上层业务可直接 import 具体适配器
- Debug 日志的敏感字段脱敏列表：`Authorization` / `X-API-Key` / `API-Key` / `X-Goog-API-Key`，脱敏策略为保留前 4 后 4 字符
- Debug 日志的长文本截断字段：`content` / `text` / `system` / `thinking` / `reasoning_content` / `arguments`，截断阈值 `MaxDebugBodyLen` = 500 字符
- Gemini 适配器使用独立的 `params.go` 文件（`buildContentConfig`），与其他适配器的 `buildParams` 命名不同但职责一致
- `CacheCreationInputTokens` 在跨协议到 OpenAI/Responses/Gemini 时无原生字段，当前按目标协议最佳实践透传或记录限制
- relay 层流式路径为纯透传：`RelayStream` 直接将上游 provider 产生的 SSE 帧经 codec 序列化后写入输出 channel，无中间缓冲、无调速、无 token 切分
- `pkg/` 包提供通用组件工具：`option/` 包含通用 Functional Options 模式，`helpers/` 包含指针辅助函数和 ProviderExtra 安全取值，`errors/` 包含通用错误类型
- 新增适配器时，建议优先使用 `pkg/option` 包的通用配置模式，减少重复代码
- `pkg/helpers` 包的 GetExtra* 函数与 `provider.GetExtra*` 功能相同，但 `pkg/helpers` 包不依赖 `provider` 包，可独立使用
- 请求拦截器（`RequestInterceptor`）为 SDK 用户提供正交的 HTTP 层请求改写能力，无需 fork Provider 实现
- `TimingCollector` 是 pull 模式的可观测性工具，不是 push 模式的配置；用户代码主动创建并在事件循环中调用 `Observe`
- Responses codec 的流式序列化器完全重写为 `responsesStreamSerializer` 状态机，支持 `sequence_number` 自动递增、`response_id` 注入、双轨 reasoning（raw + summary）、`encrypted_content` 透传
- 错误透传简化：`bamboo.go` 的 `wrapProviderError` 使用 `errors.As` 提取 `*BambooError` 直接透传，避免重复包装；非 BambooError 降级为 `NewBambooError("SDK", err.Error(), 0)`；`BambooError` 字段为 `Category` + `Message` + `StatusCode`

## 引用

- [provider](./provider/AGENTS.md) — 核心抽象层知识库
- [bamboo](./bamboo/AGENTS.md) — 公共 SDK 层知识库
- [bamboo/codec](./bamboo/codec/AGENTS.md) — N-to-N 协议编解码层知识库
- [bamboo/relay](./bamboo/relay/AGENTS.md) — 跨协议中继层知识库
- [provider/anthropic](./provider/anthropic/AGENTS.md) — Anthropic Messages 协议适配器知识库
- [provider/openai/completions](./provider/openai/completions/AGENTS.md) — OpenAI Chat Completions 协议适配器知识库
- [provider/openai/responses](./provider/openai/responses/AGENTS.md) — OpenAI Responses 协议适配器知识库
- [provider/gemini](./provider/gemini/AGENTS.md) — Google Gemini 协议适配器知识库
- [provider/bamboo](./provider/bamboo/AGENTS.md) — bamboo 原生协议适配器知识库

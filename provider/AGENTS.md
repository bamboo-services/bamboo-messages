# provider 知识库

## 概述

`provider/` 是 Bamboo Messages 的核心抽象层（公共包），定义统一的 AI 对话接口、通用类型系统、流式事件模型、请求拦截器链和流式耗时统计。所有协议适配器（Anthropic、OpenAI Completions、OpenAI Responses、Gemini）都基于该层构建，确保上层业务零改动切换 AI 后端。

> **架构变更**：此包已从 `internal/provider/` 提升为公共包 `provider/`，使得上层业务可以直接 import 具体适配器。

## 目录结构

```text
provider/
├── provider.go              # Provider 接口 (6 方法)
├── type.go                  # Message / ChatConfig / ThinkingConfig / Tool / CompletionResult / CacheControl / ProviderExtra helpers
├── stream.go                # StreamEvent / StreamDelta[E] + 7 种 Delta 构造函数 (含 NewUsageDeltaWithCache) + IndexedToolCallDeltaData
├── http_client.go           # 统一 HTTP 客户端 HTTPClient + NewHTTPClient（认证/拦截器/User-Agent/URL 拼接）
├── sse_scanner.go           # 共享 SSE 帧解析器 SSEScanner + GLM 截断容错
├── debug.go                 # Debug 全局开关 + DebugRequest / FormatDebugRequest + 敏感字段脱敏 + 长文本截断
├── version.go               # SDKName + GetUserAgent() + GetSDKVersion() (sync.Once 并发安全)
├── interceptor.go           # RequestInterceptor 函数类型 + ApplyInterceptors 链式执行
├── interceptor_transport.go # NewInterceptorHTTPClient — HTTP Transport 层拦截器注入
├── options.go               # 公共 Options 结构体 + Option 函数类型 + WithInterceptor + ApplyOptions
├── timing.go                # TimingCollector / TimingStats / TokenRates / RateSample — 流式耗时统计与 Token 速率测量
├── stream_test.go           # 流模型单元测试
├── type_test.go             # 类型单元测试
├── options_test.go          # 公共 Options 单元测试
├── interceptor_test.go      # 拦截器链单元测试
├── interceptor_transport_test.go # 拦截器 Transport 集成测试
├── http_client_test.go      # HTTPClient 单元测试（Do/DoWith Debug/buildURL/applyHeaders）
├── sse_scanner_test.go      # SSEScanner 单元测试（帧解析/json.Valid 容错/GLM 截断恢复）
├── timing_test.go           # 耗时收集器单元测试
├── debug_test.go            # Debug 机制单元测试
├── testutil_test.go         # 测试辅助工具
├── anthropic/               # Anthropic Messages 协议适配器
├── openai/
│   ├── completions/         # OpenAI Chat Completions 协议适配器
│   └── responses/           # OpenAI Responses 协议适配器
└── gemini/                  # Google Gemini 协议适配器
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解核心接口 | `provider.go` | 6 个方法：Chat, ChatWithSystem, Complete, CompleteWithSystem, GetProviderType, GetAvailableModels |
| 理解通用类型 | `type.go` | Message, ChatConfig, Tool, CompletionResult, ThinkingConfig, CacheControl, ProviderExtra |
| 理解流式模型 | `stream.go` | StreamEvent channel + 7 种 DeltaData 类型 + 11 个构造函数 + IndexedToolCallDeltaData |
| 理解 Debug 机制 | `debug.go` | `DebugEnabled` 全局开关 + 环境变量 `BAMBOO_DEBUG` 唯一入口 |
| 理解版本/UserAgent | `version.go` | `GetUserAgent()` 返回 `"BM-SDK/{version}"`，基于 `runtime/debug.ReadBuildInfo()` |
| 理解请求拦截器 | `interceptor.go` + `interceptor_transport.go` | `RequestInterceptor` 函数类型 + `ApplyInterceptors` 链式执行 + `NewInterceptorHTTPClient` Transport 注入 |
| 理解公共 Options | `options.go` | `Options` 结构体 + `WithInterceptor` + `ApplyOptions` — 各适配器 config 可匿名嵌入 |
| 理解流式耗时统计 | `timing.go` | `TimingCollector` 零侵入 Observe 模式 + `TimingStats` + `TokenRates` + `RateSample` |
| 添加新 Delta 类型 | `stream.go` | 新增 StreamDeltaType 常量 + DeltaData 类型 + 构造函数 |
| 添加 ProviderExtra 键 | `type.go` | 添加常量 + 在适配器中使用 GetExtra* 提取 |
| 添加缓存控制 | `type.go` | `CacheControl` / `NewEphemeralCacheControl()` + Message/Tool/SystemCacheControl 字段 |
| 理解 BlockStart 事件 | `stream.go` | BlockStartData + NewBlockStartDelta / NewBlockStartDeltaWithID |
| 理解 Thinking 内容 | `type.go` | `Message.ThinkingContent` / `Message.ThinkingSignature` / `CompletionResult.Thinking` |
| 理解 FinishReason 流式传递 | `stream.go` | `StreamEvent.FinishReason` — 仅在 `StreamTypeStop` 事件中填充 |
| 理解 ToolName 字段 | `type.go` | `Message.ToolName` — Gemini FunctionResponse 需要函数名与 ToolCallID 分离 |
| 理解带索引工具调用 | `stream.go` | `IndexedToolCallDeltaData` + `NewToolCallDeltaWithIndex` / `NewToolCallDeltaDataWithIndex` — OpenAI 并行工具调用 |
| 理解 ReasoningID | `type.go` | `Message.ReasoningID` — OpenAI Responses reasoning item ID（如 `"rs_xxx"`） |

## 代码地图

### 核心接口与类型

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `Provider` | 接口 | provider.go | 6 方法统一接口（GetProviderType 返回 `ProviderType` 类型） |
| `HTTPClient` | 结构体 | http_client.go | 统一 HTTP 通信基座（认证/自定义头/拦截器/User-Agent/URL 拼接） |
| `NewHTTPClient` | 函数 | http_client.go | 创建统一 HTTP 客户端实例 |
| `HTTPClient.Do` | 方法 | http_client.go | 发起 HTTP 请求 `(ctx, method, path, body) → (*http.Response, error)` — 适配器统一入口 |
| `HTTPClient.DoWithDebug` | 方法 | http_client.go | 发起 HTTP 请求并输出 debug 日志（providerType/endpoint/headers 脱敏/body 截断），debug 开关关闭时等同 `Do` |
| `HTTPClient.GetBaseURL` | 方法 | http_client.go | 返回基础 URL 字符串 |
| `SSEScanner` | 结构体 | sse_scanner.go | 共享 SSE 帧解析器，内置 json.Valid 容错与 GLM 截断恢复 |
| `NewSSEScanner` | 函数 | sse_scanner.go | 从 io.ReadCloser 创建 SSE 帧解析器 |
| `ProviderType` | 类型 | type.go | 协议类型标识 (string) |
| `ProviderAnthropic` / `ProviderOpenAIResponses` / `ProviderOpenAICompletions` | 常量 | type.go | 3 种 ProviderType 常量 |
| `MessageRole` | 类型 | type.go | 消息角色标识 (string) |
| `RoleSystem` / `RoleUser` / `RoleAssistant` / `RoleTool` | 常量 | type.go | 4 种消息角色常量 |
| `FinishReason` | 类型 | type.go | 完成原因标识 (string) |
| `FinishReasonStop` / `FinishReasonLength` / `FinishReasonToolCalls` | 常量 | type.go | 3 种完成原因常量 |
| `Message` | 结构体 | type.go | 统一消息模型 (Role + Content + ContentBlocks + ThinkingContent + ThinkingSignature + ReasoningID + ToolCalls + ToolCallID + ToolName + IsError + CacheControl) |
| `ChatConfig` | 结构体 | type.go | 请求配置 (Model, Temperature, TopP, MaxTokens, Stop, Tools, Metadata, UserID, ToolChoice, ResponseFormat, ParallelToolCalls, ThinkingConfig, SystemCacheControl, PromptCacheKey, ProviderExtra) |
| `CompletionResult` | 结构体 | type.go | 非流式完整响应 (Content + Thinking + ThinkingSignature + ResponseID + ToolCalls + FinishReason + Usage) |
| `ToolCall` | 结构体 | type.go | 工具调用 (ID + Type + Function) |
| `FunctionCall` | 结构体 | type.go | 函数调用详情 (Name + Arguments) |
| `Tool` | 结构体 | type.go | 工具定义 (Type + Function + CacheControl) |
| `FunctionDef` | 结构体 | type.go | 函数定义 (Name + Description + Parameters) |
| `ThinkingConfig` | 结构体 | type.go | 思考/推理配置 (Effort: none/low/medium/high) |
| `CacheControl` | 结构体 | type.go | 缓存控制标记 (Type + TTL)，用于 Anthropic prompt caching |
| `CacheControlEphemeralTTL` | 类型 | type.go | TTL 类型 (`CacheTTL5m` / `CacheTTL1h`) |
| `NewEphemeralCacheControl` | 函数 | type.go | 创建 ephemeral CacheControl 标记，默认 5m |
| `ProviderExtra` | map[string]any | type.go (ChatConfig 字段) | Provider 特有参数透传 |
| `GetExtraFloat64/Int64/String/Bool/Any` | 函数 | type.go | ProviderExtra 安全取值 helpers |
| `ContentBlock` | 接口 | type.go | 多媒体内容块接口 (BlockType() string) |
| `ImageContentBlock` | 结构体 | type.go | 图片内容块 (Source: ImageSource) |
| `ImageSource` | 结构体 | type.go | 图片来源 (Type + MediaType + Data + URL) |
| `DocumentContentBlock` | 结构体 | type.go | 文档内容块 (Source: DocumentSource) |
| `DocumentSource` | 结构体 | type.go | 文档来源 (Type + MediaType + Data + URL) |

### 流式模型

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `StreamEvent` | 结构体 | stream.go | 流事件 (Type + Delta + Err + FinishReason)，值类型，Err 为 `*xerr.Error`，FinishReason 仅在 StreamTypeStop 中填充 |
| `StreamDelta[E]` | 泛型结构体 | stream.go | 流增量 (Type + Data) |
| `StreamType` | 类型 | stream.go | 流事件类型 (string): `StreamTypeStart`/`Stop`/`Done`/`Error`/`Delta` |
| `StreamDeltaType` | 类型 | stream.go | 流增量数据类型 (string): 7 种常量 (TextOutput/Thinking/Signature/ToolCall/ToolCallDelta/Usage/BlockStart) |
| `TextData` | 类型 | stream.go | 文本数据 (string) |
| `ThinkingData` | 类型 | stream.go | 思考数据 (string) |
| `SignatureData` | 类型 | stream.go | 推理签名/密文数据 (Anthropic signature / OpenAI encrypted_content) |
| `ToolCallData` | 结构体 | stream.go | 工具调用开始数据 (ID + Name + Index + HasIndex) |
| `ToolCallDeltaData` | 类型 | stream.go | 工具调用参数增量 (string) |
| `UsageData` | 结构体 | stream.go | Token 用量（含 CacheCreationInputTokens / CacheReadInputTokens） |
| `BlockStartData` | 结构体 | stream.go | 内容块开始数据 (BlockType + ID + Name) |
| `IndexedToolCallDeltaData` | 结构体 | stream.go | 带 Provider 原生索引的工具调用参数增量 (PartialJSON + Index + HasIndex) — OpenAI 并行工具调用支持 |
| `NewTextDelta` | 构造函数 | stream.go | 文本增量 |
| `NewThinkingDelta` | 构造函数 | stream.go | 思考增量 |
| `NewSignatureDelta` | 构造函数 | stream.go | 签名/密文增量 |
| `NewToolCallDelta` / `NewToolCallDeltaWithIndex` | 构造函数 | stream.go | 工具调用开始（无索引 / 带索引） |
| `NewToolCallDeltaData` / `NewToolCallDeltaDataWithIndex` | 构造函数 | stream.go | 工具调用参数增量（无索引 / 带索引） |
| `NewUsageDelta` / `NewUsageDeltaWithCache` | 构造函数 | stream.go | 用量统计（不含缓存 / 含缓存） |
| `NewBlockStartDelta` / `NewBlockStartDeltaWithID` | 构造函数 | stream.go | 内容块开始（无 ID/Name / 含 ID/Name） |

### 请求拦截器

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `RequestInterceptor` | 函数类型 | interceptor.go | `func(ctx, body []byte) ([]byte, error)` — 对已序列化的上游请求 body 进行任意修改 |
| `ApplyInterceptors` | 函数 | interceptor.go | 按注册顺序依次应用拦截器链，任意一个返回 error 立即停止 |
| `NewInterceptorHTTPClient` | 函数 | interceptor_transport.go | 构造注入拦截器的 `*http.Client`；无拦截器时返回 nil（零包装） |

### 公共 Options 体系

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `Options` | 结构体 | options.go | 公共运行时选项 (`Interceptors []RequestInterceptor`)，各 Provider 私有 config 可匿名嵌入 |
| `Option` | 函数类型 | options.go | `func(*Options)` — 配置公共选项 |
| `WithInterceptor` | 函数 | options.go | 注册一个请求拦截器，多次调用按调用顺序追加 |
| `ApplyOptions` | 函数 | options.go | 将公共 Option 列表应用到默认 Options |

### 流式耗时统计

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `RateSampleKind` | 类型 | timing.go | 速率采样类型: `"thinking"` / `"output"` / `"tool"` |
| `RateSampleKindThinking` | 常量 | timing.go | 思考阶段速率采样 |
| `RateSampleKindOutput` | 常量 | timing.go | 输出阶段速率采样 |
| `RateSampleKindTool` | 常量 | timing.go | 工具调用阶段速率采样 `"tool"` |
| `TimingStats` | 结构体 | timing.go | 流式耗时统计 (TotalDuration, FirstByteDuration/TTFT, ThinkingDuration, ContentDuration, ToolDuration, ToolTokens / TotalTokens / TokenSource) |
| `TokenRates` | 结构体 | timing.go | Token 生成速率 (.2f 精度): ThinkingTokensPerSec, OutputTokensPerSec, ToolTokensPerSec |
| `minReliableDuration` | 常量 | timing.go | 最小可信耗时阈值 (1ms)，低于此值的速率标记为不可靠（负值） |
| `computeRate` | 方法 | timing.go | 计算 token/s 速率，耗时低于阈值时取负标记不可靠 |
| `RateSample` | 结构体 | timing.go | 速率采样点 (ElapsedSec, TokensPerSec, Kind) |
| `TimingCollector` | 结构体 | timing.go | 流式请求耗时收集器（零侵入 Observe 模式） |
| `NewTimingCollector` | 函数 | timing.go | 创建耗时收集器实例 |
| `(*TimingCollector).Observe` | 方法 | timing.go | 观察一个 StreamEvent，更新内部计时状态（驱动 4 阶段状态机） |
| `(*TimingCollector).RecordRateSample` | 方法 | timing.go | 记录速率采样点（由上层业务回调触发） |
| `(*TimingCollector).Stats` | 方法 | timing.go | 返回 TimingStats（取消场景使用 lastEventTime 回退） |
| `(*TimingCollector).Rates` | 方法 | timing.go | 返回 TokenRates（CJK 1 tok/char, Latin 4 chars/tok, Other 2 chars/tok） |
| `(*TimingCollector).RateSeries` | 方法 | timing.go | 返回速率采样序列副本（由上层业务通过 RecordRateSample 填充） |
| `(*TimingCollector).Usage` | 方法 | timing.go | 返回最后收到的 UsageDelta 数据 |

### Debug 与版本

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `DebugEnabled` | 变量 | debug.go | Debug 全局开关，通过环境变量 `BAMBOO_DEBUG=1/true/on` 启用（唯一入口） |
| `MaxDebugBodyLen` | 常量 | debug.go | debug 日志中请求体最大长度 (500) |
| `DebugRequest` | 函数 | debug.go | 输出请求 debug 日志（providerType/endpoint/headers/body） |
| `FormatDebugRequest` | 函数 | debug.go | 返回格式化 debug 字符串（不受开关限制） |
| `SummarizeTools` | 函数 | debug.go | 简化 tools 数组的 debug 输出（供 relay 层复用） |
| `SDKName` | 常量 | version.go | SDK 名称标识 `"BM-SDK"`，User-Agent 前缀 |
| `GetUserAgent` | 函数 | version.go | 生成统一 User-Agent 字符串 `"BM-SDK/{version}"` |
| `GetSDKVersion` | 函数 | version.go | 读取 SDK 版本号 |

## 约定

- **值类型传递** — `Message`, `StreamEvent` 为值类型，通过 channel 安全传递，传递后视为只读
- **统一 HTTP 基座** — `BaseProvider[T]` 已移除，所有适配器统一持有 `*provider.HTTPClient` 字段，通过 `provider.NewHTTPClient` 创建，认证头、User-Agent、自定义头、请求拦截器均由其统一处理
- **Channel 模式** — 流式通过 `make(chan StreamEvent)` 返回，在 goroutine 中发送，发送完 close
- **参数透传三层方案** — Layer 1: `ChatConfig` 类型化字段 (UserID/ToolChoice/ResponseFormat/ParallelToolCalls/TopP/Stop/SystemCacheControl/PromptCacheKey/Metadata)；Layer 2: Provider 包独立的 Options 体系 (AnthropicMessagesOption / OpenaiCompletionsOption / OpenaiResponsesOption) + 公共 `provider.Options` (拦截器)；Layer 3: `WithExtra()` 兜底
- **ProviderExtra 安全取值** — 适配器中使用 GetExtra* 类型安全 helper，不做裸类型断言
- **统一 UserAgent** — 所有适配器通过 `provider.HTTPClient` 自动注入 `"BM-SDK/{version}"` 格式 UserAgent
- **版本读取策略** — `GetSDKVersion()` 优先 `info.Main.Version`，回退到依赖列表查找，最终 `"dev"`
- **sync.Once 并发安全** — `GetUserAgent()` 和 `GetSDKVersion()` 使用 sync.Once 保证只初始化一次
- **ContentBlock 接口** — 多媒体内容块统一实现 `BlockType() string` 方法，`ContentBlocks` 字段优先于 `Content` 字符串字段
- **Thinking 内容双向支持** — `Message.ThinkingContent` / `ThinkingSignature` 用于多轮对话中保留 thinking block（Anthropic extended thinking 验证签名）；`CompletionResult.Thinking` 用于非流式响应中的思考过程内容
- **ReasoningID 独立追踪** — `Message.ReasoningID` 用于 OpenAI Responses API 的 reasoning item 标识（如 `"rs_xxx"`），与 `ThinkingSignature`（加密内容）分离
- **ToolName/ToolCallID 分离** — `Message.ToolName` 存储函数名（Gemini FunctionResponse 需要），`Message.ToolCallID` 存储调用 ID；其他 Provider 通常只需 ToolCallID
- **FinishReason 流式透传** — `StreamEvent.FinishReason` 仅在 `StreamTypeStop` 事件中由适配器填充，标识流结束的具体原因（stop/tool_calls/length/content_filter 等），上层可通过此字段判断流结束状态
- **Debug 环境变量唯一入口** — 仅通过环境变量 `BAMBOO_DEBUG=1/true/on` 启用；`provider.DebugEnabled` 为全局开关变量，所有 debug 函数检查此变量；编程式开关函数和适配器 Debug Option 已移除
- **Debug 敏感字段脱敏** — `Authorization` / `X-API-Key` / `API-Key` / `X-Goog-API-Key` 等敏感 header 自动脱敏（仅保留前 4 和后 4 字符）
- **Debug 长文本截断** — `content` / `text` / `system` / `thinking` / `reasoning_content` / `arguments` 等长文本字段超过 `MaxDebugBodyLen` (500) 时自动截断
- **Prompt Caching 统一抽象** — `CacheControl` 统一结构体表达缓存断点：Anthropic 显式标记、OpenAI 自动缓存（`PromptCacheKey` 仅作路由粘性键）、Gemini 通过 `cached_content` ProviderExtra 引用外部资源
- **拦截器链契约** — `RequestInterceptor` 接收已 marshal 完成的上游请求 body（`[]byte`），返回处理后的 body；nil 拦截器被防御性跳过；nil/空切片零开销原样返回
- **拦截器 Transport 零包装** — `NewInterceptorHTTPClient` 无拦截器时返回 nil，Provider 保留标准库默认 client（避免无谓包装）
- **拦截器 Content-Length 重算** — Transport 修改 body 后自动重算 `Content-Length`
- **公共 Options 嵌入模式** — `provider.Options` 通过匿名嵌入为各 Provider 提供统一的拦截器注册能力；`Interceptors` 字段首字母大写保证嵌入子包后仍可访问
- **统一 SSE 解析** — 所有适配器流式响应使用 `provider.SSEScanner` 解析，内置 json.Valid 校验与 GLM 截断容错
- **TimingCollector 零侵入** — 用户代码主动创建 `TimingCollector` 并在事件循环中调用 `Observe(event)`，不修改 StreamEvent 结构；非并发安全（单 goroutine 使用）
- **TimingCollector 阶段状态机** — 内部 `collectorPhase` (init → thinking → content → tool) 驱动耗时计算，tool 阶段从首个 tool BlockStart/ToolCall 到 Stop
- **Token 估算规则标准化** — CJK 1:1, Latin 4:1, Other 2:1
- **不可靠速率标记** — 阶段耗时低于 `minReliableDuration` (1ms) 时，`Rates()` 返回负值标记不可靠；绝对值为基于阈值估算的参考值（如 -6000 表示不可靠的 6000 tok/s）；零值表示阶段未发生
- **带索引工具调用** — `IndexedToolCallDeltaData` + `ToolCallData.HasIndex/Index` 支持 OpenAI 并行工具调用的原生索引
- **HTTPClient.Do 统一入口** — 所有适配器通过 `httpClient.Do(ctx, method, path, body)` 发起请求，path 为相对路径（如 `/v1/messages`），由 `buildURL` 拼接 BaseURL；debug 模式下改用 `DoWithDebug`，两者共享 URL 拼接和 header 注入逻辑
- **HTTP 错误结构化委托** — `HTTPClient.Do` 仅返回 `*http.Response`，不包装错误；适配器 `complete.go` 负责检查 `resp.StatusCode >= 400` 并使用 `pkgErrors` 的结构化错误包装，使状态码作为结构化字段贯穿错误链路

## 反模式

- **禁止** 在 `provider/` 核心包中引入任何具体 SDK 依赖 — 核心包零外部依赖
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 裸类型断言访问 ProviderExtra — 必须使用 GetExtra* helpers
- **禁止** 在 stream.go 中不关闭 channel — 必须在 goroutine 结束时 close
- **禁止** 适配器之间互相引用 — 每个适配器独立，零耦合
- **禁止** 将 `ThinkingContent` 与 `Content` 混淆 — 前者是 thinking block 的内容，后者是 text block 的内容
- **禁止** 将 `ReasoningID` 与 `ThinkingSignature` 混用 — 前者是 OpenAI Responses reasoning item 标识，后者是加密内容

## 调试路径

1. 接口不匹配 → 检查 `provider.go` 的 Provider 接口定义是否被适配器完整实现（编译期 `var _ provider.Provider = (*Provider)(nil)` 检查）
2. 类型转换错误 → 检查 `type.go` 的类型定义是否与适配器使用一致
3. 流事件丢失 → 检查 `stream.go` 的 StreamEvent 是否正确构造和发送
4. UserAgent 异常 → 检查 `version.go` 的 `ReadBuildInfo()` 是否返回正确信息
5. ProviderExtra 取值失败 → 检查 key 常量是否正确，类型断言是否匹配
6. 请求参数不确定 → 启用 Debug（`BAMBOO_DEBUG=1`），查看实际发送的 headers 和 body
7. Prompt caching 不生效 → 检查 `CacheControl` 标记位置（system / messages / tools）和 TTL 值
8. FinishReason 缺失 → 检查适配器是否在 `StreamTypeStop` 事件中正确填充 `StreamEvent.FinishReason`
9. Thinking 内容丢失 → 检查适配器是否正确提取 thinking/reasoning content 并填充到 `CompletionResult.Thinking` 或 `Message.ThinkingContent`
10. 拦截器不生效 → 检查 `NewInterceptorHTTPClient` 是否返回 nil（空拦截器列表），确认适配器构造函数正确注入
11. 耗时统计不准 → 确认 `TimingCollector.Observe` 在单 goroutine 中调用，检查阶段状态机是否正确切换；速率值为负表示耗时低于 1ms 不可靠
12. 并行工具调用索引丢失 → 检查适配器是否使用 `NewToolCallDeltaWithIndex` / `NewToolCallDeltaDataWithIndex` 而非无索引版本

## 引用

- [anthropic](./anthropic/AGENTS.md) — Anthropic Messages 协议适配器
- [completions](./openai/completions/AGENTS.md) — OpenAI Chat Completions 协议适配器
- [responses](./openai/responses/AGENTS.md) — OpenAI Responses 协议适配器
- [gemini](./gemini/AGENTS.md) — Google Gemini 协议适配器

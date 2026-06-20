# provider 知识库

## 概述

`provider/` 是 Bamboo Messages 的核心抽象层（公共包），定义统一的 AI 对话接口、通用类型系统和流式事件模型。所有协议适配器（Anthropic、OpenAI Completions、OpenAI Responses、Gemini）都基于该层构建，确保上层业务零改动切换 AI 后端。

> **架构变更**：此包已从 `internal/provider/` 提升为公共包 `provider/`，使得上层业务可以直接 import 具体适配器。

## 目录结构

```text
provider/
├── provider.go      # Provider 接口 (6 方法) + BaseProvider[T] 泛型基座
├── type.go          # Message / ChatConfig / ThinkingConfig / Tool / CompletionResult / CacheControl / ProviderExtra helpers
├── stream.go        # StreamEvent / StreamDelta[E] + 7 种 Delta 构造函数 (含 NewUsageDeltaWithCache)
├── debug.go         # Debug 全局开关 + DebugRequest / FormatDebugRequest + 敏感字段脱敏 + 长文本截断
├── version.go       # SDKName + GetUserAgent() + GetSDKVersion() (sync.Once 并发安全)
├── stream_test.go   # 流模型单元测试
├── type_test.go     # 类型单元测试
├── anthropic/       # Anthropic Messages 协议适配器
├── openai/
│   ├── completions/ # OpenAI Chat Completions 协议适配器
│   └── responses/   # OpenAI Responses 协议适配器
└── gemini/          # Google Gemini 协议适配器
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解核心接口 | `provider.go` | 6 个方法：Chat, ChatWithSystem, Complete, CompleteWithSystem, GetProviderType, GetAvailableModels |
| 理解通用类型 | `type.go` | Message, ChatConfig, Tool, CompletionResult, ThinkingConfig, CacheControl, ProviderExtra |
| 理解流式模型 | `stream.go` | StreamEvent channel + 6 种 DeltaData 类型 + 构造函数 |
| 理解 Debug 机制 | `debug.go` | `DebugEnabled` 全局开关 + `SetDebug()` + `DebugRequest()` + 环境变量 `BAMBOO_DEBUG` |
| 理解版本/UserAgent | `version.go` | `GetUserAgent()` 返回 `"BM-SDK/{version}"`，基于 `runtime/debug.ReadBuildInfo()` |
| 添加新 Delta 类型 | `stream.go` | 新增 StreamDeltaType 常量 + DeltaData 类型 + 构造函数 |
| 添加 ProviderExtra 键 | `type.go` | 添加常量 + 在适配器中使用 GetExtra* 提取 |
| 添加缓存控制 | `type.go` | `CacheControl` / `NewEphemeralCacheControl()` + Message/Tool/SystemCacheControl 字段 |
| 理解 BlockStart 事件 | `stream.go` | BlockStartData + NewBlockStartDelta / NewBlockStartDeltaWithID |

## 代码地图

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `BaseProvider[T]` | 泛型结构体 | provider.go:14 | 适配器基座，嵌入底层 SDK Client |
| `Provider` | 接口 | provider.go:23 | 6 方法统一接口（GetProviderType 返回 `ProviderType` 类型） |
| `Message` | 结构体 | type.go:96 | 统一消息模型 (Role + Content + ContentBlocks + ToolCalls + ToolCallID + IsError + CacheControl) |
| `ChatConfig` | 结构体 | type.go:221 | 请求配置 (Model, Temperature, MaxTokens, Tools, Metadata, UserID, ToolChoice, ResponseFormat, ParallelToolCalls, ThinkingConfig, SystemCacheControl, PromptCacheKey, ProviderExtra) |
| `ThinkingConfig` | 结构体 | type.go:212 | 思考/推理配置 (Effort: none/low/medium/high) |
| `CacheControl` | 结构体 | type.go:60 | 缓存控制标记 (Type + TTL)，用于 Anthropic prompt caching |
| `CacheControlEphemeralTTL` | 类型 | type.go:52 | TTL 类型 (`CacheTTL5m` / `CacheTTL1h`) |
| `NewEphemeralCacheControl` | 函数 | type.go:68 | 创建 ephemeral CacheControl 标记，默认 5m |
| `ProviderExtra` | map[string]any | type.go (ChatConfig.ProviderExtra) | Provider 特有参数透传 |
| `GetExtraFloat64/Int64/String/Bool/Any` | 函数 | type.go:246-310 | ProviderExtra 安全取值 helpers |
| `ContentBlock` | 接口 | type.go:155 | 多媒体内容块接口 (BlockType() string) |
| `ImageContentBlock` | 结构体 | type.go:162 | 图片内容块 (Source: ImageSource) |
| `DocumentContentBlock` | 结构体 | type.go:183 | 文档内容块 (Source: DocumentSource) |
| `StreamEvent` | 结构体 | stream.go:10 | 流事件 (Type + Delta + Err)，值类型，Err 为 `*xerr.Error` |
| `StreamDelta[E]` | 泛型结构体 | stream.go:17 | 流增量 (Type + Data) |
| `UsageData` | 结构体 | stream.go:82 | Token 用量（含 CacheCreationInputTokens / CacheReadInputTokens） |
| `BlockStartData` | 结构体 | stream.go:93 | 内容块开始数据 |
| `NewUsageDeltaWithCache` | 构造函数 | stream.go:176 | 带缓存统计的 Usage Delta 工厂函数 |
| `DebugEnabled` | 变量 | debug.go:17 | Debug 全局开关，通过环境变量 `BAMBOO_DEBUG` 初始化 |
| `SetDebug` | 函数 | debug.go:23 | 全局开启/关闭 Provider 层 debug 日志 |
| `DebugRequest` | 函数 | debug.go:50 | 输出请求 debug 日志（providerType/endpoint/headers/body） |
| `FormatDebugRequest` | 函数 | debug.go:67 | 返回格式化 debug 字符串（不受开关限制） |
| `GetUserAgent` | 函数 | version.go | 生成统一 User-Agent 字符串 |
| `GetSDKVersion` | 函数 | version.go | 读取 SDK 版本号 |

## 约定

- **值类型传递** — `Message`, `StreamEvent` 为值类型，通过 channel 安全传递，传递后视为只读
- **泛型基座** — `BaseProvider[T any]` 通过泛型参数嵌入不同 SDK Client，适配器通过类型别名使用（如 `type Provider = BaseProvider[anthropic.Client]`）
- **Channel 模式** — 流式通过 `make(chan StreamEvent)` 返回，在 goroutine 中发送，发送完 close
- **参数透传三层方案** — Layer 1: `ChatConfig` 类型化字段 (UserID/ToolChoice/ResponseFormat/ParallelToolCalls/SystemCacheControl/PromptCacheKey/Metadata)；Layer 2: Provider 包独立的 Options 体系 (AnthropicMessagesOption / OpenaiCompletionsOption / OpenaiResponsesOption)；Layer 3: `WithExtra()` 兜底
- **ProviderExtra 安全取值** — 适配器中使用 GetExtra* 类型安全 helper，不做裸类型断言
- **统一 UserAgent** — 所有适配器通过 `provider.GetUserAgent()` 获取 `"BM-SDK/{version}"` 格式 UserAgent
- **版本读取策略** — `GetSDKVersion()` 优先 `info.Main.Version`，回退到依赖列表查找，最终 `"dev"`
- **sync.Once 并发安全** — `GetUserAgent()` 和 `GetSDKVersion()` 使用 sync.Once 保证只初始化一次
- **ContentBlock 接口** — 多媒体内容块统一实现 `BlockType() string` 方法，`ContentBlocks` 字段优先于 `Content` 字符串字段
- **Debug 三入口** — 环境变量 `BAMBOO_DEBUG=1/true/on` / `provider.SetDebug(true)` / 适配器 `WithDebug()` Option；三者任一启用即生效，适配器在发起请求前调用 `DebugRequest()` 输出实际参数
- **Debug 敏感字段脱敏** — `Authorization` / `X-API-Key` / `API-Key` / `X-Goog-API-Key` 等敏感 header 自动脱敏（仅保留前 4 和后 4 字符）
- **Debug 长文本截断** — `content` / `text` / `system` / `thinking` / `reasoning_content` / `arguments` 等长文本字段超过 `MaxDebugBodyLen` (500) 时自动截断
- **Prompt Caching 统一抽象** — `CacheControl` 统一结构体表达缓存断点：Anthropic 显式标记、OpenAI 自动缓存（`PromptCacheKey` 仅作路由粘性键）、Gemini 通过 `cached_content` ProviderExtra 引用外部资源

## 反模式

- **禁止** 在 `provider/` 核心包中引入任何具体 SDK 依赖 — 核心包零外部依赖
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 裸类型断言访问 ProviderExtra — 必须使用 GetExtra* helpers
- **禁止** 在 stream.go 中不关闭 channel — 必须在 goroutine 结束时 close
- **禁止** 适配器之间互相引用 — 每个适配器独立，零耦合

## 调试路径

1. 接口不匹配 → 检查 `provider.go` 的 Provider 接口定义是否被适配器完整实现（编译期 `var _ provider.Provider = (*Provider)(nil)` 检查）
2. 类型转换错误 → 检查 `type.go` 的类型定义是否与适配器使用一致
3. 流事件丢失 → 检查 `stream.go` 的 StreamEvent 是否正确构造和发送
4. UserAgent 异常 → 检查 `version.go` 的 `ReadBuildInfo()` 是否返回正确信息
5. ProviderExtra 取值失败 → 检查 key 常量是否正确，类型断言是否匹配
6. 请求参数不确定 → 启用 Debug（`BAMBOO_DEBUG=1` 或 `WithDebug()`），查看实际发送的 headers 和 body
7. Prompt caching 不生效 → 检查 `CacheControl` 标记位置（system / messages / tools）和 TTL 值

## 引用

- [anthropic](./anthropic/AGENTS.md) — Anthropic Messages 协议适配器
- [completions](./openai/completions/AGENTS.md) — OpenAI Chat Completions 协议适配器
- [responses](./openai/responses/AGENTS.md) — OpenAI Responses 协议适配器
- [gemini](./gemini/AGENTS.md) — Google Gemini 协议适配器

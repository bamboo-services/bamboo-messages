# PROJECT KNOWLEDGE BASE

**Generated:** 2026-06-02
**Commit:** 7290942
**Branch:** master

## OVERVIEW

Bamboo Messages — AI 对话协议标准化适配层，纯 Go SDK 库。通过统一 `Provider` 接口屏蔽 Anthropic Messages / OpenAI Chat Completions / OpenAI Responses 等协议差异，上层业务零改动切换 AI 后端。

## STRUCTURE

```
bamboo-messages/
├── internal/provider/              # 核心抽象层 — 接口 + 通用类型 + 流模型
│   ├── provider.go                # Provider 接口 (6 methods) + BaseProvider[T] 泛型基座
│   ├── type.go                    # Message / ChatConfig / ThinkingConfig / Tool / CompletionResult / ProviderExtra helpers
│   ├── stream.go                  # StreamEvent / StreamDelta[E] + 7 种 Delta 构造函数
│   ├── version.go                 # SDKName + GetUserAgent() + GetSDKVersion()
│   ├── stream_test.go             # 流模型单元测试
│   └── type_test.go               # 类型单元测试
│
├── internal/provider/anthropic/    # Anthropic Messages 协议适配器
├── internal/provider/openai/completions/  # OpenAI Chat Completions 协议适配器
├── internal/provider/openai/responses/    # OpenAI Responses 协议适配器
│
├── bamboo/                        # 公共 SDK 层 — 面向上层业务的统一 API
│   ├── bamboo.go                 # BambooClient 接口 + Chat/Complete 实现
│   ├── message.go                # BambooMessage + ContentBlock 消息模型
│   ├── response.go               # Response / Usage 非流式响应类型
│   ├── stream.go                 # StreamEvent / StreamDelta 流事件模型
│   ├── tool.go                   # Tool / ToolInputSchema 工具定义
│   ├── config.go                 # RequestConfig + ThinkingConfig + PtrFloat64/PtrBool/PtrInt64
│   ├── option.go                 # ClientOption + RequestOption + WithTopK/WithFrequencyPenalty/WithPresencePenalty/WithSeed/WithToolChoice/WithResponseFormat/WithExtra
│   ├── convert.go                # 类型转换 (provider ↔ bamboo) + StreamConverter
│   ├── content.go                # ContentBlock 构造函数
│   ├── errors.go                 # BambooError 错误类型
│   └── *_test.go                 # 单元测试 + 集成测试
│
├── example/                       # 使用示例
│   └── main.go                   # 完整示例代码
│
├── develop/docs/                  # 设计文档 (overview / interface / message / stream / roadmap)
└── go.mod                         # Go 1.25, anthropic-sdk-go v1.27, openai-go v3.30
```

## WHERE TO LOOK

| 想做什么 | 去哪里 | 备注 |
|----------|--------|------|
| 理解核心接口 | `internal/provider/provider.go` | 6 个方法，Chat/Complete 各有带 System 变体 |
| 理解通用类型 | `internal/provider/type.go` | Message, ChatConfig, Tool, CompletionResult, ThinkingConfig, ProviderExtra |
| 理解流式模型 | `internal/provider/stream.go` | StreamEvent channel + 6 种 DeltaData 类型 |
| 理解 UserAgent/版本 | `internal/provider/version.go` | SDKName + GetUserAgent() + GetSDKVersion() |
| 添加新协议适配器 | 参见 `internal/provider/AGENTS.md` | 结构完全模板化，7 文件固定分工 |
| 理解消息转换 | `*/message.go` | 各适配器的 协议类型 ↔ provider 类型映射 |
| 理解流式解析 | `*/stream.go` | 各适配器的 SSE 事件 → StreamEvent 转换 |
| 查看模型定义 | `*/models.go` | 各协议的模型常量 |
| 运行测试 | `*/provider_test.go` | 每个适配器独立测试 |
| 理解参数透传 | `bamboo/option.go` + `bamboo/convert.go` | ThinkingConfig + ProviderExtra 映射 |
| 理解 BlockStart 事件 | `internal/provider/stream.go` | BlockStartData + 构造函数 |
| 查看使用示例 | `example/main.go` | 完整示例代码 |

## CODE MAP

### 核心抽象层 (`internal/provider/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `BaseProvider[T]` | 泛型结构体 | provider.go:14 | 适配器基座，嵌入底层 SDK Client |
| `Provider` | 接口 | provider.go:23 | 6 方法：Chat, ChatWithSystem, Complete, CompleteWithSystem, GetProviderType, GetAvailableModels |
| `Message` | 结构体 | type.go:73 | 统一消息模型 (Role + Content + ToolCalls + ToolCallID) |
| `ChatConfig` | 结构体 | type.go:141 | 请求配置 (Model, Temperature, MaxTokens, Tools, ThinkingConfig, ProviderExtra 等) |
| `ThinkingConfig` | 结构体 | type.go:130 | 思考/推理配置 (Enabled + BudgetTokens + ReasoningEffort + Summary) |
| `ProviderExtra` | map[string]any | type.go:151 | Provider 特有参数透传 |
| `GetExtraFloat64/Int64/String/Any` | 函数 | type.go:160-205 | ProviderExtra 安全取值 helpers |
| `StreamEvent` | 结构体 | stream.go:10 | 流事件 (Type + Delta + Err)，值类型，跨 goroutine 传递 |
| `StreamDelta[E]` | 泛型结构体 | stream.go:17 | 流增量 (Type + Data)，泛型确保类型安全 |
| `CompletionResult` | 结构体 | type.go:58 | 非流式完整响应 |
| `BlockStartData` | 结构体 | stream.go:90 | 内容块开始数据 (BlockType + ID + Name) |
| `GetUserAgent` | 函数 | version.go:32 | 生成统一 User-Agent 字符串 |
| `GetSDKVersion` | 函数 | version.go:48 | 读取 SDK 版本号 (runtime/debug) |
| `NewBlockStartDelta` / `NewBlockStartDeltaWithID` | 构造函数 | stream.go:173-205 | BlockStart Delta 工厂函数 |
| `NewTextDelta` 等 | 构造函数 | stream.go:100-171 | 7 种 Delta 工厂函数 |

### 适配器层 (结构完全一致)

| 适配器 | 包路径 | 底层 SDK | Provider 类型名 |
|--------|--------|----------|----------------|
| Anthropic Messages | `internal/provider/anthropic` | `anthropic-sdk-go` | `Provider` |
| OpenAI Completions | `internal/provider/openai/completions` | `openai-go/v3` | `CompletionsProvider` |
| OpenAI Responses | `internal/provider/openai/responses` | `openai-go/v3` | `ResponsesProvider` |

### 公共 SDK 层 (`bamboo/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `BambooClient` | 接口 | bamboo.go:19 | 公共接口：Chat + Complete |
| `BambooMessage` | 结构体 | message.go:20 | 上层消息模型 (Role + ContentBlock 数组) |
| `RequestConfig` | 结构体 | config.go:15 | 请求配置 (Model, Temperature, ThinkingConfig, ProviderExtra 等) |
| `ThinkingConfig` | 类型别名 | config.go:8 | = provider.ThinkingConfig |
| `PtrFloat64/PtrBool/PtrInt64` | 函数 | config.go:38-54 | 指针辅助函数 |
| `ClientOption` | 函数类型 | option.go:9 | 客户端配置选项 func(*clientConfig) |
| `RequestOption` | 函数类型 | option.go:56 | 请求配置选项 func(*RequestConfig) |
| `WithTopK/WithFrequencyPenalty/...` | 函数 | option.go:62-147 | 7 个 ProviderExtra 配置函数 + WithExtra |
| `StreamConverter` | 结构体 | convert.go:201 | 将 provider.StreamEvent 序列转换为 Anthropic 风格事件序列 |
| `NewStreamConverter` | 构造函数 | convert.go:209 | StreamConverter 实例化 |

## CONVENTIONS

1. **值类型传递** — `Message`, `StreamEvent` 为值类型，通过 channel 安全传递，传递后视为只读
2. **Functional Options** — 所有适配器统一使用 `Option func(*config)` 模式，提供 `WithAPIKey`, `WithBaseURL`, `WithHeader` 三个选项
3. **双构造函数** — 每个适配器提供 `New*(apiKey)` 最简形式 + `New*WithOptions(opts...)` 完整形式，前者调用后者
4. **文件分工固定** — 每个适配器固定 7 个文件：`provider.go` / `chat.go` / `complete.go` / `stream.go` / `message.go` / `models.go` / `provider_test.go`
5. **中文注释** — 所有文档注释使用中文，遵循 Go doc 规范
6. **错误透传** — 使用 `bamboo-base-go/common/error` 包的 `xError.Error` 类型，保留完整上下文
7. **泛型基座** — `BaseProvider[T any]` 通过泛型参数嵌入不同 SDK Client
8. **参数透传双层方案** — Layer 1: 类型化 WithXxx() 函数写入 ProviderExtra；Layer 2: WithExtra() 兜底传递任意 key-value
9. **BlockStart 事件** — 所有适配器在首个文本增量前必须发出 BlockStart delta；Anthropic 原生支持，OpenAI 适配器通过 textBlockStarted/thinkingBlockStarted 参数合成
10. **ProviderExtra 取值** — 适配器中使用 GetExtra* 类型安全 helper，不做裸类型断言
11. **统一 UserAgent** — 所有适配器在构造函数中通过 `option.WithHeader("User-Agent", provider.GetUserAgent())` 设置统一 UserAgent，格式为 `BM-SDK/{version}`
12. **Reasoning 内容独立追踪** — OpenAI 适配器使用 `textBlockStarted` 和 `thinkingBlockStarted` 两个独立布尔标志追踪不同内容块状态
13. **StreamConverter 防御性自动补发** — 若 Provider 未发送 BlockStart，在首个文本/推理增量时自动合成对应类型的 BlockStart

## ANTI-PATTERNS (THIS PROJECT)

- **禁止** 直接依赖具体适配器包的业务逻辑 — 必须面向 `provider.Provider` 接口编程
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 在 `internal/provider/` 核心包中引入任何具体 SDK 依赖 — 核心包零外部依赖
- **禁止** 适配器之间互相引用 — 每个适配器独立，零耦合
- **禁止** 裸类型断言访问 ProviderExtra — 必须使用 GetExtra* helpers
- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — OpenAI 适配器中两者必须独立追踪

## UNIQUE STYLES

- `BaseProvider[T]` 泛型基座 + 类型别名 (`type Provider = BaseProvider[anthropic.Client]`) 模式，既统一又保留 SDK 特有能力
- `StreamDelta[E any]` 泛型增量，统一使用时通过 `StreamDelta[any]` + 具体 DeltaData 类型 (TextData / ThinkingData / ToolCallData / ToolCallDeltaData / UsageData) 做类型区分
- 配置可选字段用指针 (`*float64`) 区分"未设置"和"零值"
- `ThinkingConfig` 统一结构体，通过 `Enabled` + `BudgetTokens` 适配 Anthropic Thinking，通过 `ReasoningEffort` + `Summary` 适配 OpenAI Reasoning
- `ProviderExtra map[string]any` + string key 常量模式，扩展新参数只需添加常量和 WithXxx 函数
- `GetUserAgent()` 动态读取版本号 — 通过 `runtime/debug.ReadBuildInfo()` 在运行时读取，避免硬编码版本
- `StreamConverter` 防御性自动补发 — 若 Provider 未发送 BlockStart，自动合成，兼容不完整的 Provider 实现

## COMMANDS

```bash
# 测试
go test ./...

# 测试单个适配器
go test ./internal/provider/anthropic/...
go test ./internal/provider/openai/completions/...
go test ./internal/provider/openai/responses/...

# 编译检查
go build ./...

# 依赖整理
go mod tidy
```

## NOTES

- 所有适配器统一使用 `User-Agent: BM-SDK/{version}`（见 `internal/provider/version.go`），版本通过 `runtime/debug.ReadBuildInfo()` 动态读取
- `develop/` 目录在 `.gitignore` 中被忽略，仅本地设计文档
- 测试需要有效的 API Key，目前测试为 integration test（需要网络）
- 规划中的适配器：DeepSeek (OpenAI 兼容) / Google Gemini / 自定义端点
- StreamConverter 具有防御性自动补发机制：若 Provider 未发送 BlockStart，在首个文本/推理增量时自动合成对应类型的 BlockStart
- bamboo 包的 RequestOption 与 ClientOption 是两个独立的 Functional Options 体系，前者配置请求参数，后者配置客户端
- OpenAI Completions/Responses 适配器新增了独立的 `thinkingBlockStarted` 追踪，与 `textBlockStarted` 互不干扰
- `example/main.go` 提供了完整的使用示例代码

## 引用

- [internal/provider](./internal/provider/AGENTS.md) — 核心抽象层知识库
- [bamboo](./bamboo/AGENTS.md) — 公共 SDK 层知识库

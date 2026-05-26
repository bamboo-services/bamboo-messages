# PROJECT KNOWLEDGE BASE

**Generated:** 2026-05-26
**Commit:** 3a951f4
**Branch:** master

## OVERVIEW

Bamboo Messages — AI 对话协议标准化适配层，纯 Go SDK 库。通过统一 `Provider` 接口屏蔽 Anthropic Messages / OpenAI Chat Completions / OpenAI Responses 等协议差异，上层业务零改动切换 AI 后端。

## STRUCTURE

```
bamboo-messages/
├── provider/                    # 核心抽象层 — 接口 + 通用类型 + 流模型
│   ├── provider.go             # Provider 接口 (6 methods) + BaseProvider[T] 泛型基座
│   ├── type.go                 # Message / ChatConfig / Tool / CompletionResult 等值类型
│   └── stream.go               # StreamEvent / StreamDelta[E] + 5 种 Delta 构造函数
│
├── provider/anthropic/          # Anthropic Messages 协议适配器
├── provider/openai/completions/ # OpenAI Chat Completions 协议适配器
├── provider/openai/responses/   # OpenAI Responses 协议适配器
│
├── develop/docs/                # 设计文档 (overview / interface / message / stream / roadmap)
└── go.mod                       # Go 1.25, anthropic-sdk-go v1.27, openai-go v3.30
```

## WHERE TO LOOK

| 想做什么 | 去哪里 | 备注 |
|----------|--------|------|
| 理解核心接口 | `provider/provider.go` | 6 个方法，Chat/Complete 各有带 System 变体 |
| 理解通用类型 | `provider/type.go` | Message, ChatConfig, Tool, CompletionResult |
| 理解流式模型 | `provider/stream.go` | StreamEvent channel + 5 种 DeltaData 类型 |
| 添加新协议适配器 | 参见 `provider/AGENTS.md` | 结构完全模板化 |
| 理解消息转换 | `*/message.go` | 各适配器的 协议类型 ↔ provider 类型映射 |
| 理解流式解析 | `*/stream.go` | 各适配器的 SSE 事件 → StreamEvent 转换 |
| 查看模型定义 | `*/models.go` | 各协议的模型常量 |
| 运行测试 | `*/provider_test.go` | 每个适配器独立测试 |

## CODE MAP

### 核心抽象层 (`provider/`)

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `BaseProvider[T]` | 泛型结构体 | provider.go:14 | 适配器基座，嵌入底层 SDK Client |
| `Provider` | 接口 | provider.go:23 | 6 方法：Chat, ChatWithSystem, Complete, CompleteWithSystem, GetProviderType, GetAvailableModels |
| `Message` | 结构体 | type.go:48 | 统一消息模型 (Role + Content + ToolCalls + ToolCallID) |
| `ChatConfig` | 结构体 | type.go:90 | 请求配置 (Model, Temperature, MaxTokens, Tools 等) |
| `StreamEvent` | 结构体 | stream.go:10 | 流事件 (Type + Delta + Err)，值类型，跨 goroutine 传递 |
| `StreamDelta[E]` | 泛型结构体 | stream.go:17 | 流增量 (Type + Data)，泛型确保类型安全 |
| `CompletionResult` | 结构体 | type.go:36 | 非流式完整响应 |
| `NewTextDelta` 等 | 构造函数 | stream.go:73-115 | 5 种 Delta 工厂函数 |

### 适配器层 (结构完全一致)

| 适配器 | 包路径 | 底层 SDK | Provider 类型名 |
|--------|--------|----------|----------------|
| Anthropic Messages | `provider/anthropic` | `anthropic-sdk-go` | `Provider` |
| OpenAI Completions | `provider/openai/completions` | `openai-go/v3` | `CompletionsProvider` |
| OpenAI Responses | `provider/openai/responses` | `openai-go/v3` | `ResponsesProvider` |

## CONVENTIONS

1. **值类型传递** — `Message`, `StreamEvent` 为值类型，通过 channel 安全传递，传递后视为只读
2. **Functional Options** — 所有适配器统一使用 `Option func(*config)` 模式，提供 `WithAPIKey`, `WithBaseURL`, `WithHeader` 三个选项
3. **双构造函数** — 每个适配器提供 `New*(apiKey)` 最简形式 + `New*WithOptions(opts...)` 完整形式，前者调用后者
4. **文件分工固定** — 每个适配器固定 7 个文件：`provider.go` / `chat.go` / `complete.go` / `stream.go` / `message.go` / `models.go` / `provider_test.go`
5. **中文注释** — 所有文档注释使用中文，遵循 Go doc 规范
6. **错误透传** — 使用 `bamboo-base-go/common/error` 包的 `xError.Error` 类型，保留完整上下文
7. **泛型基座** — `BaseProvider[T any]` 通过泛型参数嵌入不同 SDK Client

## ANTI-PATTERNS (THIS PROJECT)

- **禁止** 直接依赖具体适配器包的业务逻辑 — 必须面向 `provider.Provider` 接口编程
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 在 `provider/` 核心包中引入任何具体 SDK 依赖 — 核心包零外部依赖
- **禁止** 适配器之间互相引用 — 每个适配器独立，零耦合

## UNIQUE STYLES

- `BaseProvider[T]` 泛型基座 + 类型别名 (`type Provider = BaseProvider[anthropic.Client]`) 模式，既统一又保留 SDK 特有能力
- `StreamDelta[E any]` 泛型增量，统一使用时通过 `StreamDelta[any]` + 具体 DeltaData 类型 (TextData / ThinkingData / ToolCallData / ToolCallDeltaData / UsageData) 做类型区分
- 配置可选字段用指针 (`*float64`) 区分"未设置"和"零值"

## COMMANDS

```bash
# 测试
go test ./...

# 测试单个适配器
go test ./provider/anthropic/...
go test ./provider/openai/completions/...
go test ./provider/openai/responses/...

# 编译检查
go build ./...

# 依赖整理
go mod tidy
```

## NOTES

- Anthropic 适配器构造时硬编码了 `User-Agent: vesper-ling/agent 0.0.1`（见 `anthropic/provider.go:71`），OpenAI 适配器未设置
- `develop/` 目录在 `.gitignore` 中被忽略，仅本地设计文档
- 测试需要有效的 API Key，目前测试为 integration test（需要网络）
- 规划中的适配器：DeepSeek (OpenAI 兼容) / Google Gemini / 自定义端点

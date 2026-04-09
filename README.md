# Bamboo Messages

> AI 对话协议标准化适配层 —— 为 Bamboo Services 生态提供统一的 AI 协议交互能力。

## 简介

Bamboo Messages 是一个**纯 Go SDK 库**，为上层业务提供标准化的 AI 对话协议接口。只需面向 `Provider` 接口编程，即可在 **Anthropic Messages 协议**、**OpenAI Chat Completions 协议**、**OpenAI Responses 协议** 等不同 AI 对话协议之间无缝切换。每个 Provider 实例可独立配置目标端点（官方 API、自建网关、代理服务或任何兼容第三方），无需关心底层协议差异。

### 核心能力

| 能力 | 说明 |
|------|------|
| **统一抽象** | 一套 `Message` / `StreamEvent` 模型屏蔽所有协议差异 |
| **流式 + 非流式** | 同时支持 SSE 流式输出和同步请求-响应两种模式 |
| **Provider 可插拔** | 每种协议实现独立包，按需引入，互不依赖 |
| **端点可配置** | 支持自定义 BaseURL，对接任意兼容端点（官方 API / 自建网关 / 第三方代理） |
| **Options 模式** | Functional Options 灵活配置 API Key、BaseURL、Headers 等 |

## 快速开始

```go
import (
    "context"
    "github.com/bamboo-services/bamboo-messages/provider"
    "github.com/bamboo-services/bamboo-messages/provider/anthropic"
)

func main() {
    ctx := context.Background()

    // ── 方式一：最简创建（默认连接 SDK 默认端点）──
    p := anthropic.NewProvider("sk-ant-xxx")

    // ── 方式二：自定义端点（自建网关 / 代理 / 第三方兼容服务）──
    // p := anthropic.NewProviderWithOptions(
    //     anthropic.WithAPIKey("your-api-key"),
    //     anthropic.WithBaseURL("https://your-gateway.example.com/v1"),
    //     anthropic.WithHeader("X-Custom-Header", "value"),
    // )

    // 构建消息
    messages := []provider.Message{
        {Role: provider.RoleUser, Content: "你好！"},
    }

    config := &provider.ChatConfig{
        Model:       "claude-sonnet-4-20250514",
        MaxTokens:   1024,
        Temperature: provider.Ptr(0.7),
    }

    // 流式对话
    eventCh := p.Chat(ctx, messages, config)
    for event := range eventCh {
        switch event.Type {
        case provider.StreamTypeDelta:
            if event.Delta.Type == provider.StreamDeltaTypeTextOutput {
                fmt.Print(string(event.Delta.Data.(provider.TextData)))
            }
        case provider.StreamTypeDone:
            fmt.Println("\n--- 完成 ---")
        case provider.StreamTypeError:
            log.Printf("错误: %v", event.Err)
        }
    }

    // 非流式对话
    result, err := p.Complete(ctx, messages, config)
    if err != nil {
        log.Fatalf("对话失败: %v", err)
    }
    fmt.Println(result.Content)
}
```

## 流式调用

通过 `<-chan StreamEvent` 实时接收 AI 模型的增量输出：

```
Start → Delta(text/thinking/tool_call) → Stop → Done
```

```go
eventCh := p.ChatWithSystem(ctx, "你是一个有帮助的助手。", messages, config)
for event := range eventCh {
    switch event.Delta.Type {
    case provider.StreamDeltaTypeTextOutput:     // 文本增量
    case provider.StreamDeltaTypeThinking:       // 思考过程
    case provider.StreamDeltaTypeToolCall:       // 工具调用开始
    case provider.StreamDeltaTypeToolCallDelta:  // 工具参数增量
    case provider.StreamDeltaTypeUsage:           // Token 用量统计
    }
}
```

## 非流式调用

同步获取完整响应，适用于不需要实时输出的场景：

```go
result, err := p.CompleteWithSystem(ctx, "你是一个有帮助的助手。", messages, config)
// result.Content      — 文本内容
// result.ToolCalls    — 工具调用列表
// result.FinishReason — 结束原因 (stop / length / tool_calls)
// result.Usage         — Token 用量统计
```

## 支持的协议适配器

| 协议适配器 | 包路径 | 目标协议 | 默认端点 | 状态 |
|------------|--------|---------|---------|------|
| **Anthropic Messages** | `provider/anthropic` | Anthropic Messages Protocol | api.anthropic.com | ✅ |
| **OpenAI Completions** | `provider/openai/completions` | Chat Completions Protocol | api.openai.com | ✅ |
| **OpenAI Responses** | `provider/openai/responses` | Responses Protocol | api.openai.com | ✅ |
| DeepSeek (兼容) | `provider/deepseek` | OpenAI Completions 兼容 | api.deepseek.com | 📋 规划中 |
| Google Gemini | `provider/gemini` | Gemini Protocol | generativelanguage.googleapis.com | 📋 规划中 |
| 自定义端点 | `provider/custom` | 任意兼容协议 | 用户自定义 | 📋 规划中 |

## Options 配置参考

所有 Provider 构造函数均支持 Functional Options 模式：

```go
// 完整选项（以 Anthropic 为例）
p := anthropic.NewProviderWithOptions(
    anthropic.WithAPIKey("sk-ant-xxx"),                          // API 密钥
    anthropic.WithBaseURL("https://custom-endpoint.example.com"), // 自定义端点（可选）
    anthropic.WithHeader("X-Custom-Header", "value"),            // 自定义请求头（可选）
)

// 最简形式（向后兼容）
p := anthropic.NewProvider("sk-ant-xxx")
```

| Option | 类型 | 说明 | 默认值 |
|--------|------|------|--------|
| `WithAPIKey(key)` | `string` | API 认证密钥 | 无（必填） |
| `WithBaseURL(url)` | `string` | 自定义基础 URL | 各协议 SDK 默认端点 |
| `WithHeader(k, v)` | `string, string` | 附加 HTTP 请求头 | 无 |

> OpenAI Completions / Responses 的 Options 用法完全一致，只需替换包名前缀即可：
> `completions.NewCompletionsProviderWithOptions(...)` / `responses.NewResponsesProviderWithOptions(...)`

## 核心类型

### Message — 对话消息

```go
type Message struct {
    Role       MessageRole `json:"role"`                   // user / assistant / tool
    Content    string      `json:"content,omitempty"`      // 消息内容
    ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // 助手发起的工具调用
    ToolCallID string      `json:"tool_call_id,omitempty"` // 工具响应的调用 ID
}
```

### ChatConfig — 请求配置

```go
type ChatConfig struct {
    Model       string            `json:"model"`
    Temperature *float64          `json:"temperature,omitempty"`
    TopP        *float64          `json:"top_p,omitempty"`
    MaxTokens   int64             `json:"max_tokens,omitempty"`
    Stop        []string          `json:"stop,omitempty"`
    Tools       []Tool            `json:"tools,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
}
```

### StreamEvent — 流事件

```go
type StreamEvent struct {
    Type  StreamType       `json:"type"`   // start / stop / done / error / delta
    Delta StreamDelta[any] `json:"delta"` // 增量数据（仅 Delta 类型有值）
    Err   *xError.Error    `json:"err"`     // 错误信息（仅 Error 类型有值）
}
```

### CompletionResult — 非流式结果

```go
type CompletionResult struct {
    Content      string        `json:"content"`
    ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
    FinishReason FinishReason  `json:"finish_reason"` // stop / length / tool_calls
    Usage        UsageData     `json:"usage"`
}
```

## 项目结构

```
bamboo-messages/
├── provider/                        # 核心抽象层
│   ├── provider.go                 # Provider 接口定义 (8 个方法)
│   ├── type.go                     # 通用类型定义
│   └── stream.go                   # 流式事件模型
│
├── provider/anthropic/              # Anthropic Messages 协议适配器
│   ├── provider.go · chat.go · complete.go
│   ├── stream.go · message.go · models.go
│   └── provider_test.go
│
├── provider/openai/
│   ├── completions/                # OpenAI Chat Completions 协议适配器
│   │   └── provider.go · chat.go · complete.go
│   │       stream.go · message.go · models.go · provider_test.go
│   └── responses/                  # OpenAI Responses 协议适配器
│       └── provider.go · chat.go · complete.go
│           stream.go · message.go · models.go · provider_test.go
│
├── develop/docs/                   # 设计文档
│   ├── overview.md                 # 项目概览
│   ├── provider-interface.md       # 接口详细设计
│   ├── message-format.md           # 消息模型设计
│   ├── stream-design.md            # 流式处理设计
│   └── roadmap.md                  # 开发路线图
│
├── go.mod · go.sum · LICENSE
└── README.md
```

## 技术栈

| 类别 | 选型 | 版本要求 |
|------|------|---------|
| 语言 | Go | 1.25+ |
| Anthropic 协议 SDK | [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) | v1.27+ |
| OpenAI 协议 SDK | [openai-go](https://github.com/openai/openai-go) | v3.30+ |
| 基础库 | [bamboo-base-go](https://github.com/bamboo-services/bamboo-base-go) | v1.0+ |

## 设计原则

1. **接口最小化** — Provider 接口只定义必要方法，扩展通过组合实现
2. **值类型优先** — `Message`、`StreamEvent` 为值类型，通过 channel 安全传递
3. **协议隔离** — 每种协议实现独立包，零耦合，按需引入
4. **配置外置** — API Key、Base URL、Headers 等通过 Options 模式注入，不硬编码
5. **错误透传** — 底层错误包装为统一类型，保留完整上下文

## 许可证

[MIT License](LICENSE)

---

属于 [Bamboo Services](https://github.com/bamboo-services) 生态。

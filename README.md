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
    "fmt"

    "github.com/bamboo-services/bamboo-messages/bamboo"
    "github.com/bamboo-services/bamboo-messages/internal/provider/anthropic"
)

func main() {
    ctx := context.Background()

    // ── 创建底层 Provider ──
    p := anthropic.NewProvider("sk-ant-xxx")

    // ── 自定义端点（自建网关 / 代理 / 第三方兼容服务）──
    // p := anthropic.NewProviderWithOptions(
    //     anthropic.WithAPIKey("your-api-key"),
    //     anthropic.WithBaseURL("https://your-gateway.example.com/v1"),
    //     anthropic.WithHeader("X-Custom-Header", "value"),
    // )

    // ── 创建 Bamboo Messages SDK 客户端 ──
    client := bamboo.NewClient(p)

    // 构建消息（ContentBlock 数组风格）
    messages := []bamboo.BambooMessage{
        bamboo.NewUserMessage("你好！"),
    }

    config := &bamboo.RequestConfig{
        Model:     "claude-sonnet-4-20250514",
        MaxTokens: 1024,
    }

    // ── 流式对话 ──
    eventCh, _ := client.Chat(ctx, messages, "你是一个有帮助的助手。", config)
    for event := range eventCh {
        switch event.Type {
        case bamboo.EventContentBlockDelta:
            if delta, ok := event.Delta.(*bamboo.StreamDelta); ok && delta.Type == bamboo.DeltaTextDelta {
                fmt.Print(delta.Text)
            }
        case bamboo.EventMessageStop:
            fmt.Println("\n--- 完成 ---")
        }
    }

    // ── 非流式对话 ──
    resp, _ := client.Complete(ctx, messages, "", config)
    fmt.Printf("响应: %s\n", resp.Content[0].Text)
    fmt.Printf("服务商: %s\n", resp.ProviderType)
    fmt.Printf("Token: input=%d, output=%d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
}
```

## 流式调用

通过 `<-chan StreamEvent` 实时接收 AI 模型的增量输出：

```
message_start → content_block_start → content_block_delta(text/thinking/tool) → content_block_stop → message_delta → message_stop
```

```go
eventCh, _ := client.Chat(ctx, messages, "你是一个有帮助的助手。", config)
for event := range eventCh {
    switch event.Type {
    case bamboo.EventContentBlockDelta:
        // event.Delta 为 *bamboo.StreamDelta，通过 Type 区分增量类型
        //   - bamboo.DeltaTextDelta      文本增量
        //   - bamboo.DeltaThinkingDelta  思考过程增量
        //   - bamboo.DeltaInputJSON      工具调用参数增量
        //   - bamboo.DeltaSignature      思考签名增量
    case bamboo.EventMessageDelta:
        // event.Delta 为 *bamboo.MessageDelta，携带停止原因
    case bamboo.EventMessageStop:
        // 消息传输完成
    }
}
```

## 非流式调用

同步获取完整响应，适用于不需要实时输出的场景：

```go
resp, err := client.Complete(ctx, messages, "你是一个有帮助的助手。", config)
// resp.Content      — 响应内容块列表 ([]ContentBlock)
// resp.StopReason   — 结束原因 (end_turn / max_tokens / tool_use / stop_sequence)
// resp.Usage         — Token 用量统计 (InputTokens / OutputTokens)
// resp.ProviderType  — 底层协议类型 (如 "anthropic")
// resp.RequestID     — 请求追踪 ID
```

## 支持的协议适配器

| 协议适配器 | 包路径 | 目标协议 | 默认端点 | 状态 |
|------------|--------|---------|---------|------|
| **Anthropic Messages** | `internal/provider/anthropic` | Anthropic Messages Protocol | api.anthropic.com | ✅ |
| **OpenAI Completions** | `internal/provider/openai/completions` | Chat Completions Protocol | api.openai.com | ✅ |
| **OpenAI Responses** | `internal/provider/openai/responses` | Responses Protocol | api.openai.com | ✅ |
| DeepSeek (兼容) | `internal/provider/deepseek` | OpenAI Completions 兼容 | api.deepseek.com | 📋 规划中 |
| Google Gemini | `internal/provider/gemini` | Gemini Protocol | generativelanguage.googleapis.com | 📋 规划中 |
| 自定义端点 | `internal/provider/custom` | 任意兼容协议 | 用户自定义 | 📋 规划中 |

## Options 配置参考

所有 Provider 构造函数均支持 Functional Options 模式：

```go
import "github.com/bamboo-services/bamboo-messages/internal/provider/anthropic"

// 完整选项（以 Anthropic 为例）
p := anthropic.NewProviderWithOptions(
    anthropic.WithAPIKey("sk-ant-xxx"),                          // API 密钥
    anthropic.WithBaseURL("https://custom-endpoint.example.com"), // 自定义端点（可选）
    anthropic.WithHeader("X-Custom-Header", "value"),            // 自定义请求头（可选）
)

// 最简形式（向后兼容）
p := anthropic.NewProvider("sk-ant-xxx")

// 然后创建 BambooClient
client := bamboo.NewClient(p)
```

| Option | 类型 | 说明 | 默认值 |
|--------|------|------|--------|
| `WithAPIKey(key)` | `string` | API 认证密钥 | 无（必填） |
| `WithBaseURL(url)` | `string` | 自定义基础 URL | 各协议 SDK 默认端点 |
| `WithHeader(k, v)` | `string, string` | 附加 HTTP 请求头 | 无 |

> OpenAI Completions / Responses 的 Options 用法完全一致，只需替换包名前缀即可：
> `completions.NewCompletionsProviderWithOptions(...)` / `responses.NewResponsesProviderWithOptions(...)`

## 核心类型

> 以下类型均定义在 `bamboo` 包中，是面向上层业务的公共 API。

### BambooMessage — 对话消息

```go
// 创建消息的便捷构造函数
msg := bamboo.NewUserMessage("你好！")
msg := bamboo.NewAssistantMessage("回复内容")

// 工具结果消息：使用 ContentBlock 构建
msg := bamboo.NewUserMessageBlocks(
    bamboo.NewToolResultBlock("call-id", "工具返回内容", false),
)

// 也可使用 ContentBlock 数组构建富文本消息
msg := bamboo.NewUserMessageBlocks(
    bamboo.NewTextBlock("描述这张图片"),
    bamboo.NewImageBlock(bamboo.ContentSource{
        Type: "url",
        URL:  "https://example.com/img.png",
    }),
)
```

### RequestConfig — 请求配置

```go
config := &bamboo.RequestConfig{
    Model:       "claude-sonnet-4-20250514",
    MaxTokens:   1024,
    Temperature: bamboo.PtrFloat64(0.7), // *float64，区分未设置和零值
    Tools:       []bamboo.Tool{...}, // 工具定义（可选）
}
```

### StreamEvent — 流事件

```go
type StreamEvent struct {
    Type        StreamEventType  // message_start / content_block_delta / message_stop / ...
    Message     *BambooMessage   // message_start 事件
    Index       int              // 内容块索引
    ContentBlock *ContentBlock   // content_block_start 事件
    Delta       any              // *StreamDelta 或 *MessageDelta
    Usage       *Usage           // Token 用量
    Error       *BambooError     // 错误详情
}
```

### Response — 非流式结果

```go
type Response struct {
    ID           string          // 消息 ID
    Type         string          // "message"
    Role         MessageRole     // "assistant"
    Content      []ContentBlock  // 响应内容块
    Model        string          // 使用的模型
    StopReason   FinishReason    // end_turn / max_tokens / tool_use
    Usage        Usage           // Token 用量
    ProviderType string          // 底层协议类型（Bamboo 扩展）
    RequestID    string          // 请求追踪 ID（Bamboo 扩展）
    CreatedAt    int64           // 创建时间戳（Bamboo 扩展）
}

## 项目结构

```
bamboo-messages/
├── bamboo/                          # 公共 SDK 层 — 面向上层业务的统一 API
│   ├── bamboo.go                   # BambooClient 接口 + Chat/Complete 实现
│   ├── message.go                  # BambooMessage + ContentBlock 消息模型
│   ├── response.go                 # Response / Usage 非流式响应类型
│   ├── stream.go                   # StreamEvent / StreamDelta 流事件模型
│   ├── tool.go                     # Tool / ToolInputSchema 工具定义
│   ├── config.go                   # RequestConfig 请求配置
│   ├── option.go                   # Functional Options (WithProvider / WithDefaultModel)
│   ├── convert.go · content.go     # 类型转换 + 内容处理
│   ├── errors.go                   # BambooError 错误类型
│   └── *_test.go                   # 单元测试 + 集成测试
│
├── internal/provider/              # 核心抽象层（内部包）
│   ├── provider.go                 # Provider 接口定义 (6 个方法)
│   ├── type.go                     # 通用类型定义
│   └── stream.go                   # 流式事件模型
│
├── internal/provider/anthropic/    # Anthropic Messages 协议适配器
│   ├── provider.go · chat.go · complete.go
│   ├── stream.go · message.go · models.go
│   └── provider_test.go
│
├── internal/provider/openai/
│   ├── completions/                # OpenAI Chat Completions 协议适配器
│   │   └── provider.go · chat.go · complete.go
│   │       stream.go · message.go · models.go · provider_test.go
│   └── responses/                  # OpenAI Responses 协议适配器
│       └── provider.go · chat.go · complete.go
│           stream.go · message.go · models.go · provider_test.go
│
├── develop/docs/                   # 设计文档（本地，不提交）
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

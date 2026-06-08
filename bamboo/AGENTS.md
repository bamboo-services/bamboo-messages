# bamboo 知识库

## 概述

Bamboo Messages SDK 的公共 API 层（门面层），面向业务开发者提供统一的消息模型、流事件、工具定义和客户端接口。该包零外部 SDK 依赖，仅依赖 Go 标准库和 `internal/provider` 核心抽象层。

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
├── content.go       # ContentBlock 构造函数
├── errors.go        # BambooError 错误类型
└── *_test.go        # 单元测试 + 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解公共 API 入口 | `bamboo.go` | `BambooClient` 接口定义 `Chat` 和 `Complete` |
| 创建客户端 | `bamboo.go` | `NewClient(p)` 或 `NewClientWithOptions(opts...)` |
| 构建消息 | `message.go` + `content.go` | `NewUserMessage`, `NewTextBlock`, `NewToolResultBlock` 等 |
| 配置请求参数 | `config.go` + `option.go` | `RequestConfig` 结构体 + `WithToolChoice`/`WithResponseFormat`/`WithUserID`/`WithParallelToolCalls` 等 |
| 处理流事件 | `stream.go` | `StreamEvent` 结构体 + 事件类型常量 |
| 理解类型转换 | `convert.go` | `messagesToProvider`, `configToProvider`, `resultToResponse` |
| 添加 ContentBlock 类型 | `content.go` + `stream.go` | 扩展 `ContentBlockType` 和 `StreamDeltaType` |
| 自定义错误处理 | `errors.go` | `BambooError` 类型 + 错误类型常量 |

## 约定

- **指针区分未设置/零值** — `Temperature`/`TopP` 等可选字段使用 `*float64`，通过 `PtrFloat64()` 辅助函数设置
- **双 Options 体系** — `ClientOption`（配置客户端）和 `RequestOption`（配置单次请求）完全独立
- **类型化字段优先** — `RequestConfig` 的 `UserID`/`ToolChoice`/`ResponseFormat`/`ParallelToolCalls` 等通用参数使用类型化字段，不再通过 ProviderExtra 传递
- **ProviderExtra 兜底** — `WithExtra()` 用于传递任何未覆盖的扩展参数，直接写入 `RequestConfig.ProviderExtra`
- **StreamConverter 防御性设计** — 若 provider 未发送 BlockStart，在首个文本/推理增量时自动合成
- **ContentBlock 数组风格** — 一条消息可包含多个不同类型的内容块（文本、图片、工具调用等）
- **工具结果拆分** — `convert.go` 的 `messagesToProvider` 将单条消息的 tool_result 拆分为独立的 `RoleTool` 消息

## 反模式

- **禁止** 在 `convert.go` 中遗漏 ContentBlock 类型处理 — 新增类型必须在 `messagesToProvider` 和 `resultToResponse` 中同步处理
- **禁止** 修改 `StreamEvent` 传递后的字段 — 值类型传递后应视为只读
- **禁止** 在 `bamboo` 包中引入具体 SDK 依赖 — 必须面向 `provider.Provider` 接口编程
- **禁止** 裸类型断言访问 `StreamEvent.Delta` — 应通过事件类型判断后安全断言

## 调试路径

1. 消息转换错误 → 检查 `convert.go` 的 `messagesToProvider` 是否正确处理 ContentBlock 类型
2. 流事件类型不匹配 → 检查 `convert.go` 的 `StreamConverter.Convert` 是否正确映射 Delta 类型
3. 配置参数不生效 → 检查 `configToProvider` 是否遗漏了新字段
4. 工具调用结果丢失 → 检查 `messagesToProvider` 中 tool_result 的拆分逻辑
5. 客户端初始化失败 → 确认 `NewClient` 传入了非 nil 的 provider

## 引用

无子级 AGENTS.md

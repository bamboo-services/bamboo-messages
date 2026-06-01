# responses 知识库

## 概述

OpenAI Responses 协议适配器，将 OpenAI Responses API 转换为统一的 `provider` 接口。基于 `openai-go/v3` SDK 构建，与 `completions` 适配器共享同一 SDK 但使用不同的 API 端点（Responses 端点）。

## 目录结构

```text
internal/provider/openai/responses/
├── provider.go       # Provider 构造函数 + Options 模式 + 类型别名
├── chat.go           # 流式对话实现
├── complete.go       # 非流式对话实现
├── stream.go         # SSE → StreamEvent 转换
├── stream_test.go    # 流式事件单元测试
├── message.go        # 消息格式双向转换 (buildInput/buildAssistantItem)
├── models.go         # 模型常量 + GetAvailableModels
├── tools.go          # 工具定义转换
└── provider_test.go  # 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewResponsesProvider(apiKey)` 或 `NewResponsesProviderWithOptions(opts...)` |
| 修改流式请求构建 | `chat.go` | 构建 `responses.ResponseNewParams` |
| 修改非流式请求构建 | `complete.go` | 与 chat.go 相同的参数构建逻辑 |
| 修改 SSE 事件解析 | `stream.go` | 处理 `response.*` 系列事件 |
| 修改消息格式映射 | `message.go` | `buildInput` 和 `buildAssistantItem` 函数 |
| 添加支持的模型 | `models.go` | 使用 `openai.ChatModel*` 常量 |
| 修改工具定义转换 | `tools.go` | `buildTools` 函数 |
| 测试流式事件处理 | `stream_test.go` | reasoning_text.delta 相关测试 |

## 约定

- **事件类型丰富** — OpenAI Responses 协议提供更多事件类型：`response.created`, `response.output_item.added`, `response.output_text.delta`, `response.reasoning_text.delta`, `response.function_call_arguments.delta`, `response.function_call_arguments.done`, `response.completed`, `response.failed`, `response.incomplete`
- **BlockStart 合成** — 同 Completions，没有原生 `content_block_start`，通过 `textBlockStarted`/`thinkingBlockStarted` 合成
- **Reasoning 参数** — 支持 `ReasoningEffort` + `Summary` 双参数，映射到 `shared.ReasoningParam`
- **ResponseFormat 字符串模式** — 支持 `"text"` 和 `"json_object"` 两种简单字符串值
- **ToolChoice 字符串模式** — 支持 `"auto"`, `"none"`, `"required"` 等值
- **完成原因推断** — Responses 协议不直接提供 finish_reason，根据 `Status` 和是否有 ToolCalls 推断
- **输入格式差异** — 使用 `ResponseInputItemUnionParam` 而非 `Message` 数组，支持更丰富的输入类型

## 反模式

- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — 两者必须独立追踪
- **禁止** 在 `stream.go` 中遗漏事件类型处理 — 新增事件类型必须添加到 `handleStreamEvent` 的 switch 中
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtra*` helper
- **禁止** 遗漏 `stream.Close()` — 必须在 goroutine 结束时关闭 stream

## 调试路径

1. 流式输出异常 → 检查 `stream.go` 的 `handleStreamEvent` 是否正确分发事件类型
2. Reasoning 内容不显示 → 检查 `contentReasoningTextDelta` 是否正确合成 BlockStart
3. BlockStart 重复或缺失 → 检查 `textBlockStarted`/`thinkingBlockStarted` 状态管理
4. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成 `ToolUnionParam`
5. 响应格式不生效 → 检查 `ResponseFormat` 字符串值是否为 `"text"` 或 `"json_object"`
6. 完成原因错误 → 检查 `complete.go` 中根据 `Status` 和 ToolCalls 推断的逻辑

## 引用

无子级 AGENTS.md

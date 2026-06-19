# responses 知识库

## 概述

OpenAI Responses 协议适配器，将 OpenAI Responses API 转换为统一的 `provider.Provider` 接口。基于 `openai-go/v3` SDK 构建，与 `completions` 适配器共享同一 SDK 但使用不同的 API 端点（Responses 端点）。

## 目录结构

```text
provider/openai/responses/
├── provider.go       # Provider 构造函数 + Options 模式 + 类型别名
├── params.go         # buildParams — 共享参数构建（Chat/Complete 统一入口）
├── chat.go           # 流式对话实现
├── complete.go       # 非流式对话实现
├── stream.go         # SSE → StreamEvent 转换
├── stream_test.go    # 流式事件单元测试
├── message.go        # 消息格式双向转换 (buildInput/buildAssistantItem)
├── models.go         # 模型常量 + GetAvailableModels
├── option.go         # OpenaiResponsesOption + WithStore/WithModalities/WithPreviousResponseID/WithTruncation
├── tools.go          # 工具定义转换 (buildTools)
└── provider_test.go  # 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewResponsesProvider(apiKey)` 或 `NewResponsesProviderWithOptions(opts...)` |
| 修改参数构建逻辑 | `params.go` | `buildParams` — Chat/Complete 共享的参数构建入口 |
| 修改流式请求流程 | `chat.go` | 调用 `buildParams` 构建 `responses.ResponseNewParams` 后发起流式请求 |
| 修改非流式请求流程 | `complete.go` | 调用 `buildParams` 后发起同步请求 |
| 修改 SSE 事件解析 | `stream.go` | 处理 `response.*` 系列事件 |
| 修改消息格式映射 | `message.go` | `buildInput` 和 `buildAssistantItem` 函数 |
| 添加支持的模型 | `models.go` | 使用 `openai.ChatModel*` 常量 |
| 修改工具定义转换 | `tools.go` | `buildTools` 函数 |
| 配置特有参数 | `option.go` | `WithStore` / `WithModalities` / `WithPreviousResponseID` / `WithTruncation` |
| 测试流式事件处理 | `stream_test.go` | reasoning_text.delta 相关测试 |

## 约定

- **参数构建集中化** — `params.go` 的 `buildParams` 是 Chat 和 Complete 的共享参数构建入口，确保流式和非流式路径参数一致
- **事件类型丰富** — OpenAI Responses 协议提供更多事件类型：`response.created`, `response.output_item.added`, `response.output_text.delta`, `response.reasoning_text.delta`, `response.function_call_arguments.delta`, `response.function_call_arguments.done`, `response.completed`, `response.failed`, `response.incomplete`
- **BlockStart 合成** — 同 Completions，没有原生 `content_block_start`，通过 `textBlockStarted` / `thinkingBlockStarted` 合成
- **Reasoning 参数** — `ThinkingConfig.Effort` 映射为 `ReasoningEffort`，`Summary` 按 effort 自动推导（none→""、low→"concise"、medium→"auto"、high→"detailed"），映射到 `shared.ReasoningParam`
- **ResponseFormat 字符串模式** — 支持 `"text"` 和 `"json_object"` 两种简单字符串值
- **ToolChoice 字符串模式** — 支持 `"auto"` / `"none"` / `"required"` 等值
- **完成原因推断** — Responses 协议不直接提供 finish_reason，根据 `Status` 和是否有 ToolCalls 推断
- **输入格式差异** — 使用 `ResponseInputItemUnionParam` 而非 `Message` 数组，支持更丰富的输入类型

## 反模式

- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — 两者必须独立追踪
- **禁止** 在 `chat.go` 和 `complete.go` 中重复构建参数逻辑 — 必须统一调用 `params.go` 的 `buildParams`
- **禁止** 在 `stream.go` 中遗漏事件类型处理 — 新增事件类型必须添加到 `handleStreamEvent` 的 switch 中
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtra*` helper
- **禁止** 遗漏 `stream.Close()` — 必须在 goroutine 结束时关闭 stream

## 调试路径

1. 参数构建错误 → 检查 `params.go` 的 `buildParams` 是否正确映射所有字段
2. 流式输出异常 → 检查 `stream.go` 的 `handleStreamEvent` 是否正确分发事件类型
3. Reasoning 内容不显示 → 检查 `contentReasoningTextDelta` 是否正确合成 BlockStart
4. BlockStart 重复或缺失 → 检查 `textBlockStarted` / `thinkingBlockStarted` 状态管理
5. 响应格式不生效 → 检查 `ResponseFormat` 字符串值是否为 `"text"` 或 `"json_object"`
6. 完成原因错误 → 检查 `complete.go` 中根据 `Status` 和 ToolCalls 推断的逻辑
7. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成 `ToolUnionParam`

## 引用

无子级 AGENTS.md

# completions 知识库

## 概述

OpenAI Chat Completions 协议适配器，将 OpenAI Chat Completions API 转换为统一的 `provider` 接口。基于 `openai-go/v3` SDK 构建，与 `responses` 适配器共享同一 SDK 但使用不同的 API 端点。

## 目录结构

```text
internal/provider/openai/completions/
├── provider.go       # Provider 构造函数 + Options 模式 + 类型别名
├── chat.go           # 流式对话实现
├── complete.go       # 非流式对话实现
├── stream.go         # SSE → StreamEvent 转换
├── stream_test.go    # 流式事件单元测试
├── message.go        # 消息格式双向转换
├── models.go         # 模型常量 + GetAvailableModels
├── tools.go          # 工具定义转换
└── provider_test.go  # 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewCompletionsProvider(apiKey)` 或 `NewCompletionsProviderWithOptions(opts...)` |
| 修改流式请求构建 | `chat.go` | 构建 `openai.ChatCompletionNewParams` |
| 修改非流式请求构建 | `complete.go` | 与 chat.go 相同的参数构建逻辑 |
| 修改 SSE 事件解析 | `stream.go` | 处理 `ChatCompletionChunk` 和 `handleChoice` |
| 修改消息格式映射 | `message.go` | `buildMessages` 函数 |
| 添加支持的模型 | `models.go` | 使用 `openai.ChatModel*` 常量 |
| 修改工具定义转换 | `tools.go` | `buildTools` 和 `buildStop` 函数 |
| 测试流式事件处理 | `stream_test.go` | `handleChoice` 的 reasoning_content 测试 |

## 约定

- **BlockStart 合成** — OpenAI Completions 没有原生 `content_block_start` 事件，通过 `textBlockStarted *bool` 在首个文本增量前合成 `NewBlockStartDelta("text")`
- **Reasoning 内容提取** — 从 `delta.JSON.ExtraFields["reasoning_content"]` 提取推理内容，首次推理增量前合成 `NewBlockStartDelta("thinking")`
- **双 Block 状态追踪** — `textBlockStarted` 和 `thinkingBlockStarted` 独立追踪，互不干扰
- **Usage 流式返回** — 通过 `params.StreamOptions.IncludeUsage=true` 启用，在最后一个 chunk 中提取
- **参数透传** — FrequencyPenalty/PresencePenalty/Seed/ToolChoice/ResponseFormat 均通过 `ProviderExtra` 透传
- **ReasoningEffort 映射** — `ThinkingConfig.ReasoningEffort` → `shared.ReasoningEffort`

## 反模式

- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — 两者必须独立追踪
- **禁止** 在 `handleChunk` 中直接处理 choices 逻辑 — 应委托给 `handleChoice`
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtra*` helper
- **禁止** 遗漏 `stream.Close()` — 必须在 goroutine 结束时关闭 stream

## 调试路径

1. 流式输出异常 → 检查 `stream.go` 的 `handleChunk`/`handleChoice` 是否正确提取 delta
2. Reasoning 内容不显示 → 检查 `delta.JSON.ExtraFields["reasoning_content"]` 是否正确解析
3. BlockStart 重复或缺失 → 检查 `textBlockStarted`/`thinkingBlockStarted` 状态管理
4. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成 `ChatCompletionToolUnionParam`
5. Usage 统计缺失 → 确认 `StreamOptions.IncludeUsage` 已设置

## 引用

无子级 AGENTS.md

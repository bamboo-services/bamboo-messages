# completions 知识库

## 概述

OpenAI Chat Completions 协议适配器，将 OpenAI Chat Completions API 转换为统一的 `provider.Provider` 接口。基于 `net/http` + JSON 构建，不依赖外部 SDK，与 `responses` 适配器共享相同的 HTTP 基座但使用不同的 API 端点。支持 Legacy 兼容模式，可对接旧版 OpenAI 兼容端点（如第三方代理）。

## 目录结构

```text
provider/openai/completions/
├── provider.go              # Provider 构造函数 + Options 模式 (WithAPIKey/WithBaseURL/WithHeader/WithLegacyCompat/WithInterceptor) + legacyCompat 标志 + 拦截器 Transport 注入
├── params.go                # buildParams — 共享参数构建（含 Legacy 分支 + Prediction JSON 回退）
├── chat.go                  # 流式对话实现
├── complete.go              # 非流式对话实现 — 含 reasoning_content 提取
├── stream.go                # SSE → StreamEvent 转换 + FinishReason 携带
├── stream_test.go           # 流式事件单元测试
├── message.go               # 消息格式双向转换 — 空 tool_calls 防御 + 文档块警告
├── models.go                # 模型常量 + GetAvailableModels
├── option.go                # OpenaiCompletionsOption + WithFrequencyPenalty/WithPresencePenalty/WithSeed/WithPrediction
├── tools.go                 # 工具定义转换 (buildTools/buildStop)
├── types.go                 # OpenAI Completions 协议原生请求/响应 DTO
├── mock_test.go             # httptest mock server 测试辅助工具
├── provider_test.go         # 集成测试
├── legacy_compat_test.go    # Legacy 兼容模式单元测试
├── message_test.go          # 空 tool_calls 序列化测试
├── integration_cross_protocol_test.go # 跨协议集成测试
└── params_audit_test.go     # Prediction 类型断言 + ResponseFormat 审计测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewCompletionsProvider(apiKey)` 或 `NewCompletionsProviderWithOptions(opts...)` |
| 注册请求拦截器 | `provider.go` | `WithInterceptor(fn)` — 在 HTTP Transport 层改写已序列化的请求 body |
| 修改参数构建逻辑 | `params.go` | `buildParams` — Chat/Complete 共享入口，含 Legacy 分支 |
| 修改流式请求流程 | `chat.go` | 调用 `buildParams` 后发起流式请求 |
| 修改非流式请求流程 | `complete.go` | 调用 `buildParams` 后发起同步请求 |
| 修改 SSE 事件解析 | `stream.go` | 处理 `ChatCompletionChunk` 和 `handleChoice` |
| 修改消息格式映射 | `message.go` | `buildMessages` / `buildAssistantMessage` 函数 |
| 添加支持的模型 | `models.go` | 使用 `openai.ChatModel*` 常量 |
| 修改工具定义转换 | `tools.go` | `buildTools` 和 `buildStop` 函数 |
| 配置特有参数 | `option.go` | `WithFrequencyPenalty` / `WithPresencePenalty` / `WithSeed` / `WithPrediction` |
| 测试 Legacy 兼容 | `legacy_compat_test.go` | max_tokens / parallel_tool_calls / reasoning_effort 差异测试 |
| 启用 debug 日志 | `provider.go` | 环境变量 `BAMBOO_DEBUG=1/true/on` |

## 代码地图

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `CompletionsProvider` | 结构体 | provider.go | 持有 `*provider.HTTPClient` + `legacyCompat bool` + `streamDrainTimeout time.Duration` |
| `Option` | 函数类型 | provider.go | `func(*config)` — Provider 配置选项 |
| `OpenaiCompletionsOption` | 函数类型 | option.go | `func(*completionsRequestConfig)` — 请求级配置选项 |
| `WithInterceptor` | 函数 | provider.go | 注册请求拦截器（转发到 `provider.WithInterceptor`） |
| `WithLegacyCompat` | 函数 | provider.go | Option: 启用 Legacy 兼容模式 |
| `WithFrequencyPenalty` / `WithPresencePenalty` / `WithSeed` / `WithPrediction` | 函数 | option.go | OpenaiCompletionsOption 请求级参数 |
| `buildParams` | 方法 | params.go | Chat/Complete 共享参数构建（含 Prediction JSON 回退） |
| `buildAssistantMessage` | 方法 | message.go | provider.Message → OpenAI Assistant 消息（空 tool_calls 防御） |
| `handleChoice` | 方法 | stream.go | 处理单个 choice 的 delta + FinishReason |
| `mapFinishReason` | 函数 | stream.go | OpenAI finish_reason → provider.FinishReason 映射 |

## 约定

- **参数构建集中化** — `params.go` 的 `buildParams` 是 Chat 和 Complete 的共享参数构建入口，通过 `p.legacyCompat` 标志做条件分支
- **统一 HTTP 传输** — 通过 `provider.NewHTTPClient` 创建 `*provider.HTTPClient`，认证使用 `Authorization: Bearer` 模式；流式响应通过 `provider.NewSSEScanner` 解析 SSE 帧
- **本地 DTO 模型** — 请求/响应结构通过 `types.go` 本地定义（`chatCompletionChunk`、`chatCompletionResponse` 等），不依赖外部 SDK
- **Legacy 兼容模式** — `legacyCompat` 标志控制旧版端点兼容行为：MaxTokens 使用旧字段名 `max_tokens`（非 `max_completion_tokens`）；ParallelToolCalls 仅在有工具时设置；thinking 从 ProviderExtra 提取并通过 `params["thinking"]` 注入
- **BlockStart 合成** — OpenAI Completions 没有原生 `content_block_start` 事件，通过 `textBlockStarted *bool` 在首个文本增量前合成 `NewBlockStartDelta("text")`
- **Reasoning 内容提取（流式）** — 从 `delta.ReasoningContent` 提取推理内容，兼容 `delta.Reasoning` 字段，首次推理增量前合成 `NewBlockStartDelta("thinking")`
- **Reasoning 内容提取（非流式）** — `complete.go` 从 `choice.Message.ReasoningContent` 提取推理内容，兼容 `reasoning` 字段，填充到 `CompletionResult.Thinking`
- **FinishReason 流式携带** — `handleChoice` 在 `choice.FinishReason != ""` 时，通过 `mapFinishReason` 映射并填充到 `StreamEvent.FinishReason`
- **空 tool_calls 防御** — `buildAssistantMessage` 仅在 `len(msg.ToolCalls) > 0` 时填充 `ToolCalls` 字段，避免序列化出 `"tool_calls": []` 空数组。部分第三方兼容端点（如 Kimi coding API）会将空数组视为无效请求
- **Prediction JSON 回退** — `buildParams` 中 Prediction 参数的类型断言失败时，通过 `json.Marshal` → `json.Unmarshal` 做安全转换，避免 `map[string]any` 类型的 Prediction 被静默丢弃
- **双 Block 状态追踪** — `textBlockStarted` 和 `thinkingBlockStarted` 独立追踪，互不干扰
- **Usage 流式返回** — 通过请求 DTO 的 `stream_options.include_usage=true` 启用，在最后一个 chunk 中提取
- **参数透传** — FrequencyPenalty / PresencePenalty / Seed / Prediction 通过 `OpenaiCompletionsOption` 设置，合并到 ProviderExtra 后透传；ToolChoice / ResponseFormat 通过 `ChatConfig` 类型化字段传递
- **ReasoningEffort 映射** — `ThinkingConfig.Effort` → 请求 DTO 的 `reasoning_effort` 字段（Legacy 模式仍透传 thinking）
- **Debug 日志** — 通过环境变量 `BAMBOO_DEBUG=1/true/on` 启用；请求前通过 `httpClient.DoWithDebug` 输出 Provider 类型、端点、headers（敏感字段脱敏）和 body（长文本截断）
- **拦截器 Transport 注入** — 构造函数中调用 `provider.NewHTTPClient` 时传入 `cfg.interceptors`；非空时由 `NewInterceptorHTTPClient` 包装 Transport，无拦截器时使用标准库默认 client
- **HTTP 错误结构化** — `complete.go` 在上游返回 HTTP >= 400 时，使用 `pkgErrors.NewBambooError(category, message, statusCode)` 包装错误，使状态码作为结构化字段贯穿错误链路；`bamboo.go` 的 `wrapProviderError` 使用 `errors.As` 提取 `*BambooError`，直接透传避免重复包装

## 反模式

- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — 两者必须独立追踪
- **禁止** 在 `chat.go` 和 `complete.go` 中重复构建参数逻辑 — 必须统一调用 `params.go` 的 `buildParams`
- **禁止** 在 `handleChunk` 中直接处理 choices 逻辑 — 应委托给 `handleChoice`
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtra*` helper
- **禁止** 遗漏 `stream.Close()` — 必须在 goroutine 结束时关闭 stream
- **禁止** 无条件初始化 `ToolCalls` 为空切片 — 必须检查 `len(msg.ToolCalls) > 0` 后再填充，避免序列化空数组

## 调试路径

1. 参数构建错误 → 检查 `params.go` 的 `buildParams`，特别是 `legacyCompat` 分支
2. 流式输出异常 → 检查 `stream.go` 的 `handleChunk` / `handleChoice` 是否正确提取 delta
3. Reasoning 内容不显示 → 流式：检查 `delta.JSON.ExtraFields["reasoning_content"]` 是否正确解析；非流式：检查 `complete.go` 中的 ExtraFields 提取
4. BlockStart 重复或缺失 → 检查 `textBlockStarted` / `thinkingBlockStarted` 状态管理
5. FinishReason 缺失 → 检查 `handleChoice` 是否在 `choice.FinishReason != ""` 时正确映射
6. Legacy 兼容失败 → 检查 `legacyCompat` 标志是否正确设置，max_tokens / parallel_tool_calls / reasoning_effort 行为是否符合预期
7. Prediction 被丢弃 → 检查类型断言是否失败，启用 debug 查看 JSON 回退日志
8. 空 tool_calls 问题 → 检查 `buildAssistantMessage` 是否正确跳过空 ToolCalls
9. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成工具定义（`[]map[string]any`）
10. Usage 统计缺失 → 确认 `StreamOptions.IncludeUsage` 已设置
11. 请求参数不确定 → 设置 `BAMBOO_DEBUG=1`，查看实际发送的 headers 和 body

## BaseURL 配置说明

OpenAI Completions 适配器通过 `provider.HTTPClient` 发起请求，请求路径为 `/chat/completions`。`HTTPClient` 将 BaseURL 与路径简单拼接，因此 BaseURL 必须包含 `/v1` 版本路径（或 `/v4`、`v3` 等），否则拼接出的路径会缺少版本前缀。

### 版本路径要求

BaseURL **必须包含 `/v1` 版本路径**（或 `/v4`、`/v3` 等其他版本号），否则 `HTTPClient` 拼出的路径会缺少版本前缀，导致上游返回错误。

### 正确示例

| 端点 | BaseURL | 实际请求路径 |
|------|---------|-----------------|
| OpenAI 官方 | `https://api.openai.com/v1` | `https://api.openai.com/v1/chat/completions` |
| 智谱 GLM Coding | `https://open.bigmodel.cn/api/coding/paas/v4` | `.../paas/v4/chat/completions` |
| Kimi Coding | `https://api.kimi.com/coding/v1` | `.../coding/v1/chat/completions` |
| 豆包 Coding | `https://ark.cn-beijing.volces.com/api/coding/v3` | `.../coding/v3/chat/completions` |

### ⚠️ 常见错误

```
❌ https://ai.akass.cn         → HTTPClient 请求 https://ai.akass.cn/chat/completions（缺少 /v1）
✅ https://ai.akass.cn/v1      → HTTPClient 请求 https://ai.akass.cn/v1/chat/completions
```

> **注意**: newapi bridge 层会自动检测并补全 `/v1`，但直接配置 Provider 时需手动确保 BaseURL 包含版本路径。

## 引用

无子级 AGENTS.md

# anthropic 知识库

## 概述

Anthropic Messages 协议适配器，将 Anthropic Claude 系列模型的原生协议转换为统一的 `provider.Provider` 接口。基于 `anthropic-sdk-go` v1.27+ 构建，支持 Claude 4 和 Claude 3 系列模型，原生支持 prompt caching（通过 `CacheControl` 标记）。

## 目录结构

```text
provider/anthropic/
├── provider.go      # Provider 构造函数 + Options 模式 (WithAPIKey/WithBaseURL/WithHeader/WithDebug) + 类型别名
├── params.go        # buildParams — 共享参数构建（Chat/Complete 统一入口）+ ResponseFormat/ParallelToolCalls 处理
├── chat.go          # 流式对话实现 (Chat/ChatWithSystem) + finishReason 跨事件追踪
├── complete.go      # 非流式对话实现 (Complete/CompleteWithSystem) — 含 thinking content block 处理
├── stream.go        # SSE → StreamEvent 转换 + finishReason 提取与携带
├── message.go       # 消息格式双向转换 (buildMessages)
├── models.go        # 模型常量 + GetAvailableModels
├── option.go        # AnthropicMessagesOption + WithTopK/WithBudgetTokens
├── tools.go         # 工具定义转换 (buildTools)
├── params_test.go   # buildParams 单元测试
├── params_audit_test.go  # ResponseFormat/ParallelToolCalls/SystemCacheControl 审计测试
└── provider_test.go # 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewProvider(apiKey)` 或 `NewProviderWithOptions(opts...)` |
| 修改参数构建逻辑 | `params.go` | `buildParams` — Chat/Complete 共享的参数构建入口 |
| 修改流式请求流程 | `chat.go` | 调用 `buildParams` 构建 `anthropic.BetaMessageNewParams` 后发起流式请求 |
| 修改非流式请求流程 | `complete.go` | 调用 `buildParams` 后发起同步请求 |
| 修改 SSE 事件解析 | `stream.go` | 处理 `content_block_start/delta/stop` 等事件，含 FinishReason 追踪 |
| 修改消息格式映射 | `message.go` | `buildMessages` 函数 |
| 添加支持的模型 | `models.go` | 在 `GetAvailableModels` 列表中追加 |
| 修改工具定义转换 | `tools.go` | `buildTools` 函数 |
| 配置 TopK 等特有参数 | `option.go` | `WithTopK` / `WithBudgetTokens` |
| 启用 debug 日志 | `provider.go` | `WithDebug()` Option 或环境变量 `BAMBOO_DEBUG=1` |
| 配置 prompt caching | `message.go` / `tools.go` | `CacheControl` 标记（通过 `provider.NewEphemeralCacheControl()`） |

## 代码地图

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `Provider` | 类型别名 | provider.go | `BaseProvider[anthropic.Client]` |
| `NewProvider` | 函数 | provider.go | 最简构造函数 |
| `NewProviderWithOptions` | 函数 | provider.go | 完整构造函数 (WithAPIKey/WithBaseURL/WithHeader/WithDebug) |
| `buildParams` | 方法 | params.go | Chat/Complete 共享参数构建入口 |
| `handleStreamEvent` | 方法 | stream.go | SSE 事件分发（含 `finishReason *provider.FinishReason` 参数） |
| `contentBlockStart` | 方法 | stream.go | 内容块开始事件处理（text/thinking/tool_use） |
| `contentMessageDelta` | 方法 | stream.go | 消息增量事件 — 提取 usage 和 stop_reason |
| `contentMessageStop` | 方法 | stream.go | 消息结束事件 — 携带 FinishReason |
| `mapFinishReason` | 函数 | stream.go | Anthropic stop_reason → provider.FinishReason 映射 |

## 约定

- **参数构建集中化** — `params.go` 的 `buildParams` 是 Chat 和 Complete 的共享参数构建入口，确保流式和非流式路径参数一致，避免重复逻辑
- **原生 BlockStart 支持** — Anthropic 协议原生提供 `content_block_start` 事件，直接映射为 `NewBlockStartDelta`
- **Thinking BlockStart 优先** — thinking 类型的 `content_block_start` 先发出 `NewBlockStartDelta("thinking")`，再附带 `NewThinkingDelta`（如有初始内容），确保 StreamConverter 正确开启 thinking block
- **Thinking 非流式提取** — `complete.go` 处理 `"thinking"` 类型的 content block，提取 `block.AsThinking().Thinking` 填充到 `CompletionResult.Thinking`
- **Thinking 配置映射** — `ThinkingConfig.Effort` → `anthropic.BetaThinkingConfigAdaptiveParam` (adaptive thinking 模式)
- **FinishReason 跨事件追踪** — `message_delta` 事件提取 `stop_reason` 并映射为 `provider.FinishReason`，通过指针传递给 `message_stop` 事件，最终填充到 `StreamEvent.FinishReason`
- **ResponseFormat best-effort 适配** — Anthropic 不原生支持 ResponseFormat；当 `config.ResponseFormat == "json_object"` 时，注入系统提示 `"Respond with valid JSON only."` 作为替代方案，并输出 debug 日志
- **ParallelToolCalls 不支持** — Anthropic 不支持 `parallel_tool_calls` 参数，当设置时仅输出 debug 日志，不报错
- **TopK 透传** — 通过 `AnthropicMessagesOption` 的 `WithTopK` 设置，合并到 ProviderExtra 后使用 `param.NewOpt(int64)` 包装
- **ToolChoice 字符串模式** — 支持 `"auto"` / `"any"` / `"required"` / `"forced"` / `"none"` 多种字符串值，映射到 `OfAuto` / `OfAny` / `OfNone`
- **消息角色映射** — `RoleUser→NewBetaUserMessage`, `RoleAssistant→支持 text+tool_use blocks`, `RoleTool→NewBetaToolResultBlock`
- **UserAgent 统一** — 构造函数中通过 `option.WithHeader("User-Agent", provider.GetUserAgent())` 设置
- **Prompt Caching 原生支持** — Anthropic 是唯一使用显式缓存断点的 Provider；`Message.CacheControl` / `Tool.CacheControl` / `ChatConfig.SystemCacheControl` 通过 `provider.NewEphemeralCacheControl()` 创建标记，直接映射到 SDK 的 cache_control 字段
- **Debug 日志** — 通过 `WithDebug()` Option 或环境变量 `BAMBOO_DEBUG=1` 启用；启用后调用 `provider.SetDebug(true)`，请求前输出 Provider 类型、端点、headers（敏感字段脱敏）和 body（长文本截断）

## 反模式

- **禁止** 在 `chat.go` 和 `complete.go` 中重复构建参数逻辑 — 必须统一调用 `params.go` 的 `buildParams`
- **禁止** 在 `stream.go` 中遗漏 `content_block_start` 的 text/thinking 类型处理 — text 必须返回 `NewBlockStartDelta("text")`，thinking 必须先返回 `NewBlockStartDelta("thinking")`
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtraFloat64` / `GetExtraAny` 等 helper
- **禁止** 在 `message_stop` 事件中重复提取 stop_reason — stop_reason 在 `message_delta` 中提取，通过指针传递

## 调试路径

1. 参数构建错误 → 检查 `params.go` 的 `buildParams` 是否正确映射所有 `ChatConfig` 字段
2. 流式输出异常 → 检查 `stream.go` 的 `handleStreamEvent` 是否正确分发事件类型
3. 消息格式错误 → 检查 `message.go` 的 `buildMessages` 是否正确处理 `RoleAssistant` 的 tool_calls
4. Thinking 不生效 → 确认 `ThinkingConfig.Effort` 已设置为 low/medium/high，检查 `buildParams` 中的映射
5. Thinking 内容丢失 → 检查 `contentBlockStart` 是否正确发出 `NewBlockStartDelta("thinking")`；检查 `complete.go` 是否处理 `"thinking"` content block
6. FinishReason 缺失 → 检查 `contentMessageDelta` 是否从 `msgDelta.Delta.StopReason` 提取并映射
7. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成 `BetaToolUnionParam`
8. 认证/连接问题 → 检查 `provider.go` 中 Options 是否正确应用到 `sdkOpts`
9. 缓存未命中 → 检查 `CacheControl` 标记是否正确设置在 system/messages/tools 上，确认 TTL 值
10. 请求参数不确定 → 启用 `WithDebug()` 或设置 `BAMBOO_DEBUG=1`，查看实际发送的 headers 和 body

## 引用

无子级 AGENTS.md

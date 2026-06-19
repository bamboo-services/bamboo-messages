# anthropic 知识库

## 概述

Anthropic Messages 协议适配器，将 Anthropic Claude 系列模型的原生协议转换为统一的 `provider.Provider` 接口。基于 `anthropic-sdk-go` v1.27+ 构建，支持 Claude 4 和 Claude 3 系列模型。

## 目录结构

```text
provider/anthropic/
├── provider.go      # Provider 构造函数 + Options 模式 + 类型别名
├── params.go        # buildParams — 共享参数构建（Chat/Complete 统一入口）
├── chat.go          # 流式对话实现 (Chat/ChatWithSystem)
├── complete.go      # 非流式对话实现 (Complete/CompleteWithSystem)
├── stream.go        # SSE → StreamEvent 转换
├── message.go       # 消息格式双向转换 (buildMessages)
├── models.go        # 模型常量 + GetAvailableModels
├── option.go        # AnthropicMessagesOption + WithTopK/WithBudgetTokens
├── tools.go         # 工具定义转换 (buildTools)
├── params_test.go   # buildParams 单元测试
└── provider_test.go # 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewProvider(apiKey)` 或 `NewProviderWithOptions(opts...)` |
| 修改参数构建逻辑 | `params.go` | `buildParams` — Chat/Complete 共享的参数构建入口 |
| 修改流式请求流程 | `chat.go` | 调用 `buildParams` 构建 `anthropic.BetaMessageNewParams` 后发起流式请求 |
| 修改非流式请求流程 | `complete.go` | 调用 `buildParams` 后发起同步请求 |
| 修改 SSE 事件解析 | `stream.go` | 处理 `content_block_start/delta/stop` 等事件 |
| 修改消息格式映射 | `message.go` | `buildMessages` 函数 |
| 添加支持的模型 | `models.go` | 在 `GetAvailableModels` 列表中追加 |
| 修改工具定义转换 | `tools.go` | `buildTools` 函数 |
| 配置 TopK 等特有参数 | `option.go` | `WithTopK` / `WithBudgetTokens` |

## 约定

- **参数构建集中化** — `params.go` 的 `buildParams` 是 Chat 和 Complete 的共享参数构建入口，确保流式和非流式路径参数一致，避免重复逻辑
- **原生 BlockStart 支持** — Anthropic 协议原生提供 `content_block_start` 事件，直接映射为 `NewBlockStartDelta`，无需合成
- **Thinking 配置映射** — `ThinkingConfig.Effort` → `anthropic.BetaThinkingConfigAdaptiveParam` (adaptive thinking 模式)
- **TopK 透传** — 通过 `AnthropicMessagesOption` 的 `WithTopK` 设置，合并到 ProviderExtra 后使用 `param.NewOpt(int64)` 包装
- **ToolChoice 字符串模式** — 支持 `"auto"` / `"any"` / `"required"` / `"forced"` / `"none"` 多种字符串值，映射到 `OfAuto` / `OfAny` / `OfNone`
- **消息角色映射** — `RoleUser→NewBetaUserMessage`, `RoleAssistant→支持 text+tool_use blocks`, `RoleTool→NewBetaToolResultBlock`
- **UserAgent 统一** — 构造函数中通过 `option.WithHeader("User-Agent", provider.GetUserAgent())` 设置

## 反模式

- **禁止** 在 `chat.go` 和 `complete.go` 中重复构建参数逻辑 — 必须统一调用 `params.go` 的 `buildParams`
- **禁止** 在 `stream.go` 中遗漏 `content_block_start` 的 text 类型处理 — 必须返回 `NewBlockStartDelta("text")`
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtraFloat64` / `GetExtraAny` 等 helper

## 调试路径

1. 参数构建错误 → 检查 `params.go` 的 `buildParams` 是否正确映射所有 `ChatConfig` 字段
2. 流式输出异常 → 检查 `stream.go` 的 `handleStreamEvent` 是否正确分发事件类型
3. 消息格式错误 → 检查 `message.go` 的 `buildMessages` 是否正确处理 `RoleAssistant` 的 tool_calls
4. Thinking 不生效 → 确认 `ThinkingConfig.Effort` 已设置为 low/medium/high，检查 `buildParams` 中的映射
5. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成 `BetaToolUnionParam`
6. 认证/连接问题 → 检查 `provider.go` 中 Options 是否正确应用到 `sdkOpts`

## 引用

无子级 AGENTS.md

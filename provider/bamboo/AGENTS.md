# bamboo 原生协议 Provider 适配器知识库

## 概述

`provider/bamboo` 是 Bamboo Messages 的 bamboo 原生协议适配器。它面向 bamboo 原生协议端点（`/v1/bamboo`），将统一的 `provider.Provider` 类型转换为 bamboo 原生请求体，并把 bamboo 原生响应 / SSE 流映射回 `provider.CompletionResult` 和 `provider.StreamEvent`。该适配器是 `bamboo/codec` 层中继目标之一，也允许上层业务直接通过 `bamboo` SDK 客户端调用 bamboo 原生端点。

## 目录结构

```text
provider/bamboo/
├── provider.go      # Provider 构造函数 + Options 模式 (WithAPIKey/WithBaseURL/WithHeader/WithInterceptor)
├── params.go        # buildParams — Chat/Complete 共享参数构建入口 + buildTools
├── chat.go          # 流式对话实现 (Chat/ChatWithSystem) + SSE 事件循环
├── complete.go      # 非流式对话实现 (Complete/CompleteWithSystem) + 内容块提取
├── stream.go        # SSE → StreamEvent 转换 + 7 值 FinishReason 映射
├── message.go       # 消息格式转换 (buildMessages) — provider.Message ↔ wireMessage
├── models.go        # GetAvailableModels（开放端点，返回空列表）
├── types.go         # 本地 wire DTO，镜像 bamboo facade JSON 形状
├── mock_test.go     # httptest mock server + 测试辅助工具
├── complete_test.go # 非流式对话单元测试
├── stream_test.go   # 流式事件单元测试
└── provider_test.go # 集成测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewProvider(apiKey)` 或 `NewProviderWithOptions(opts...)` |
| 注册请求拦截器 | `provider.go` | `WithInterceptor(fn)` — 在 HTTP Transport 层改写已序列化的请求 body |
| 修改参数构建逻辑 | `params.go` | `buildParams` — Chat/Complete 共享的参数构建入口 |
| 修改流式请求流程 | `chat.go` | 调用 `buildParams` 后设置 `Stream=true`，再发起 SSE 请求 |
| 修改非流式请求流程 | `complete.go` | 调用 `buildParams` 后发起同步 POST `/v1/bamboo` |
| 修改 SSE 事件解析 | `stream.go` | 处理 `message_start/content_block_start/delta/stop/message_delta/message_stop/ping/error` |
| 修改消息格式映射 | `message.go` | `buildMessages` 函数，注意 RoleTool 合并到前一条 user 消息 |
| 修改停止原因映射 | `stream.go` | `mapBambooFinishReason` 7 值映射 |
| 启用 debug 日志 | `provider.go` | 环境变量 `BAMBOO_DEBUG=1/true/on` |
| 配置自定义端点 | `provider.go` | `WithBaseURL(url)` — 必须为完整 API 根路径（见 BaseURL 配置说明） |

## 代码地图

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `Provider` | 结构体 | `provider.go` | 持有 `*provider.HTTPClient` 的 bamboo 原生协议适配器 |
| `Option` | 函数类型 | `provider.go` | `func(*config)` — Provider 配置选项 |
| `config` | 结构体 | `provider.go` | 运行时配置（APIKey、BaseURL、Headers、Interceptors） |
| `NewProvider` | 函数 | `provider.go` | 最简构造函数 |
| `NewProviderWithOptions` | 函数 | `provider.go` | 完整构造函数（WithAPIKey/WithBaseURL/WithHeader/WithInterceptor） |
| `WithAPIKey` / `WithBaseURL` / `WithHeader` / `WithInterceptor` | 函数 | `provider.go` | 配置选项 |
| `applyOptions` | 函数 | `provider.go` | 将 Option 列表应用到默认 `config` |
| `GetProviderType` | 方法 | `provider.go` | 返回 `provider.ProviderBamboo` |
| `GetAvailableModels` | 方法 | `models.go` | 返回空列表（开放端点无固定模型白名单） |
| `Complete` / `CompleteWithSystem` | 方法 | `complete.go` | 非流式对话（后者带 system prompt） |
| `Chat` / `ChatWithSystem` | 方法 | `chat.go` | 流式对话（后者带 system prompt） |
| `buildParams` | 函数 | `params.go` | Chat/Complete 共享参数构建入口 |
| `buildWireRequestConfig` | 函数 | `params.go` | 将 `provider.ChatConfig` 转换为 `wireRequestConfig` |
| `buildTools` | 函数 | `params.go` | `provider.Tool` → 扁平 `wireTool` |
| `marshalInputSchema` | 函数 | `params.go` | 将参数 map 序列化为 `json.RawMessage`，空值返回 `{}` |
| `buildMessages` | 函数 | `message.go` | `provider.Message` 列表 → `wireMessage` 列表 |
| `buildWireContentBlocks` | 函数 | `message.go` | 从 `provider.Message` 构建 7 种内容块 |
| `buildToolResultBlock` | 函数 | `message.go` | 从 `RoleTool` 消息构建 `tool_result` 块 |
| `handleStreamEvent` | 方法 | `stream.go` | SSE 事件分发入口 |
| `handleMessageStart` | 方法 | `stream.go` | `message_start` → `NewUsageDeltaWithCache` |
| `handleContentBlockStart` | 方法 | `stream.go` | `content_block_start` → `BlockStartDelta`（tool_use 用 `NewBlockStartDeltaWithID`） |
| `handleContentBlockDelta` | 方法 | `stream.go` | `content_block_delta` → text/thinking/input_json/signature delta |
| `handleContentBlockStop` | 方法 | `stream.go` | `content_block_stop` → `BlockStopDelta`（不可返回 nil） |
| `handleMessageDelta` | 方法 | `stream.go` | 提取 `stop_reason` 并映射为 `FinishReason` |
| `handleMessageStop` | 方法 | `stream.go` | `message_stop` → `StreamTypeStop` |
| `handleError` | 方法 | `stream.go` | `error` → `StreamTypeError`（使用 `pkgErrors.BambooError`） |
| `mapBambooFinishReason` | 函数 | `stream.go` | 7 值停止原因映射到 `provider.FinishReason` |
| `formatBambooError` | 函数 | `chat.go` | 将 HTTP 错误响应体格式化为可读字符串 |

## 约定

- **参数构建集中化** — `params.go` 的 `buildParams` 是 Chat 和 Complete 的共享入口；`Stream` 字段由 `chat.go` 在 `buildParams` 之后设置，保证非流式默认不传 `stream`。
- **统一 HTTP 传输** — 通过 `provider.NewHTTPClient` 创建 `*provider.HTTPClient`，认证使用 `Authorization: Bearer` 模式（authPrefix 为 `"Bearer "`，末尾含空格）；流式响应通过 `provider.NewSSEScanner` 解析。
- **本地 wire DTO 镜像 facade JSON** — `types.go` 定义与 `bamboo` 门面层 JSON 形状 1:1 对应的独立 wire 类型，避免 `provider/bamboo` 导入上层 `bamboo` 包造成循环依赖。
- **消息角色降级** — `provider.RoleSystem` 在 `buildMessages` 中降级为 `user`（系统提示应通过 system 参数传递）；`provider.RoleTool` 合并到前一条 user 消息的 `tool_result` 块中（Anthropic 风格）。
- **工具定义扁平结构** — `wireTool` 为 `{name, description, input_schema, cache_control}`，不同于 OpenAI 的 `{type, function}` 嵌套。`input_schema` 为 `json.RawMessage` 且不带 `omitempty`，空值序列化为 `{}`。
- **CacheControl 序列化为 RawMessage** — `wireContentBlock.CacheControl` 和 `wireTool.CacheControl` 使用 `json.RawMessage` 存储序列化后的 `provider.CacheControl`，保持与 facade 的 JSON 兼容。
- **7 值 FinishReason 映射** — `end_turn` → `stop`、`max_tokens` → `length`、`tool_use` → `tool_calls`、`stop_sequence` → `stop`、`pause_turn` → `pause_turn`、`refusal` → `refusal`、`server_tool_use` → `server_tool_use`，其余默认 `stop`。
- **content_block_stop 必须发出 BlockStop** — `handleContentBlockStop` 永远返回非空的 `BlockStopDelta`（有索引 / 无索引），不可返回 nil。
- **错误使用 `pkgErrors.BambooError`** — 所有 `StreamEvent.Err` 字段均为 `*pkgErrors.BambooError`；不引入 `internal/xerr`。
- **无默认 BaseURL** — 原生协议没有官方默认端点，必须显式设置 `WithBaseURL`；空 BaseURL 被接受，但请求时由 HTTP 层失败安全处理。
- **构造时不 panic** — `NewProviderWithOptions` 对空 BaseURL 保持容忍，不会 panic。
- **拦截器 Transport 注入** — 通过 `provider.NewHTTPClient` 传入 `cfg.interceptors`；非空时由 `NewInterceptorHTTPClient` 包装 Transport，无拦截器时使用标准库默认 client。

## 反模式

- **禁止** `provider/bamboo` 直接 import `github.com/bamboo-services/bamboo-messages/bamboo` — 必须通过 `types.go` 的本地 wire 类型镜像 JSON。
- **禁止** 在 `chat.go` 和 `complete.go` 中重复构建参数逻辑 — 必须统一调用 `params.go` 的 `buildParams`。
- **禁止** 在 `stream.go` 中遗漏 `content_block_stop` 的处理 — 必须返回 `NewBlockStopDelta` / `NewBlockStopDeltaNoIndex`。
- **禁止** `handleStreamEvent` 返回裸 error 或 panic — 错误应包装为 `pkgErrors.BambooError` 并作为 `StreamTypeError` 事件发送。
- **禁止** 在 `handleContentBlockStart` 中对 `tool_use` 使用 `NewToolCallDelta` — 应使用 `NewBlockStartDeltaWithID`。
- **禁止** 在 `handleMessageStop` 中重新解析 stop_reason — 原因由 `message_delta` 提取并通过指针传递。
- **禁止** 保留 `Chat/Complete/ChatWithSystem/CompleteWithSystem` 的 stub 实现 — 这些均为接口方法，必须提供完整实现。
- **禁止** 在 Provider 构造函数中 panic — 即使缺少 BaseURL 也应返回 `Provider` 实例，让请求时自然失败。

## 调试路径

1. 参数构建错误 → 检查 `params.go` 的 `buildParams` 是否正确映射 `ChatConfig` 字段，确认 `buildTools` 是否过滤非 `function` 类型工具。
2. 流式输出异常 → 检查 `stream.go` 的 `handleStreamEvent` 是否正确分发事件类型；确认 `content_block_stop` 未返回 nil。
3. 消息格式错误 → 检查 `message.go` 的 `buildMessages` 是否正确处理 `RoleTool` 合并和 `RoleSystem` 降级。
4. Thinking 内容丢失 → 检查 `buildWireContentBlocks` 是否把 `ThinkingContent` 写入 `thinking` 块，以及 `complete.go` 是否从 `thinking` 块回填 `CompletionResult.Thinking`。
5. FinishReason 缺失 → 检查 `handleMessageDelta` 是否从 `delta.stop_reason` 提取并映射；`handleMessageStop` 使用指针传递的 finishReason。
6. 工具调用失败 → 检查 `buildTools` 是否生成正确的扁平 `wireTool`（`input_schema` 应为 `json.RawMessage`）。
7. 认证/连接问题 → 检查 `provider.go` 中 `provider.NewHTTPClient` 的 authPrefix 是否为 `"Bearer "`（含末尾空格），以及 BaseURL 是否包含完整 API 根路径。
8. 缓存未命中 → 检查 `CacheControl` 是否通过 `marshalCacheControl` 正确写入 `wireContentBlock` / `wireTool`。
9. 请求参数不确定 → 设置 `BAMBOO_DEBUG=1`，查看实际发送的 headers 和 body。
10. SSE 解析失败 → 确认上游返回的是标准 SSE 帧；`provider.SSEScanner` 内置 GLM 截断容错。
11. 空流挂起 → 检查 `chat.go` 的 `startSent` 降级逻辑：未收到 `message_stop` 时会补发 `StreamTypeStop`。

## BaseURL 配置说明

bamboo 原生协议没有官方默认端点，用户必须显式设置 `WithBaseURL`。`HTTPClient` 将 BaseURL 与路径简单拼接为 `BaseURL + path`，适配器内部请求路径为 `/v1/bamboo`，因此 BaseURL 应到 API 根路径级别（不含 `/v1/bamboo`）。

### 路径要求

BaseURL 应到 API 根路径级别（不含 `/v1`），适配器内部拼接 `/v1/bamboo` 路径。

### 正确示例

| 端点 | BaseURL | 实际请求路径 |
|------|---------|-----------------|
| 自建 bamboo 网关 | `https://gateway.example.com` | `https://gateway.example.com/v1/bamboo` |
| 本地代理 | `http://localhost:8080` | `http://localhost:8080/v1/bamboo` |
| 带路径前缀的网关 | `https://api.example.com/bamboo` | `https://api.example.com/bamboo/v1/bamboo` |

### 注意

- 认证头固定为 `Authorization: Bearer {apiKey}`，`Bearer` 后有一个空格（由 `http_client.go` 的 `prefix + key` 拼接逻辑决定）。
- 若 BaseURL 为空，构造函数不会报错，但后续请求会失败；请始终在配置时设置 `WithBaseURL`。

## 引用

- [provider 父级知识库](../AGENTS.md) — 核心抽象层总览
- [bamboo 原生 codec](../../bamboo/codec/bamboo/AGENTS.md) — bamboo 原生协议恒等变换编解码

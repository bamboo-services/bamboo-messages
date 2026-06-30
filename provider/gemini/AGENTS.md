# gemini 知识库

## 概述

Google Gemini 协议适配器，将 Google Gemini API 转换为统一的 `provider.Provider` 接口。基于 `net/http` + JSON 构建，不依赖外部 SDK，支持 Gemini API 与 Vertex AI 两种后端（默认 Gemini API），支持 Claude / GPT 系列之外第三大主流 AI 协议接入。

## 目录结构

```text
provider/gemini/
├── provider.go      # Provider 构造函数 + Options 模式 (WithAPIKey/WithBaseURL/WithHeader/WithDebug/WithInterceptor) + 类型别名 + 拦截器 HTTPClient 注入
├── params.go        # buildContentConfig — 共享参数构建（Chat/Complete 统一入口）+ mapThinkingConfig/mapToolChoice + MaxTokens 溢出保护 + UserID→Labels
├── chat.go          # 流式对话实现 (Chat/ChatWithSystem) — GenerateContentStream
├── complete.go      # 非流式对话实现 (Complete/CompleteWithSystem) — 含 thinking parts 提取
├── stream.go        # 流式响应 → StreamEvent 转换 + handleStreamEvent + FinishReason 携带
├── stream_test.go   # 流式事件单元测试
├── message.go       # 消息格式双向转换 (buildMessages) — ToolName/ToolCallID 分离映射
├── models.go        # 模型常量 (gemini-2.5 系列等)
├── option.go        # GeminiOption + WithAPIKey/WithBaseURL/WithHeader
├── tools.go         # 工具定义转换 (buildTools)
├── types.go         # Gemini 协议原生请求/响应 DTO
├── audit_test.go    # 工具调用 BlockStart + Thinking BlockStart 审计测试
└── params_audit_test.go  # MaxTokens 溢出 + SafetySettings + UserID + ParallelToolCalls + ResponseFormat 审计测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewProvider(apiKey)` 或 `NewProviderWithOptions(opts...)` |
| 注册请求拦截器 | `provider.go` | `WithInterceptor(fn)` — 在 HTTP Transport 层改写已序列化的请求 body |
| 修改参数构建逻辑 | `params.go` | `buildContentConfig` — Chat/Complete 共享的本地 `generationConfig` 构建入口 |
| 修改流式请求构建 | `chat.go` | `ChatWithSystem` → 通过 `httpClient` 发起 `/v1beta/models/{model}:streamGenerateContent` 流式请求 |
| 修改非流式请求构建 | `complete.go` | `CompleteWithSystem` → 通过 `httpClient` 发起 `/v1beta/models/{model}:generateContent` 同步请求 |
| 修改流式事件解析 | `stream.go` | `handleStreamEvent` 处理 `candidates` / `contents` |
| 修改消息格式映射 | `message.go` | `buildMessages` (provider.Message → `geminiContent`) |
| 修改 Thinking 映射 | `params.go` | `mapThinkingConfig` (effort → `thinkingConfig`) |
| 修改 ToolChoice 映射 | `params.go` | `mapToolChoice` (字符串 → `functionCallingConfig`) |
| 添加支持的模型 | `models.go` | 在 `GetAvailableModels` 中追加 Gemini 模型常量 |
| 修改工具定义转换 | `tools.go` | `buildTools` → `geminiTool` (FunctionDeclaration) |
| 启用 debug 日志 | `provider.go` | `WithDebug()` Option 或环境变量 `BAMBOO_DEBUG=1` |

## 代码地图

| 符号 | 类型 | 位置 | 作用 |
|------|------|------|------|
| `Provider` | 结构体 | provider.go | 持有 `*provider.HTTPClient` 的 Gemini 协议适配器实现 |
| `Option` | 函数类型 | provider.go | `func(*config)` — Provider 配置选项 |
| `GeminiProviderOption` | 函数类型 | option.go | `func(*geminiRequestConfig)` — 请求级配置选项 |
| `ModelGemini25Flash` / `ModelGemini25Pro` / `ModelGemini20Flash` | 常量 | models.go | Gemini 模型常量 |
| `WithInterceptor` | 函数 | provider.go | 注册请求拦截器（转发到 `provider.WithInterceptor`） |
| `WithTopK` / `WithSafetySettings` | 函数 | option.go | GeminiProviderOption 请求级参数 |
| `buildContentConfig` | 方法 | params.go | Chat/Complete 共享参数构建入口（含 MaxTokens 溢出保护） |
| `mapThinkingConfig` | 函数 | params.go | Effort → `thinkingConfig` 映射 |
| `mapToolChoice` | 函数 | params.go | 字符串 → `functionCallingConfig` 映射 |
| `handlePart` | 方法 | stream.go | 处理单个 Part（text/thinking/function_call）— 工具调用不再发 BlockStart |
| `handleCandidate` | 方法 | stream.go | 处理 Candidate + FinishReason |
| `mapFinishReason` | 函数 | stream.go | Gemini `finishReason` 字符串 → provider.FinishReason 映射 |
| `buildToolMessage` | 方法 | message.go | 构建工具响应 — 优先使用 ToolName，回退到 ToolCallID |

## 约定

- **参数构建集中化** — `params.go` 的 `buildContentConfig` 是 Chat 和 Complete 的共享参数构建入口（与其他适配器的 `buildParams` 规则一致），确保流式和非流式路径参数一致
- **统一 HTTP 传输** — 通过 `provider.NewHTTPClient` 创建 `*provider.HTTPClient`，认证使用 `x-goog-api-key` 模式；流式响应通过 `provider.NewSSEScanner` 解析 SSE 帧
- **本地 DTO 模型** — 请求/响应结构通过 `types.go` 本地定义（`generateContentRequest`、`generateContentResponse`、`geminiContent` 等），不依赖外部 SDK
- **MaxTokens 溢出保护** — `config.MaxTokens`（int64）转 `generationConfig.MaxOutputTokens`（int）时，超过 `math.MaxInt32` 的值被截断为 `math.MaxInt32`，避免静默溢出导致负数或截断值
- **UserID → Labels 映射** — Gemini 无原生 UserID 字段，`config.UserID` 存入请求 DTO 的 `generationConfig.Labels["user_id"]`，并在 debug 模式下输出日志
- **BlockStart 合成** — Gemini 没有原生 `content_block_start` 事件，通过 `textBlockStarted` / `thinkingBlockStarted` 两个独立布尔标志在首个文本/推理增量前合成
- **工具调用不发 BlockStart** — `handlePart` 为 FunctionCall 仅发出 `ToolCallDelta` + `ToolCallDeltaData`，不再发出 `BlockStartDeltaWithID("tool_use")`。block 生命周期由 StreamConverter 统一管理，与 Anthropic/OpenAI 适配器保持一致
- **双 Block 状态追踪** — `textBlockStarted` 和 `thinkingBlockStarted` 独立追踪，互不干扰（与 OpenAI 适配器模式一致）
- **Thinking 非流式提取** — `complete.go` 遍历 `candidate.Content.Parts`，`part.Thought == true` 的内容收集到 `CompletionResult.Thinking`
- **ToolName/ToolCallID 分离** — `buildToolMessage` 优先使用 `msg.ToolName`（函数名），回退到 `msg.ToolCallID`；构建 `functionResponse` 时同时设置 `ID`（= ToolCallID）和 `Name`（= ToolName/ToolCallID），保留完整 ID 信息
- **FinishReason 流式携带** — `handleCandidate` 在 `FinishReason` 非空且非 Unspecified 时，通过 `mapFinishReason` 映射并填充到 `StreamEvent.FinishReason`
- **Gemini HTTP 后端** — 默认 BaseURL 为 `https://generativelanguage.googleapis.com`，请求路径为 `/v1beta/models/{model}:generateContent`（非流式）或 `/v1beta/models/{model}:streamGenerateContent`（流式）
- **Options 模式** — `WithAPIKey` / `WithBaseURL` / `WithHeader`，与其他适配器保持一致的 Functional Options 接口
- **UserAgent 统一** — 由 `provider.HTTPClient` 自动注入统一 `User-Agent: BM-SDK/{version}`
- **ProviderType** — `GetProviderType()` 返回 `"gemini"`
- **ThinkingLevel 映射** — `Effort: low/medium/high` → 请求 DTO `thinkingConfig.IncludeThoughts: true` + `ThinkingLevel"low"/"medium"/"high"`；`none` → 不设置 thinkingConfig
- **ToolChoice 映射** — `auto→AUTO`、`none→NONE`、`required/forced/any→ANY`
- **ResponseFormat 映射** — `"json_object"` → 请求 DTO `generationConfig.ResponseMIMEType: "application/json"`
- **TopK / SafetySettings / CachedContent** — 通过 ProviderExtra 提取（Gemini 特有参数），合并到请求 DTO
- **ParallelToolCalls 不支持** — Gemini 不支持此参数，当设置时仅输出 debug 日志，不报错
- **Debug 日志** — 通过 `WithDebug()` Option 或环境变量 `BAMBOO_DEBUG=1` 启用；构造函数中检测到 debug 标志后调用 `provider.SetDebug(true)`，请求前通过 `httpClient.DoWithDebug` 输出 Provider 类型、端点、headers（敏感字段脱敏）和 body（长文本截断）
- **拦截器 Transport 注入** — 构造函数中调用 `provider.NewHTTPClient` 时传入 `cfg.interceptors`；非空时由 `NewInterceptorHTTPClient` 包装 Transport，无拦截器时使用标准库默认 client

## 反模式

- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — 两者必须独立追踪
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtra*` helper
- **禁止** 在 `chat.go` 和 `complete.go` 中重复构建参数逻辑 — 必须统一调用 `params.go` 的 `buildContentConfig`
- **禁止** 在 `handlePart` 中为 FunctionCall 发送 BlockStartDelta — block 生命周期由 StreamConverter 统一管理
- **禁止** 在构建工具响应时丢失 `ToolCallID` — 必须同时设置 `functionResponse.ID`（= ToolCallID）和 `Name`（= ToolName/ToolCallID）

## 调试路径

1. 参数构建错误 → 检查 `params.go` 的 `buildContentConfig` 是否正确映射所有字段
2. MaxTokens 异常 → 检查 `buildContentConfig` 中 int64→int32 溢出保护是否生效（值超过 MaxInt32 时应截断）
3. 流式输出异常 → 检查 `stream.go` 的 `handleStreamEvent` 是否正确提取 `candidates[0].content.parts`
4. BlockStart 重复或缺失 → 检查 `textBlockStarted` / `thinkingBlockStarted` 状态管理
5. 工具调用 BlockStart 多余 → 确认 `handlePart` 不再为 FunctionCall 发送 BlockStart（由 StreamConverter 处理）
6. FinishReason 缺失 → 检查 `handleCandidate` 是否在 FinishReason 非空时正确映射
7. 工具响应 name 错误 → 检查 `buildToolMessage` 是否正确使用 `msg.ToolName`（优先）和 `msg.ToolCallID`（回退）
8. Thinking 内容丢失 → 非流式：检查 `complete.go` 是否正确处理 `part.Thought == true`
9. UserID 丢失 → 检查 `buildContentConfig` 中 UserID → Labels 映射
10. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成 `geminiTool` 结构
11. 认证失败 → 确认 API Key 有效，或检查 `WithBaseURL` 是否指向正确的 Gemini 兼容端点
12. 模型不可用 → 检查 `models.go` 的模型常量是否与 Gemini API 当前支持的版本匹配
13. Thinking 配置不生效 → 检查 `mapThinkingConfig` 中 effort 到 `thinkingConfig.ThinkingLevel` 的映射
14. 请求参数不确定 → 启用 `WithDebug()` 或设置 `BAMBOO_DEBUG=1`，查看实际发送的 headers 和 body

## 引用

无子级 AGENTS.md

# gemini 知识库

## 概述

Google Gemini 协议适配器，将 Google Gemini API（`google.golang.org/genai`）转换为统一的 `provider.Provider` 接口。支持 Gemini API 与 Vertex AI 两种后端（默认 Gemini API），支持 Claude / GPT 系列之外第三大主流 AI 协议接入。

## 目录结构

```text
provider/gemini/
├── provider.go      # Provider 构造函数 + Options 模式 + 类型别名
├── chat.go          # 流式对话实现 (Chat/ChatWithSystem) — GenerateContentStream
├── complete.go      # 非流式对话实现 (Complete/CompleteWithSystem) — GenerateContent
├── stream.go        # 流式响应 → StreamEvent 转换 + handleStreamEvent
├── message.go       # 消息格式双向转换 (buildMessages/buildContentConfig)
├── models.go        # 模型常量 (gemini-2.5 系列等)
├── option.go        # GeminiOption + WithAPIKey/WithBaseURL/WithHeader
├── tools.go         # 工具定义转换 (buildTools)
└── (provider_test.go 待补充)
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 创建 Provider 实例 | `provider.go` | `NewProvider(apiKey)` 或 `NewProviderWithOptions(opts...)` |
| 修改流式请求构建 | `chat.go` | `ChatWithSystem` → `GenerateContentStream` |
| 修改非流式请求构建 | `complete.go` | `CompleteWithSystem` → `GenerateContent` |
| 修改流式事件解析 | `stream.go` | `handleStreamEvent` 处理 `candidates` / `contents` |
| 修改消息格式映射 | `message.go` | `buildMessages` (provider.Message → genai.Content) |
| 添加支持的模型 | `models.go` | 在 `GetAvailableModels` 中追加 Gemini 模型常量 |
| 修改工具定义转换 | `tools.go` | `buildTools` → `genai.Tool` (FunctionDeclaration) |

## 约定

- **BlockStart 合成** — Gemini 没有原生 `content_block_start` 事件，通过 `textBlockStarted` / `thinkingBlockStarted` 两个独立布尔标志在首个文本/推理增量前合成
- **双 Block 状态追踪** — `textBlockStarted` 和 `thinkingBlockStarted` 独立追踪，互不干扰（与 OpenAI 适配器模式一致）
- **genai SDK** — 使用 `google.golang.org/genai` (v1.60+)，`genai.Client` 支持 Gemini API (`BackendGeminiAPI`) 和 Vertex AI (`BackendVertexAI`) 两种后端
- **Options 模式** — `WithAPIKey` / `WithBaseURL` / `WithHeader`，与其他适配器保持一致的 Functional Options 接口
- **UserAgent 统一** — 构造函数中通过 `clientCfg.HTTPOptions.Headers.Set("User-Agent", provider.GetUserAgent())` 设置
- **ProviderType** — `GetProviderType()` 返回 `"gemini"`

## 反模式

- **禁止** 将 `textBlockStarted` 和 `thinkingBlockStarted` 混用 — 两者必须独立追踪
- **禁止** 裸类型断言访问 `ProviderExtra` — 必须使用 `provider.GetExtra*` helper
- **禁止** 在 `genai.NewClient` 返回 error 时 panic — 构造函数做防御性处理，返回零值 Client

## 调试路径

1. 流式输出异常 → 检查 `stream.go` 的 `handleStreamEvent` 是否正确提取 `candidates[0].content.parts`
2. BlockStart 重复或缺失 → 检查 `textBlockStarted` / `thinkingBlockStarted` 状态管理
3. 工具调用失败 → 检查 `tools.go` 的 `buildTools` 是否正确生成 `genai.Tool`
4. 认证失败 → 确认 API Key 有效，或检查 `WithBaseURL` 是否指向正确的 Gemini 兼容端点
5. 模型不可用 → 检查 `models.go` 的模型常量是否与 Gemini API 当前支持的版本匹配

## 引用

无子级 AGENTS.md

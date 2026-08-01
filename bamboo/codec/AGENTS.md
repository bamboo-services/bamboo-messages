# codec 知识库

## 概述

N-to-N 协议编解码层，负责外部协议格式（OpenAI / Anthropic / Responses / Gemini）与 Bamboo 内部统一类型之间的双向转换。Codec 只做格式翻译，不调用 Provider；实际的 Provider 调用由上层 `bamboo/relay` 负责。

## 目录结构

```text
bamboo/codec/
├── codec.go                # Codec 接口 + StreamSerializer 接口 + FormatType 常量
├── types.go                # RelayRequest 统一请求中间表示
├── registry.go             # 全局 Codec 注册变量 + Get() 查找函数
├── errors.go               # 包注释（错误处理统一使用 pkg/errors.BambooError，Codec 层不再单独定义错误类型）
├── anthropic/              # Anthropic Messages 协议编解码
│   ├── codec.go            # 全局 Codec 实例 + init() 注册
│   ├── request.go          # ParseRequest: Anthropic JSON → RelayRequest（含 document 块解析、tool_choice forced_tool_name）
│   ├── response.go         # SerializeResponse: bamboo.Response → Anthropic JSON（含 DocumentBlock 序列化）
│   ├── stream.go           # StreamSerializer: StreamEvent → Anthropic SSE 帧
│   ├── error.go            # SerializeError: error → Anthropic 错误 JSON
│   ├── request_test.go     # 请求解析单元测试
│   ├── request_audit_test.go  # N-to-N 转换安全性审计测试
│   ├── response_test.go    # 响应序列化单元测试
│   └── stream_test.go      # 流式序列化单元测试（SSE 帧生成）
├── openai/                 # OpenAI Chat Completions 协议编解码（结构同 anthropic/）
│   ├── codec.go / request.go / response.go / stream.go / error.go
│   ├── request_test.go / response_test.go
│   ├── request_audit_test.go  # N-to-N 转换安全性审计测试
│   └── stream_test.go      # 流式序列化单元测试（SSE 帧生成）
├── responses/              # OpenAI Responses 协议编解码（结构同 anthropic/）
│   ├── codec.go / request.go / response.go / stream.go / error.go
│   ├── request_test.go / response_test.go
│   ├── request_audit_test.go  # N-to-N 转换安全性审计测试（含 input_image/input_file、metadata 测试）
│   └── usage_regression_test.go # Usage 透传回归测试
└── gemini/                 # Google Gemini 协议编解码（结构同 anthropic/）
    ├── codec.go / request.go / response.go / stream.go / error.go
    ├── request_test.go / response_test.go
    ├── request_audit_test.go  # N-to-N 转换安全性审计测试（含 thinkingConfig、model、IsStream 测试）
    └── safety_settings_audit_test.go  # safety_settings 类型转换审计测试
└── bamboo/                 # bamboo 原生协议编解码（identity transform）
    ├── codec.go            # 全局 Codec 实例 + init() 注册
    ├── request.go          # ParseRequest: bamboo 信封 → RelayRequest
    ├── response.go         # SerializeResponse: json.Marshal(*bamboo.Response)
    ├── stream.go           # StreamSerializer: 直接 json.Marshal(StreamEvent) + Anthropic SSE 帧
    ├── error.go            # SerializeError: 提取 *bamboo.BambooError 字段
    ├── request_test.go     # 请求解析单元测试
    ├── response_test.go    # 响应序列化单元测试
    ├── stream_test.go      # 流式序列化单元测试
    └── error_test.go       # 错误序列化单元测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解 Codec 接口 | `codec.go` | `Codec` 接口（5 方法）+ `StreamSerializer` 接口（2 方法） |
| 添加新协议格式 | 子包目录 | 新建子包，实现 Codec + StreamSerializer，在 `init()` 中注册到全局变量 |
| 理解请求中间表示 | `types.go` | `RelayRequest` — 解析后的统一请求结构 |
| 查找已注册 Codec | `registry.go` | `Get(format)` 函数返回 `(Codec, error)` + 4 个全局 Codec 变量 |
| 错误处理 | `errors.go` | 统一使用 `pkg/errors.BambooError`，Codec 层不再单独定义错误类型 |
| 修改 Anthropic 编解码 | `anthropic/` | `request.go`(解析) / `response.go`(序列化) / `stream.go`(流式) |
| 修改 OpenAI 编解码 | `openai/` | 结构与 `anthropic/` 完全一致 |
| 修改 Responses 编解码 | `responses/` | 结构与 `anthropic/` 完全一致 |
| 修改 Gemini 编解码 | `gemini/` | 结构与 `anthropic/` 完全一致 |
| 修改 bamboo 原生编解码 | `bamboo/` | identity transform：`request.go`（解析） / `response.go`（序列化） / `stream.go`（流式） / `error.go`（错误） |
| 测试 OpenAI 流式序列化 | `openai/stream_test.go` | SSE 帧生成的单元测试 |
| 查看安全性审计测试 | `*/request_audit_test.go` | N-to-N 转换安全性审计（P1/P2 级问题回归测试） |
| 查看 Gemini safety_settings 审计 | `gemini/safety_settings_audit_test.go` | safety_settings 类型转换验证 |

## 约定

- **Codec 无状态** — `Codec` 接口本身无状态，可安全并发使用；有状态操作通过 `NewSerializer()` 创建独立的 `StreamSerializer` 实例
- **init() 自动注册** — 每个格式子包在 `init()` 中将自身 Codec 赋值到 `registry.go` 的全局变量，只要 import 了对应子包即自动注册
- **RelayRequest 中间表示** — 所有外部协议的请求体先解析为 `RelayRequest`（包含 `Messages`/`System`/`Config`/`IsStream`），再由 relay 层交给 Provider 处理
- **错误处理统一** — Codec 层不再单独定义 `CodecError` / `ErrorType`，统一使用 `pkg/errors.BambooError`；每个子包的 `SerializeError` 负责将错误映射为对应协议的错误响应格式
- **子包结构一致** — 四个格式子包（anthropic / openai / responses / gemini）内部文件分工完全一致：`codec.go` + `request.go` + `response.go` + `stream.go` + `error.go`
- **bamboo 原生 codec 是恒等变换** — `bamboo/codec/bamboo` 的 `ParseRequest` 直接 `json.Unmarshal` 原生 `BambooMessage` / `RequestConfig` 到 `RelayRequest`，`SerializeResponse` 直接 `json.Marshal(*bamboo.Response)`，`Serialize` 直接 `json.Marshal(StreamEvent)` 并封装为 Anthropic 风格 SSE 帧（`event: ...\ndata: ...\n\n`），不引入任何中间 DTO
- **bamboo 子包必须 import 别名** — 由于包名 `bamboo` 与门面层 `bamboo` 包名冲突，子包中 `github.com/bamboo-services/bamboo-messages/bamboo` 使用别名 `bmbamboo`，`bamboo/codec` 使用别名 `bmcodec`
- **cache_creation_input_tokens 已知限制** — OpenAI / Responses / Gemini 协议无原生 `cache_creation_input_tokens` 字段，仅映射 `CacheReadInputTokens`；`CacheCreationInputTokens` 在跨协议转换（Anthropic→其他）中会丢失，此为已知限制
- **DeltaSignature 跨协议丢弃** — `signature_delta` 为 Anthropic Extended Thinking 特有的签名增量，OpenAI / Responses / Gemini 协议无对应字段，在流式序列化中静默丢弃（返回 nil）
- **ToolResultBlock 不应出现在响应中** — 所有协议的 `serializeResponse` 对 assistant 响应中的 `ToolResultBlock` 记录警告并跳过；同理 `ImageBlock` / `DocumentBlock` 在不支持的协议响应中也记录警告并跳过
- **Anthropic tool_choice "tool" 类型保留** — `parseToolChoice` 解析 `{type:"tool", name:"xxx"}` 时返回 `"forced"` + `name`，forced tool name 存入 `ProviderExtra["forced_tool_name"]`，避免跨协议转换时丢失
- **Anthropic document 块解析** — `convertContentBlock` 支持 `"document"` 类型，解析为 `DocumentBlock`（含 source 的 type/mediaType/data/url）
- **Responses input_image / input_file 解析** — `parseInputMessage` 支持 `input_image`（→ `ImageBlock`）和 `input_file`（→ `DocumentBlock`，支持 file_id 和 file_data 两种模式）
- **Responses metadata 智能存储** — 当 `metadata` 的所有值都是 string 类型时存入 `config.Metadata`（`map[string]string`）；混合类型回退到 `ProviderExtra["metadata"]`
- **Gemini safety_settings 类型转换** — `convertSafetySettings` 将 `[]geminiSafetySetting` 转换为 `[]*genai.SafetySetting`，确保 codec 层输出与 provider 期望的类型一致，避免 relay 路径上类型断言失败
- **Gemini thinkingConfig 解析** — `generationConfig.thinkingConfig.thinkingBudget` 通过 `mapThinkingBudgetToEffort` 映射为 `ThinkingConfig`（budget≤2048→low、≤8192→medium、>8192→high）
- **Gemini model 在 URL 路径中** — Gemini 的 model 名称在 URL 路径中（如 `/v1beta/models/gemini-2.5-pro:generateContent`），不在请求 body 中，因此 `config.Model` 为空；relay 层需从 URL 路径提取
- **Gemini IsStream 硬编码 false** — Gemini 的流式标识不在 body 中，由 URL 参数 `?alt=sse` 决定，`IsStream` 硬编码为 false；relay 层应根据实际 URL 覆盖
- **Gemini ThinkingBlock 序列化** — `buildResponseParts` 将 `ThinkingBlock` 序列化为 `{text: "...", thought: true}`（此前被忽略）
- **Gemini inlineData / fileData 映射** — `buildInlineDataPart` 将 `ContentSource` 映射为 Gemini part：base64→`{inlineData}`、url→`{fileData}`
- **Gemini ToolResultBlock.ToolName** — 解析 `functionResponse` 时将 `Name` 写入 `ToolResultBlock.ToolName`
- **Responses 流式序列器重大重写** — `responses/stream.go` 完全重写为 `responsesStreamSerializer` 状态机模型，追踪 output_item 生命周期（added/done）、自动注入 `sequence_number` 和 `response_id`、双轨 reasoning（raw `reasoning_text.*` + summary `reasoning_summary_text.*` 并行发射）、支持 `encrypted_content` 透传、完整覆盖 `response.created/output_item.added/content_part.added/output_text.delta/output_text.done/content_part.done/reasoning_text.delta/reasoning_summary_text.delta/reasoning_text.done/reasoning_summary_text.done/function_call_arguments.delta/function_call_arguments.done/output_item.done/response.completed/response.failed` 事件
- **Responses SerializeResponse 签名变更** — `serializeResponse` 返回值从 `[]byte` 变为 `([]byte, error)`，与 Codec 接口保持一致；新增 `EncryptedContent` 和 `StopSequence` 字段支持
- **Responses reasoning item 三槽位语义** — 序列化 ThinkingBlock 时按官方 schema 分工：`content: [{type: "reasoning_text"}]` 承载原始思考全文；`summary: [{type: "summary_text"}]` 承载 `summarizeThinking`（`responses/summary.go`）启发式提取的摘要（首行/首句 + Markdown 剥离 + 超长截断），提取不出则为空数组；`encrypted_content` 仅透传上游签名/加密值（ThinkingBlock.Signature），绝不伪造明文。流式 done 事件（`reasoning_text.done` / `reasoning_summary_text.done`）保持原始全文作为实时展示轨道，最终 item 才做槽位分流
- **Responses reasoning 请求解析优先级** — `parseInput` 的 reasoning 分支优先取 `content`（reasoning_text 原始全文），缺失时回退 `summary`（有损摘要）；`encrypted_content` 解析为 `ThinkingBlock.Signature` 透传，保证 Codex 等客户端多轮回传的加密推理链不断裂
- **Responses assistant 轮次合并** — `parseInput` 将连续的 assistant 侧条目（`reasoning` / `message[assistant]` / `function_call`）合并为**单条** assistant 消息（thinking + text + tool_use blocks 同属一条），遇到 user 侧条目（`message[user]` / `function_call_output`）时结束当前轮次。Chat Completions 语义下单轮 assistant 消息同时携带 reasoning_content + content + tool_calls；若拆分为多条 assistant 消息，仅 reasoning 对应的消息携带 reasoning_content，DeepSeek 等思考模式强校验上游会以 "reasoning_content must be passed back" 拒绝请求，并行工具调用也会被错误拆分为多轮。reasoning item 的 `id` 保留为合并消息的 `ReasoningID`

## 反模式

- **禁止** 在 Codec 层直接调用 Provider — Codec 只做格式转换，Provider 调用由 `bamboo/relay` 负责
- **禁止** 在 `Codec` 接口方法中保存状态 — 有状态逻辑必须通过 `NewSerializer()` 返回的 `StreamSerializer` 实例处理
- **禁止** 忘记在子包 `init()` 中注册全局变量 — 否则 `registry.Get()` 返回 nil Codec
- **禁止** 裸返回 error — 内部错误应包装为 `pkg/errors.BambooError` 以保留错误分类信息
- **禁止** 在 Gemini codec 中将 safety_settings 存为原始 JSON 结构 — 必须转换为 `[]*genai.SafetySetting`，否则 relay→provider 路径类型断言失败导致静默丢弃
- **禁止** 在非 Anthropic 协议中输出 DeltaSignature — 应返回 nil 静默丢弃，避免生成协议不支持的 SSE 帧
- **禁止** 在响应序列化中遗漏 ToolResultBlock/ImageBlock/DocumentBlock 的警告日志 — 不支持的 block 类型必须记录 warning 后跳过，不得静默丢弃
- **禁止** 在 Responses reasoning item 的 `encrypted_content` 中填证明文 — 该字段是服务端加密的不透明 token（官方契约：原样回传、不可伪造），明文会导致真实 OpenAI 上游解密失败；无上游真值时留空

## 调试路径

1. 请求解析失败 → 检查对应子包的 `request.go` 的 `parseRequest` 是否正确处理所有字段
2. 响应序列化格式错误 → 检查对应子包的 `response.go` 的 `serializeResponse` 输出是否符合目标协议规范
3. 流式 SSE 帧错误 → 检查对应子包的 `stream.go` 的 `Serialize` 和 `Flush` 是否正确生成 SSE 数据帧
4. `Get(format)` 返回 nil → 检查调用方是否 import 了对应格式的子包（触发 `init()` 注册）
5. 错误响应格式不匹配 → 检查对应子包的 `error.go` 的 `serializeError` 输出
6. 缓存字段未透传 → 检查 `request.go` 解析时是否提取了 `cache_control` 字段，`response.go` 序列化时是否输出 `cache_creation_input_tokens` / `cache_read_input_tokens`
7. cache_creation_input_tokens 丢失 → OpenAI / Responses / Gemini 无原生 cache_creation 字段，Anthropic→其他转换时此字段丢失为已知限制
8. forced tool name 丢失 → Anthropic `tool_choice:{type:"tool", name:"xxx"}` 的 name 已存入 `ProviderExtra["forced_tool_name"]`，适配器需从此处读取
9. Gemini safety_settings 不生效 → 检查 `ProviderExtra["safety_settings"]` 的类型是否为 `[]*genai.SafetySetting`（而非原始 JSON 结构体）
10. Gemini thinkingConfig 未解析 → 检查 `generationConfig.thinkingConfig.thinkingBudget` 是否存在，`mapThinkingBudgetToEffort` 会根据 budget 大小映射 effort
11. Gemini model 为空 → model 在 URL 路径中，codec 层无法获取，relay 层需从 URL 提取
12. Responses metadata 丢失 → 全 string metadata 现存入 `config.Metadata`；混合类型存入 `ProviderExtra["metadata"]`；检查是否读取了正确的字段
13. input_image / input_file 未解析 → Responses codec 现支持 `input_image`（→ ImageBlock）和 `input_file`（→ DocumentBlock），检查 content part 的 type 字段
14. Responses 流式事件缺失 → 检查 `responsesStreamSerializer` 是否正确追踪 output_item 状态（added/done）；检查 `sequence_number` 是否自动递增
15. Responses reasoning 双轨不完整 → 检查 raw `reasoning_text.*` 和 summary `reasoning_summary_text.*` 是否并行发射
16. Responses encrypted_content 丢失 → 检查 `response.output_item.done` 事件是否携带 `encrypted_content` 字段
17. bamboo 请求解析失败 → 检查 `bamboo/request.go` 的 `parseRequest` 是否正确解析 `{messages,system,config,stream}` 信封，确认 `config` 缺失时是否填充零值
18. bamboo SSE 帧格式异常 → 检查 `bamboo/stream.go` 的输出是否为 `event: {type}\ndata: {json}\n\n`（无 `[DONE]` 标记）
19. bamboo 错误响应格式不对 → 检查 `bamboo/error.go` 是否输出 `{"type":"error","error":{"category":"...","message":"...","status_code":...}}`

## 引用

- [bamboo 原生编解码](./bamboo/AGENTS.md) — bamboo 原生协议恒等变换编解码知识库

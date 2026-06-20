# codec 知识库

## 概述

N-to-N 协议编解码层，负责外部协议格式（OpenAI / Anthropic / Responses / Gemini）与 Bamboo 内部统一类型之间的双向转换。Codec 只做格式翻译，不调用 Provider；实际的 Provider 调用由上层 `bamboo/relay` 负责。

## 目录结构

```text
bamboo/codec/
├── codec.go                # Codec 接口 + StreamSerializer 接口 + FormatType 常量
├── types.go                # RelayRequest 统一请求中间表示
├── registry.go             # 全局 Codec 注册变量 + Get() 查找函数
├── errors.go               # CodecError 错误类型 + ErrorType 常量
├── anthropic/              # Anthropic Messages 协议编解码
│   ├── codec.go            # 全局 Codec 实例 + init() 注册
│   ├── request.go          # ParseRequest: Anthropic JSON → RelayRequest
│   ├── response.go         # SerializeResponse: bamboo.Response → Anthropic JSON
│   ├── stream.go           # StreamSerializer: StreamEvent → Anthropic SSE 帧
│   ├── error.go            # SerializeError: error → Anthropic 错误 JSON
│   ├── request_test.go     # 请求解析单元测试
│   └── response_test.go    # 响应序列化单元测试
├── openai/                 # OpenAI Chat Completions 协议编解码（结构同 anthropic/）
│   ├── codec.go / request.go / response.go / stream.go / error.go
│   ├── request_test.go / response_test.go
│   └── stream_test.go      # 流式序列化单元测试（SSE 帧生成）
├── responses/              # OpenAI Responses 协议编解码（结构同 anthropic/）
│   ├── codec.go / request.go / response.go / stream.go / error.go
│   └── request_test.go / response_test.go
└── gemini/                 # Google Gemini 协议编解码（结构同 anthropic/）
    ├── codec.go / request.go / response.go / stream.go / error.go
    └── request_test.go
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解 Codec 接口 | `codec.go` | `Codec` 接口（5 方法）+ `StreamSerializer` 接口（2 方法） |
| 添加新协议格式 | 子包目录 | 新建子包，实现 Codec + StreamSerializer，在 `init()` 中注册到全局变量 |
| 理解请求中间表示 | `types.go` | `RelayRequest` — 解析后的统一请求结构 |
| 查找已注册 Codec | `registry.go` | `Get(format)` 函数 + 4 个全局 Codec 变量 |
| 错误分类 | `errors.go` | `CodecError` + 5 种 `ErrorType` |
| 修改 Anthropic 编解码 | `anthropic/` | `request.go`(解析) / `response.go`(序列化) / `stream.go`(流式) |
| 修改 OpenAI 编解码 | `openai/` | 结构与 `anthropic/` 完全一致 |
| 修改 Responses 编解码 | `responses/` | 结构与 `anthropic/` 完全一致 |
| 修改 Gemini 编解码 | `gemini/` | 结构与 `anthropic/` 完全一致 |
| 测试 OpenAI 流式序列化 | `openai/stream_test.go` | SSE 帧生成的单元测试 |

## 约定

- **Codec 无状态** — `Codec` 接口本身无状态，可安全并发使用；有状态操作通过 `NewSerializer()` 创建独立的 `StreamSerializer` 实例
- **init() 自动注册** — 每个格式子包在 `init()` 中将自身 Codec 赋值到 `registry.go` 的全局变量，只要 import 了对应子包即自动注册
- **RelayRequest 中间表示** — 所有外部协议的请求体先解析为 `RelayRequest`（包含 `Messages`/`System`/`Config`/`IsStream`），再由 relay 层交给 Provider 处理
- **错误分类标准化** — 使用 `CodecError` + `ErrorType` 分类错误（invalid_request / provider_error / authentication_error / rate_limit_exceeded / internal_error），每个子包的 `SerializeError` 负责将错误映射为对应协议的错误响应格式
- **子包结构一致** — 四个格式子包（anthropic / openai / responses / gemini）内部文件分工完全一致：`codec.go` + `request.go` + `response.go` + `stream.go` + `error.go`

## 反模式

- **禁止** 在 Codec 层直接调用 Provider — Codec 只做格式转换，Provider 调用由 `bamboo/relay` 负责
- **禁止** 在 `Codec` 接口方法中保存状态 — 有状态逻辑必须通过 `NewSerializer()` 返回的 `StreamSerializer` 实例处理
- **禁止** 忘记在子包 `init()` 中注册全局变量 — 否则 `registry.Get()` 返回 nil Codec
- **禁止** 裸返回 error — 内部错误应包装为 `CodecError` 以保留错误分类信息

## 调试路径

1. 请求解析失败 → 检查对应子包的 `request.go` 的 `parseRequest` 是否正确处理所有字段
2. 响应序列化格式错误 → 检查对应子包的 `response.go` 的 `serializeResponse` 输出是否符合目标协议规范
3. 流式 SSE 帧错误 → 检查对应子包的 `stream.go` 的 `Serialize` 和 `Flush` 是否正确生成 SSE 数据帧
4. `Get(format)` 返回 nil → 检查调用方是否 import 了对应格式的子包（触发 `init()` 注册）
5. 错误响应格式不匹配 → 检查对应子包的 `error.go` 的 `serializeError` 输出
6. 缓存字段未透传 → 检查 `request.go` 解析时是否提取了 `cache_control` 字段，`response.go` 序列化时是否输出 `cache_creation_input_tokens` / `cache_read_input_tokens`

## 引用

无子级 AGENTS.md

# bamboo 原生协议编解码知识库

## 概述

`bamboo/codec/bamboo` 是 bamboo 原生协议的恒等变换编解码包。它直接使用上层 `bamboo` 门面层的公开类型（`BambooMessage`、`RequestConfig`、`Response`、`StreamEvent`），不对 JSON 结构做任何转换，只负责把请求体解析为 `RelayRequest`、把响应序列化为原生 JSON、把流事件封装为 Anthropic 风格的 SSE 帧。该包存在的意义是让 bamboo 原生端点可以作为 `bamboo/relay` 的中继目标之一，实现同协议直连。

## 目录结构

```text
bamboo/codec/bamboo/
├── codec.go           # bambooCodec 实现 + 全局 Codec 变量 + init() 注册
├── request.go         # parseRequest: json.Unmarshal 原生信封 → RelayRequest
├── response.go        # serializeResponse: json.Marshal(*bamboo.Response)
├── stream.go          # bambooStreamSerializer: json.Marshal(StreamEvent) + Anthropic SSE 帧
├── error.go           # serializeError: 提取 *bamboo.BambooError 字段生成错误响应
├── request_test.go    # 请求解析单元测试
├── response_test.go   # 响应序列化单元测试
├── stream_test.go     # 流式 SSE 帧生成单元测试
└── error_test.go      # 错误序列化单元测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 理解 Codec 实现 | `codec.go` | `bambooCodec` 实现 5 方法 Codec 接口 |
| 查看全局注册 | `codec.go` | `init()` 将 `Codec` 赋值到 `bmcodec.Bamboo` |
| 理解请求解析 | `request.go` | `bambooEnvelope` 直接复用 `bamboo.BambooMessage` / `RequestConfig` |
| 理解响应序列化 | `response.go` | 真正的恒等变换：直接 `json.Marshal` |
| 理解流式序列化 | `stream.go` | 不区分事件类型，统一 JSON marshal 后加 SSE 帧头 |
| 理解错误序列化 | `error.go` | 输出 `{"type":"error","error":{...}}` 结构 |

## 代码地图

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `bambooCodec` | 结构体 | `codec.go` | bamboo 原生协议恒等变换 Codec 实现 |
| `Codec` | 变量 | `codec.go` | 全局 `bmcodec.Codec` 实例，供外部直接使用 |
| `parseRequest` | 函数 | `request.go` | 将 bamboo 原生请求体解析为 `RelayRequest` |
| `serializeResponse` | 函数 | `response.go` | 将 `*bamboo.Response` 序列化为 JSON |
| `serializeError` | 函数 | `error.go` | 将错误序列化为 bamboo 原生错误 JSON |
| `bambooStreamSerializer` | 结构体 | `stream.go` | 流式序列化器，按事件类型输出 SSE 帧 |
| `newStreamSerializer` | 函数 | `stream.go` | 创建新的 `bambooStreamSerializer` 实例 |
| `bambooEnvelope` | 结构体 | `request.go` | 原生请求信封 `{messages,system,config,stream}` |
| `bambooErrorResponse` | 结构体 | `error.go` | 错误响应外层 `{type,error}` |
| `bambooErrorPayload` | 结构体 | `error.go` | 错误响应负载 `{category,message,status_code}` |

## 约定

- **恒等变换** — `ParseRequest` 直接 `json.Unmarshal` 到 `bambooEnvelope`，`SerializeResponse` 直接 `json.Marshal(*bamboo.Response)`，流式序列化直接 `json.Marshal(StreamEvent)`。不构造中间 DTO，不做字段映射。
- **包名冲突必须用 import 别名** — 本子包包名也是 `bamboo`，因此 `github.com/bamboo-services/bamboo-messages/bamboo` 在内部使用别名 `bmbamboo`，`bamboo/codec` 使用别名 `bmcodec`。
- **SSE 帧与 Anthropic 协议一致** — 输出格式为 `event: {type}\ndata: {json}\n\n`，无 `[DONE]` 标记，`message_stop` 事件本身即终止信号。
- **错误格式与 BambooError 一致** — `serializeError` 优先通过 `errors.As` 提取 `*bmbamboo.BambooError`，透传 `Category`、`Message`、`StatusCode`；否则降级为 `Category="SDK"`。
- **流式序列化无状态分派** — `bambooStreamSerializer.Serialize` 不根据 `event.Type` 分派，而是依赖 `StreamEvent` 自身的 JSON tag 和接口字段多态，统一走 JSON marshal。
- **Config 缺失时补零值** — `parseRequest` 发现 `config` 为 nil 时填充 `&bmbamboo.RequestConfig{}`，避免下游出现 nil 指针。

## 反模式

- **禁止** 在 bamboo codec 中引入中间 DTO — 恒等变换应保持直接使用 facade 类型。
- **禁止** 不带别名 import facade 的 `bamboo` 包 — 会与当前包包名冲突。
- **禁止** 在 `Serialize` 中按事件类型做手动字段拼接 — 应直接 JSON marshal 整个 `StreamEvent`。
- **禁止** 输出 `[DONE]` 标记 — bamboo 原生协议以 `message_stop` 作为流结束信号。
- **禁止** 在响应序列化中修改 `bamboo.Response` 字段 — 必须保持 1:1 透传。

## 调试路径

1. 请求解析为空 → 检查 `bambooEnvelope` 的 JSON tag 是否与请求体一致，确认 `messages`、`system`、`config`、`stream` 字段。
2. 响应 JSON 缺少字段 → 检查 `bamboo.Response` 各字段的 `json` tag 是否被正确设置，不要在此包覆盖。
3. SSE 帧格式不对 → 检查 `stream.go` 输出是否为 `event: {type}\ndata: {json}\n\n`，不要添加多余空格或 `[DONE]`。
4. 错误响应没有 status_code → 检查错误是否为 `*bamboo.BambooError` 类型，否则会被降级为 `SDK` 分类。
5. `codec.Get(FormatBamboo)` 返回 nil → 确认调用方 import 了 `bamboo/codec/bamboo` 以触发 `init()`。
6. 流事件 `index` 字段消失 → `StreamEvent.Index` 带 `omitempty`，值为 0 时不会输出，这是预期行为。

## 引用

- [父级 codec 知识库](../AGENTS.md) — N-to-N 协议编解码层总览
- [provider/bamboo](../../../provider/bamboo/AGENTS.md) — bamboo 原生协议 Provider 适配器

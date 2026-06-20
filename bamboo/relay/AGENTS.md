# relay 知识库

## 概述

跨协议中继层，是 `bamboo/codec` 与 `bamboo` client 之间的薄包装。通过 `Relay()` 和 `RelayStream()` 两个函数，实现任意输入协议格式 → 任意输出协议格式的请求-响应互转，底层由 Provider 执行实际的 AI 对话。

## 目录结构

```text
bamboo/relay/
├── config.go              # Config + Option + WithUsageCallback/WithErrorCallback/WithDebug + applyOptions
├── relay.go               # Relay() 非流式互转 + RelayStream() 流式互转
├── debug.go               # shouldDebug + debugRelayInput/debugRelayParsed + FormatRelayInput/FormatRelayParsed + 长文本截断
├── relay_test.go          # 非流式互转单元测试
├── stream_test.go         # 流式互转单元测试
└── cross_format_test.go   # 跨格式组合测试（N-to-N 矩阵）
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 非流式协议互转 | `relay.go` | `Relay(ctx, p, body, inFormat, outFormat, opts...)` |
| 流式协议互转 | `relay.go` | `RelayStream(ctx, p, body, inFormat, outFormat, opts...)` |
| 配置回调 | `config.go` | `WithUsageCallback(fn)` / `WithErrorCallback(fn)` / `WithDebug(bool)` |
| 启用 debug 日志 | `config.go` | `WithDebug(true)` Option 或环境变量 `BAMBOO_DEBUG=1` |
| 理解内部流程 | `relay.go` | Relay: ParseRequest → Complete → SerializeResponse；RelayStream: ParseRequest → Chat → Serialize → channel |
| 查看 debug 输出格式 | `debug.go` | `debugRelayInput` (原始 body) + `debugRelayParsed` (解析后的 RelayRequest) |
| 自定义日志格式 | `debug.go` | `FormatRelayInput()` / `FormatRelayParsed()` 返回字符串（不受开关限制） |

## 约定

- **薄包装设计** — relay 层不引入新的业务逻辑，仅串联 codec 解析 → bamboo client 调用 → codec 序列化三个步骤
- **Functional Options** — 通过 `Option` 函数式选项配置 `Config`，零值 Config 即可正常工作（回调为 nil 时安全跳过）
- **Usage 回调时机** — 非流式在 `Complete` 返回后触发；流式在收到携带 Usage 的 `message_delta` 事件时触发
- **Error 回调不影响返回** — 错误仍正常返回给调用方，回调仅用于异步通知（日志/告警）
- **流式 channel 自动关闭** — `RelayStream` 返回的 `<-chan []byte` 在流结束后自动 close，调用方 range 遍历即可
- **context 取消传播** — 非流式和流式均尊重 `ctx.Done()`，在 goroutine 中 select ctx 实现取消传播
- **Debug 双层开关** — `Config.Debug`（通过 `WithDebug(true)`）优先；未设置时回退到环境变量 `BAMBOO_DEBUG=1/true/on`；启用后在 `ParseRequest` 前后分别输出原始 body 和解析后的 RelayRequest，便于排查 codec 转换偏差
- **Debug 长文本截断** — `content` / `text` / `system` / `thinking` / `reasoning_content` / `arguments` 等字段超过 `maxDebugBodyLen` (500) 时自动截断，避免日志爆炸

## 反模式

- **禁止** 在 relay 层引入协议特定的格式判断 — 所有协议差异由 codec 层处理，relay 只通过 `FormatType` 标识路由
- **禁止** 忘记触发回调 — 发生错误或收到 Usage 时必须调用 `cfg.triggerError` / `cfg.triggerUsage`
- **禁止** 在 `RelayStream` 的 goroutine 中遗漏 `close(out)` — 必须在 goroutine 退出时关闭输出 channel

## 调试路径

1. 互转结果不正确 → 先确认 `codec.Get(inFormat)` 和 `codec.Get(outFormat)` 返回的 Codec 非 nil（检查 import）
2. 流式中断 → 检查 `RelayStream` goroutine 中的 `select ctx.Done()` 是否正确响应取消
3. 回调未触发 → 检查 `cfg.triggerUsage` / `cfg.triggerError` 调用路径是否被遗漏
4. Provider 调用失败 → 检查 `bamboo.NewClient(p).Complete()` 或 `.Chat()` 是否返回错误
5. Flush 数据丢失 → 检查 `RelayStream` goroutine 结束前是否调用了 `serializer.Flush()`
6. Codec 解析偏差 → 启用 `WithDebug(true)` 或设置 `BAMBOO_DEBUG=1`，对比 `relay input`（原始 body）与 `relay parsed`（RelayRequest）的差异

## 引用

无子级 AGENTS.md

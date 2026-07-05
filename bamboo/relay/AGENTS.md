# relay 知识库

## 概述

跨协议中继层，是 `bamboo/codec` 与 `bamboo` client 之间的薄包装。通过 `Relay()` 和 `RelayStream()` 两个函数，实现任意输入协议格式 -> 任意输出协议格式的请求-响应互转，底层由 Provider 执行实际的 AI 对话。流式路径为纯透传——上游 provider 产生的 SSE 帧经 codec 序列化后直达输出 channel，无中间缓冲、无调速。

## 目录结构

```text
bamboo/relay/
├── config.go              # Config + Option + WithUsageCallback/WithErrorCallback/WithUsageEstimation + applyOptions
├── relay.go               # Relay() 非流式互转（含 SerializeError） + RelayStream() 流式互转（纯透传）
├── debug.go               # debugRelayInput/debugRelayParsed + FormatRelayInput/FormatRelayParsed + 长文本截断
├── relay_test.go          # 非流式互转单元测试
├── stream_test.go         # 流式互转单元测试
├── cross_format_test.go   # 跨格式组合测试（N-to-N 矩阵）
├── debug_test.go          # Debug 日志格式化单元测试
├── usage_fallback_test.go # Usage 回退路径单元测试
└── usage_estimate_test.go # Token 估算与 Usage 采样单元测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 非流式协议互转 | `relay.go` | `Relay(ctx, p, body, inFormat, outFormat, opts...)` |
| 流式协议互转 | `relay.go` | `RelayStream(ctx, p, body, inFormat, outFormat, opts...)` |
| 配置回调 | `config.go` | `WithUsageCallback(fn)` / `WithErrorCallback(fn)` / `WithUsageEstimation(true)` |
| 启用 debug 日志 | `config.go` | 环境变量 `BAMBOO_DEBUG=1`（relay 层直接检查 `provider.DebugEnabled`） |
| 理解内部流程 | `relay.go` | Relay: ParseRequest -> Complete -> SerializeResponse；RelayStream: ParseRequest -> Chat -> Serialize -> channel |
| 查看 debug 输出格式 | `debug.go` | `debugRelayInput` (原始 body) + `debugRelayParsed` (解析后的 RelayRequest) |
| 自定义日志格式 | `debug.go` | `FormatRelayInput()` / `FormatRelayParsed()` 返回字符串（不受开关限制） |
| 运行互转测试 | `*_test.go` | relay_test / stream_test / cross_format_test / usage_fallback_test / usage_estimate_test |

## 代码地图

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `Relay` | 函数 | relay.go | 非流式协议互转（含 SerializeError 错误格式化） |
| `RelayStream` | 函数 | relay.go | 流式协议互转（纯透传，含 Usage 采样与估算回退） |
| `SplitSSEFrames` | 函数 | relay.go | 按 SSE 事件边界（`\n\n`）拆分为独立帧，导出供上层业务复用 |
| `Config` | 结构体 | config.go | relay 运行时配置（OnUsage / OnError / EstimateOnMissingUsage） |
| `Option` | 函数类型 | config.go | Functional Options 配置函数 |
| `WithUsageCallback` | Option | config.go | 设置 Token 用量回调 |
| `WithErrorCallback` | Option | config.go | 设置错误回调 |
| `WithUsageEstimation` | Option | config.go | 启用 usage 缺失时的估算回退 |

### 内部辅助函数（未导出）

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `applyOptions` | 函数 | config.go | 应用所有选项并返回配置实例 |
| `triggerUsage` | 方法 | config.go | 安全触发 Usage 回调 |
| `triggerError` | 方法 | config.go | 安全触发 Error 回调 |
| `estimateTokenCount` | 函数 | relay.go | CJK 1:1 / Latin 4:1 / Other 2:1 token 估算 |
| `estimateUsage` | 函数 | relay.go | 基于请求内容和输出文本估算 token 用量 |
| `toBambooError` | 函数 | relay.go | 将非 BambooError 错误降级包装为 BambooError |

## 约定

- **薄包装设计** — relay 层不引入新的业务逻辑，仅串联 codec 解析 -> bamboo client 调用 -> codec 序列化三个步骤
- **Functional Options** — 通过 `Option` 函数式选项配置 `Config`，零值 Config 即可正常工作（回调为 nil 时安全跳过）
- **Usage 回调时机** — 非流式在 `Complete` 返回后触发；流式在收到携带 Usage 的 `message_delta` 事件时触发
- **Error 回调不影响返回** — 错误仍正常返回给调用方，回调仅用于异步通知（日志/告警）
- **流式 channel 自动关闭** — `RelayStream` 返回的 `<-chan []byte` 在流结束后自动 close，调用方 range 遍历即可
- **流式纯透传** — `RelayStream` goroutine 中所有 SSE 帧（含 EventPing）统一走 `select { case out <- frame: case <-ctx.Done(): }` 直通路径，无中间缓冲、无调速、无 token 切分
- **context 取消传播** — 非流式和流式均尊重 `ctx.Done()`，在 goroutine 中 select ctx 实现取消传播
- **Debug 环境变量统一** — relay 层 debug 函数直接检查 `provider.DebugEnabled`（由环境变量 `BAMBOO_DEBUG` 控制）；原有的 debug 配置字段和 Option 已移除
- **Debug 长文本截断** — `content` / `text` / `system` / `thinking` / `reasoning_content` / `arguments` 等字段超过 `maxDebugBodyLen` (500) 时自动截断，避免日志爆炸
- **Relay 失败返回协议格式错误** — `Relay` 在 provider 失败时调用 `outCodec.SerializeError(err)` 返回协议格式化的错误 body，而非 nil body
- **Usage 触发守卫** — `RelayStream` 中 `usageTriggered` 标志防止 Usage 回调重复触发
- **RelayStream 流式帧 debug 已移除** — `RelayStream` 不再通过 `debugRelayResponseFrame` 输出逐帧 debug 日志；上层业务（如 newapi）可通过自身中间层从输出 channel 捕获完整流内容

## 反模式

- **禁止** 在 relay 层引入协议特定的格式判断 — 所有协议差异由 codec 层处理，relay 只通过 `FormatType` 标识路由
- **禁止** 忘记触发回调 — 发生错误或收到 Usage 时必须调用 `cfg.triggerError` / `cfg.triggerUsage`
- **禁止** 在 `RelayStream` 的 goroutine 中遗漏 `close(out)` — 必须在 goroutine 退出时关闭输出 channel

## 调试路径

1. 互转结果不正确 -> 先确认 `codec.Get(inFormat)` 和 `codec.Get(outFormat)` 返回的 Codec 非 nil（检查 import）
2. 流式中断 -> 检查 `RelayStream` goroutine 中的 `select ctx.Done()` 是否正确响应取消
3. 回调未触发 -> 检查 `cfg.triggerUsage` / `cfg.triggerError` 调用路径是否被遗漏
4. Provider 调用失败 -> 检查 `bamboo.NewClient(p).Complete()` 或 `.Chat()` 是否返回错误
5. Flush 数据丢失 -> 检查 `RelayStream` goroutine 结束前是否调用了 `serializer.Flush()`
6. Codec 解析偏差 -> 启用 `BAMBOO_DEBUG=1` 环境变量，对比 `relay input`（原始 body）与 `relay parsed`（RelayRequest）的差异
7. Relay 返回错误格式不对 -> 检查 `outCodec.SerializeError` 是否正确格式化了 provider 错误

## 引用

无子级 AGENTS.md

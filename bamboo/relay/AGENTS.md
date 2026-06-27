# relay 知识库

## 概述

跨协议中继层，是 `bamboo/codec` 与 `bamboo` client 之间的薄包装。通过 `Relay()` 和 `RelayStream()` 两个函数，实现任意输入协议格式 -> 任意输出协议格式的请求-响应互转，底层由 Provider 执行实际的 AI 对话。支持流式平滑缓冲（Smooth Buffer），通过 EMA 自适应间隔实现匀速输出。

## 目录结构

```text
bamboo/relay/
├── config.go              # Config + Option + WithUsageCallback/WithErrorCallback/WithDebug/WithRateSampleCallback + applyOptions
├── relay.go               # Relay() 非流式互转（含 SerializeError） + RelayStream() 流式互转（含速率采样集成）
├── debug.go               # shouldDebug + debugRelayInput/debugRelayParsed + FormatRelayInput/FormatRelayParsed + 长文本截断
├── smooth.go              # SmoothLevel/SmoothParams/SmoothConfig + WithSmoothBuffer/WithSmoothBufferCustom Option
├── smooth_pacer.go        # SmoothPacer 流式平滑缓冲器（三模式状态机：NORMAL/DRAIN/FLUSH）+ SetRateSampleCallback + Close
├── smooth_parser.go       # TokenSplitter token 切分器 + FrameParser SSE 帧解析器（支持 4 种协议格式）
├── relay_test.go          # 非流式互转单元测试
├── stream_test.go         # 流式互转单元测试
├── cross_format_test.go   # 跨格式组合测试（N-to-N 矩阵）
├── smooth_test.go         # 平滑缓冲器单元测试（TokenSplitter/FrameParser/SmoothPacer/集成测试）
└── usage_fallback_test.go # Usage 回退路径单元测试
```

## 导航指南

| 任务 | 位置 | 说明 |
|------|------|------|
| 非流式协议互转 | `relay.go` | `Relay(ctx, p, body, inFormat, outFormat, opts...)` |
| 流式协议互转 | `relay.go` | `RelayStream(ctx, p, body, inFormat, outFormat, opts...)` |
| 配置回调 | `config.go` | `WithUsageCallback(fn)` / `WithErrorCallback(fn)` / `WithDebug(bool)` / `WithRateSampleCallback(fn)` |
| 启用 debug 日志 | `config.go` | `WithDebug(true)` Option 或环境变量 `BAMBOO_DEBUG=1` |
| 理解内部流程 | `relay.go` | Relay: ParseRequest -> Complete -> SerializeResponse；RelayStream: ParseRequest -> Chat -> Serialize -> channel |
| 查看 debug 输出格式 | `debug.go` | `debugRelayInput` (原始 body) + `debugRelayParsed` (解析后的 RelayRequest) |
| 自定义日志格式 | `debug.go` | `FormatRelayInput()` / `FormatRelayParsed()` 返回字符串（不受开关限制） |
| 启用平滑缓冲（预设档位） | `smooth.go` | `WithSmoothBuffer(level)` — 支持 gentle/smooth/typewriter 三档 |
| 启用平滑缓冲（自定义参数） | `smooth.go` | `WithSmoothBufferCustom(params)` — 完全自定义 SmoothParams |
| 理解平滑缓冲架构 | `smooth_pacer.go` | SmoothPacer 三模式状态机：NORMAL（EMA 自适应）-> DRAIN（阶梯加速）-> FLUSH（立即排空） |
| 理解 token 切分规则 | `smooth_parser.go` | TokenSplitter: CJK 独立 token / Latin 合并 / 空格附着前缀 / 标点附着后缀 / Emoji 独立 |
| 理解 SSE 帧解析 | `smooth_parser.go` | FrameParser: 控制事件透传 / text/thinking delta 切分 / 屏障事件排空 |
| 运行平滑缓冲测试 | `smooth_test.go` | 分组：TokenSplitter / FrameParser / SmoothPacer / 集成测试 / 边界测试 / 算法单元测试 |

## 代码地图

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `SmoothLevel` | 类型 | smooth.go | 平滑档位标识：off/gentle/smooth/typewriter/custom |
| `SmoothParams` | 结构体 | smooth.go | 平滑参数：TokensPerFrame/MinInterval/MaxInterval/EMAAlpha/DrainTier1Ratio/DrainTier1Mult/DrainTier2Ratio/DrainTier2Mult |
| `SmoothConfig` | 结构体 | smooth.go | 平滑缓冲器配置：Level + Params |
| `WithSmoothBuffer` | Option | smooth.go | 启用平滑缓冲器（预设档位），未知档位静默跳过 |
| `WithSmoothBufferCustom` | Option | smooth.go | 启用平滑缓冲器（自定义参数），档位标记为 "custom" |
| `SmoothPacer` | 结构体 | smooth_pacer.go | 流式平滑缓冲器，接收 SSE 帧按 EMA 自适应间隔匀速释放 |
| `NewSmoothPacer` | 构造函数 | smooth_pacer.go | 创建平滑缓冲器并启动 pacer goroutine |
| `SmoothPacer.Push` | 方法 | smooth_pacer.go | 非阻塞推送 SSE 帧到 pacer（input channel 128 缓冲） |
| `SmoothPacer.SignalEnd` | 方法 | smooth_pacer.go | 通知 pacer 上游已结束，切换到 DRAIN 模式 |
| `SmoothPacer.Wait` | 方法 | smooth_pacer.go | 等待 pacer goroutine 完全退出 |
| `SmoothPacer.Close` | 方法 | smooth_pacer.go | 语义清理别名（等同 `Wait()`） |
| `SmoothPacer.SetRateSampleCallback` | 方法 | smooth_pacer.go | 设置速率采样回调（thinking vs output token/s） |
| `TokenSplitter` | 结构体 | smooth_parser.go | token 切分器，维护跨帧 pendingTail |
| `NewTokenSplitter` | 构造函数 | smooth_parser.go | 创建 token 切分器实例 |
| `TokenSplitter.Split` | 方法 | smooth_parser.go | 切分文本为 token 列表，保留不完整的尾部片段 |
| `TokenSplitter.Flush` | 方法 | smooth_parser.go | 返回 pendingTail 残余并清空 |
| `FrameParser` | 结构体 | smooth_parser.go | SSE 帧解析器，将原始 SSE 帧解析为 microFrame 列表 |
| `NewFrameParser` | 构造函数 | smooth_parser.go | 创建帧解析器实例 |
| `FrameParser.Parse` | 方法 | smooth_parser.go | 解析 SSE 帧，根据 outFormat 分派到对应协议格式的解析函数 |
| `FrameParser.FlushText` | 方法 | smooth_parser.go | 输出 text splitter 的 pendingTail 残余帧 |
| `FrameParser.FlushThinking` | 方法 | smooth_parser.go | 输出 thinking splitter 的 pendingTail 残余帧 |
| `FrameParser.FlushRemaining` | 方法 | smooth_parser.go | 输出 text+thinking splitter 的全部残余帧 |
| `Config.OnRateSample` | 字段 | config.go | 速率采样回调 `func(elapsedSec, tokensPerSec float64, kind provider.RateSampleKind)` |
| `WithRateSampleCallback` | Option | config.go | 设置速率采样回调（仅 SmoothBuffer 启用时生效） |

### 内部实现（未导出）

| 符号 | 类型 | 文件 | 作用 |
|------|------|------|------|
| `microFrame` | 结构体 | smooth_parser.go | 微帧 — 切分后的最小输出单元（kind/data/tokenCount/isBarrier） |
| `pacerMode` | 类型 | smooth_pacer.go | pacer 运行模式：modeNormal/modeDrain/modeFlush |
| `effectiveInterval` | 函数 | smooth_pacer.go | 积压感知间隔缩减（queueLen 10->100 线性映射到 factor 1.0->0.0） |
| `effectiveTokensPerFrame` | 函数 | smooth_pacer.go | interval 到 floor 后 token 扩容（每 20 帧额外积压 -> multiplier +1，上限 8） |
| `effectiveDrainInterval` | 函数 | smooth_pacer.go | DRAIN 模式输出间隔（两级：高于 DrainTier2Ratio -> minIntervalFloor，低于 -> 0） |

## 约定

- **薄包装设计** — relay 层不引入新的业务逻辑，仅串联 codec 解析 -> bamboo client 调用 -> codec 序列化三个步骤
- **Functional Options** — 通过 `Option` 函数式选项配置 `Config`，零值 Config 即可正常工作（回调为 nil 时安全跳过）
- **Usage 回调时机** — 非流式在 `Complete` 返回后触发；流式在收到携带 Usage 的 `message_delta` 事件时触发
- **Error 回调不影响返回** — 错误仍正常返回给调用方，回调仅用于异步通知（日志/告警）
- **流式 channel 自动关闭** — `RelayStream` 返回的 `<-chan []byte` 在流结束后自动 close，调用方 range 遍历即可
- **context 取消传播** — 非流式和流式均尊重 `ctx.Done()`，在 goroutine 中 select ctx 实现取消传播
- **Debug 双层开关** — `Config.Debug`（通过 `WithDebug(true)`）优先；未设置时回退到环境变量 `BAMBOO_DEBUG=1/true/on`；启用后在 `ParseRequest` 前后分别输出原始 body 和解析后的 RelayRequest，便于排查 codec 转换偏差
- **Debug 长文本截断** — `content` / `text` / `system` / `thinking` / `reasoning_content` / `arguments` 等字段超过 `maxDebugBodyLen` (500) 时自动截断，避免日志爆炸
- **平滑缓冲预设档位** — `SmoothLevel` 定义三档预设：gentle（2 token/frame, 20-100ms）、smooth（1 token/frame, 15-80ms）、typewriter（1 token/frame, 30-120ms）；传入 `SmoothLevelOff` 或未知档位时静默跳过
- **平滑缓冲自定义参数** — `WithSmoothBufferCustom(params)` 允许完全自定义 `SmoothParams`，档位标记为 "custom"
- **SmoothPacer 三模式状态机** — NORMAL（EMA 自适应间隔）-> DRAIN（阶梯加速排空）-> FLUSH（立即排空）；NORMAL 通过 `SignalEnd` 切换到 DRAIN，ctx 取消切换到 FLUSH
- **SmoothPacer 并发模型** — Push 在 RelayStream goroutine 中调用，run() 在独立 pacer goroutine 中运行，两者通过 input channel + signal channel 通信，queue 只在 run goroutine 中操作（零竞争，不使用 sync.Mutex）
- **SmoothPacer 生命周期** — `NewSmoothPacer` 启动 pacer goroutine；`Push` 非阻塞推送数据；`SignalEnd` 通知上游结束；`Wait` 等待 pacer 退出；调用方在 `Wait` 返回后可以安全 `close(out)`
- **TokenSplitter 切分规则** — CJK 字符独立 token / Latin 字母数字合并为空格前缀 / 标点附着后缀 / Emoji 独立 / 换行附着前一个 token；Latin alnum 结尾或纯空格保留为 pendingTail 等待下一帧拼接
- **FrameParser 多协议支持** — 根据 `codec.FormatType` 分派到 Anthropic/OpenAI/Responses/Gemini 四种解析函数；控制事件（block_start/message_start）直接透传，屏障事件（block_stop/message_stop/error）标记 isBarrier，text/thinking delta 通过 TokenSplitter 切分为微帧
- **FrameParser 残余帧构建** — 解析器记录最后一次 text/thinking delta 的构建上下文（index/id 等字段），`FlushRemaining()` 基于该上下文生成残余帧，保证格式与原始 delta 帧一致
- **积压感知间隔缩减** — queueLen 10->100 线性映射到 factor 1.0->0.0，effective 从 baseInterval 线性缩减到 minIntervalFloor(2ms)；queueLen <= 10 时无积压感知
- **token 扩容机制** — 仅当 intervalAtFloor=true 且 queueLen > 20 时扩容；每 20 帧额外积压 -> multiplier +1，上限 8
- **Barrier 帧语义** — 遇到 barrier 时，先排空前面所有积压的数据帧，然后输出 barrier 本身（保证时序：barrier 前的数据必须先到达）
- **速率采样回调** — `Config.OnRateSample` 在 SmoothPacer 每次输出 tick 时触发，区分 thinking 阶段（`RateSampleKindThinking`）和 output 阶段（`RateSampleKindOutput`）；仅 SmoothBuffer 启用时生效
- **Relay 失败返回协议格式错误** — `Relay` 在 provider 失败时调用 `outCodec.SerializeError(err)` 返回协议格式化的错误 body，而非 nil body
- **Usage 触发守卫** — `RelayStream` 中 `usageTriggered` 标志防止 Usage 回调重复触发

## 反模式

- **禁止** 在 relay 层引入协议特定的格式判断 — 所有协议差异由 codec 层处理，relay 只通过 `FormatType` 标识路由
- **禁止** 忘记触发回调 — 发生错误或收到 Usage 时必须调用 `cfg.triggerError` / `cfg.triggerUsage`
- **禁止** 在 `RelayStream` 的 goroutine 中遗漏 `close(out)` — 必须在 goroutine 退出时关闭输出 channel
- **禁止** 在 `SignalEnd` 之后调用 `Push` — `SignalEnd` 后 input 不会再被接收，Push 会被 ctx.Done 阻塞
- **禁止** 在 SmoothPacer 的 run goroutine 外部直接操作 queue — queue 只在 run goroutine 中读写，外部操作会导致数据竞争
- **禁止** 忘记调用 `Wait()` 就 close(out) — 必须等待 pacer goroutine 完全退出后再关闭输出 channel，否则可能丢失数据
- **禁止** 在 FrameParser 中裸解析未知格式 — 未知 format 回退为 frameControl 透传，不应 panic
- **禁止** 忽略 TokenSplitter 的 pendingTail — 流结束时必须调用 `FlushRemaining()` 确保最后一帧的不完整 token 被输出

## 调试路径

1. 互转结果不正确 -> 先确认 `codec.Get(inFormat)` 和 `codec.Get(outFormat)` 返回的 Codec 非 nil（检查 import）
2. 流式中断 -> 检查 `RelayStream` goroutine 中的 `select ctx.Done()` 是否正确响应取消
3. 回调未触发 -> 检查 `cfg.triggerUsage` / `cfg.triggerError` 调用路径是否被遗漏
4. Provider 调用失败 -> 检查 `bamboo.NewClient(p).Complete()` 或 `.Chat()` 是否返回错误
5. Flush 数据丢失 -> 检查 `RelayStream` goroutine 结束前是否调用了 `serializer.Flush()`
6. Codec 解析偏差 -> 启用 `WithDebug(true)` 或设置 `BAMBOO_DEBUG=1`，对比 `relay input`（原始 body）与 `relay parsed`（RelayRequest）的差异
7. 平滑缓冲输出不匀速 -> 检查 `SmoothParams` 的 `EMAAlpha` 是否过大（导致间隔抖动），或 `MinInterval`/`MaxInterval` 范围是否合理
8. 平滑缓冲尾部数据丢失 -> 检查是否遗漏 `SignalEnd()` 调用，或 `Wait()` 未返回就 close(out)
9. 平滑缓冲 ctx 取消后卡死 -> 检查 pacer goroutine 是否正确处理 FLUSH 模式（`enterFlushAndExit` 应立即排空后 return）
10. Token 切分不正确 -> 检查 `TokenSplitter.Split()` 的切分规则：CJK 独立 / Latin 合并 / 空格前缀 / 标点后缀；跨帧拼接通过 pendingTail 实现
11. FrameParser 解析失败 -> 检查 SSE 帧格式是否符合协议规范（Anthropic/Responses 有 event 行，OpenAI/Gemini 只有 data 行）；解析失败时回退为 frameControl 透传
12. Barrier 时序错乱 -> 检查 `outputBatch()` 中 barrier 处理逻辑：遇到 barrier 时应先排空前面所有积压的数据帧，再输出 barrier 本身
13. 平滑缓冲内容不完整 -> 检查 `FlushRemaining()` 是否在流结束时被调用，确保 pendingTail 残余帧被输出
14. 速率采样回调不触发 -> 检查 `WithRateSampleCallback` 是否设置，且 `WithSmoothBuffer` 是否启用（速率采样仅在 SmoothBuffer 激活时生效）
15. Relay 返回错误格式不对 -> 检查 `outCodec.SerializeError` 是否正确格式化了 provider 错误

## 引用

无子级 AGENTS.md

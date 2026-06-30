# GLM 流式中断根因分析报告

> **调查日期**: 2026-06-30
> **调查者**: Sisyphus (OhMyOpenCode)
> **涉及 SDK**: bamboo-messages (Go 1.25, openai-go/v3 v3.30)
> **涉及端点**: 智谱 GLM Coding API / newapi 中转 / 讯飞 MaaS Coding API

## 一、调查方法

本次调查采用 systematic-debugging 方法论，分四个阶段：

1. **Phase 1 - 证据收集**：直接 curl 测试三个端点（GLM 直连、中转 newapi、讯飞）的原始 SSE 输出，带毫秒级时间戳
2. **Phase 2 - 代码审查**：3 个并行 agent 审查 SDK 完整流解析管线（`chat.go` / `stream.go` / `params.go` / `ssestream.go`）
3. **Phase 3 - 文献调研**：librarian agent 搜索 GLM 官方文档 + GitHub Issues + 第三方集成报告
4. **Phase 4 - 交叉比对**：将 GLM 实际 SSE 输出格式与 SDK 解析器期望进行逐字段对比

## 二、实测证据

### 2.1 GLM 直连 curl 测试 (open.bigmodel.cn)

```
端点: https://open.bigmodel.cn/api/coding/paas/v4/chat/completions
模型: glm-4.5
结果: ✅ 流式完整，无断流
耗时: ~4 秒，~640 chunks
```

关键 SSE 格式（最后 3 帧）：

```
data: {"id":"...","choices":[{"index":0,"delta":{"role":"assistant","content":"。"}}]}

data: {"id":"...","choices":[{"index":0,"finish_reason":"stop","delta":{"role":"assistant","content":""}}],"usage":{"prompt_tokens":25,"completion_tokens":1601,"total_tokens":1626,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":242}}}

data: [DONE]
```

GLM 直连特征：

- `finish_reason` 和 `usage` 在**同一个 chunk** 中
- `choices` 数组始终非空
- 无 `: keep-alive` 注释行
- 无 `event:` 事件类型行
- 有 `reasoning_content` 字段（198 次出现）
- 无 `stream_options` 需求（usage 始终返回）

### 2.2 中转 newapi curl 测试 (ai.x-lf.com)

```
端点: https://ai.x-lf.com/v1/chat/completions
模型: glm-5.2
结果: ✅ 流式完整，无断流
耗时: ~10 秒，~2170 chunks
```

关键 SSE 格式（最后 4 帧）：

```
: keep-alive

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"id":"chatcmpl-...","model":"xopglm52","object":"chat.completion.chunk","created":...}

data: {"id":"chatcmpl-...","object":"chat.completion.chunk","created":...,"model":"xopglm52","choices":[],"usage":{"prompt_tokens":28,"completion_tokens":1906,"total_tokens":193...}}

data: [DONE]
```

中转 newapi 特征（与 GLM 直连的差异）：

| 特征               | GLM 直连          | newapi 中转          |
| ------------------ | ----------------- | -------------------- |
| `finish_reason` + `usage` | 同一帧            | **拆分为两个帧**     |
| usage 帧的 `choices`      | 非空              | **空数组 `[]`**      |
| `: keep-alive` 注释       | 无                | **有**（newapi 注入）|
| `id` 格式                 | 时间戳 `2026063...` | `chatcmpl-...`       |
| `model` 字段              | `glm-4.5`         | `xopglm52`（内部映射）|
| `delta.role`              | 始终有            | 部分帧缺失           |
| `reasoning_content`       | 有                | 有（947 次出现）     |

### 2.3 讯飞 curl 测试 (maas-coding-api.cn-huabei-1.xf-yun.com)

```
端点: https://maas-coding-api.cn-huabei-1.xf-yun.com/v2/chat/completions
模型: xopglm52
结果: ✅ 流式完整，无断流
耗时: ~17 秒，458 chunks
```

关键 SSE 格式（最后 3 帧）：

```
data: {"choices":[{"delta":{"content":"发的强大威力。"},"index":0}],"id":"cht000...@dx19f...","model":"xopglm52","object":"chat.completion.chunk"}

data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}],"id":"cht000...@dx19f...","model":"xopglm52","object":"chat.completion.chunk","usage":{"question_tokens":4,"prompt_tokens":22,"completion_tokens":1114,"total_tokens":1136,"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":0}}}

data: [DONE]
```

讯飞特征：

- `finish_reason` 和 `usage` 在**同一帧**（与 GLM 直连一致，与 newapi 不同）
- `choices` 始终非空（与 GLM 直连一致，与 newapi 不同）
- 无 `: keep-alive` 注释行
- 无 `reasoning_content` 字段
- `[DONE]` 后连接正常关闭
- `usage` 包含额外的 `question_tokens` 字段

### 2.4 三端点 SSE 格式对比总表

| 特征                        | GLM 直连 (glm-4.5) | newapi 中转 (glm-5.2) | 讯飞 (xopglm52)   |
| --------------------------- | ------------------- | --------------------- | ----------------- |
| **finish_reason + usage**   | ✅ 同一帧           | ❌ 拆分为两个帧       | ✅ 同一帧         |
| **usage 帧 choices**        | ✅ 非空             | ❌ 空数组 `[]`        | ✅ 非空           |
| **`: keep-alive` 注释**     | ❌ 无               | ⚠️ 有（newapi 注入）  | ❌ 无             |
| **`reasoning_content`**     | ⚠️ 有 (198 次)      | ⚠️ 有 (947 次)        | ✅ 无             |
| **`delta.role`**            | ✅ 始终有           | ⚠️ 部分缺失           | ❌ 无（不影响）   |
| **连接关闭 after [DONE]**   | 未确认              | 未确认                | ✅ 正常关闭       |
| **总 chunks**               | ~640                | ~2170                 | 458               |
| **流式耗时**                | ~4s                 | ~10s                  | ~17s              |
| **id 格式**                 | 时间戳              | `chatcmpl-`           | `cht000...@dx19f` |
| **usage 额外字段**          | `cached_tokens`     | 同                    | + `question_tokens` |
| **`finish_reason` 值**      | `stop`              | `stop`                | `stop`            |

## 三、SDK 流解析管线分析

### 3.1 四层管线架构

```
HTTP 响应流
    ↓
Layer 1: openai-go SDK SSE 解析器 (ssestream.go)
    — bufio.Scanner 逐行读取 + JSON Unmarshal
    ↓
Layer 2: Completions 适配器 (chat.go + stream.go)
    — ChatCompletionChunk → StreamEvent 转换
    — streamDrainTimeout (5s) + JSON 错误降级
    ↓
Layer 3: StreamConverter (bamboo/convert.go)
    — provider.StreamEvent → Anthropic 风格事件
    — 防御性自动补发 + 优先级 FinishReason
    ↓
Layer 4: Bamboo Chat 门面 (bamboo/bamboo.go)
    — peek 首事件 + ping keepalive + ctx 取消处理
```

### 3.2 Layer 1: openai-go SDK SSE 解析器 (ssestream.go)

这是实际的 SSE 字节级解析器，使用 `bufio.Scanner` 逐行读取 HTTP 响应体：

**行扫描与事件组装** (`ssestream.go:80-116`)：

- `s.scn.Scan()` 逐行读取
- 空行 (`len(txt) == 0`) 分发累积的事件
- `data:` 行用 `\n` 分隔符连接
- **断流点 A**：如果 GLM 发送 `data:` 帧后没有尾随空行，事件不会被分发直到下一个空行到达。如果 GLM 关闭连接时没有最后的 `\n\n`，最后一个事件会丢失。

**`[DONE]` 哨兵处理** (`ssestream.go:181-185`)：

```go
if bytes.HasPrefix(s.decoder.Event().Data, []byte("[DONE]")) {
    s.done = true
    continue  // 不 break，继续 drain
}
```

- **关键**：收到 `[DONE]` 后 SDK 不停止。设置 `s.done = true` 后继续循环，等待 `decoder.Next()` 返回 false（即 EOF/连接关闭）。
- **断流点 B（GLM 挂起）**：如果 GLM 发送 `data: [DONE]` 但不关闭 TCP 连接，`s.scn.Scan()` 永久阻塞。这就是 `chat.go:13-22` (`streamDrainTimeout`) 要修复的问题。

**JSON 反序列化** (`ssestream.go:216-219`)：

```go
s.err = json.Unmarshal(data, &nxt)
if s.err != nil {
    return false  // 流立即结束
}
```

- **断流点 C**：如果 GLM 发送截断/粘连/格式错误的 JSON 帧，`json.Unmarshal` 失败，`s.err` 被设置，`Next()` 返回 false。流死亡。错误通过 `stream.Err()` 浮现。
- 这正是 `chat.go:35-46` (`jsonErrKeywords`) 和 `chat_test.go:13-14` (GLM Coding-MAX Issue #66) 引用的 bug。

### 3.3 Layer 2: Completions 适配器 (chat.go)

**Channel 创建** (`chat.go:88`)：

```go
eventCh := make(chan provider.StreamEvent, 64)
```

缓冲 channel（64），吸收突发流量。

**流创建** (`chat.go:103-104`)：

```go
stream := p.Client.Chat.Completions.NewStreaming(ctx, params)
defer stream.Close()
```

无显式 HTTP 超时 — 依赖 openai-go SDK 默认值（除非注册了拦截器，否则无 `WithHTTPClient`，见 `provider.go:130-132`）。

**Drain 超时（GLM 挂起修复）** (`chat.go:106-124`)：

```go
const streamDrainTimeout = 5 * time.Second
// ...
startDrainTimer := func() { ... time.AfterFunc(streamDrainTimeout, func() { ... _ = stream.Close() }) }
```

在 `stopSent` 变为 true 时触发（收到 finish_reason，`chat.go:144-146`）。5 秒后如果连接仍未关闭，强制调用 `stream.Close()`。

**主消费循环** (`chat.go:131-155`)：

```go
for stream.Next() {
    // 首事件发送 Start (line 132-139)
    chunk := stream.Current()
    events := p.handleChunk(chunk, ...)
    if stopSent { startDrainTimer() }  // line 144-146
    for _, e := range events { select { case eventCh <- e: case <-ctx.Done(): return } }
}
```

每个 `stream.Next()` 阻塞直到 SDK 的 SSE 解析器产生下一个事件或错误。ctx 取消在每个 channel 发送时检查。

**循环后错误处理（GLM JSON 错误缓解）** (`chat.go:157-194`)：

```go
if !timedOut {
    if err := stream.Err(); err != nil {
        errKind := classifyStreamError(err)
        if errKind == errKindJSONParse && startSent {
            // 降级：合成 Stop 事件而非 Error
            if !stopSent { eventCh <- StreamEvent{Type: Stop, FinishReason: Stop} }
        } else {
            eventCh <- StreamEvent{Type: Error, Err: ...}
        }
    }
}
```

- `classifyStreamError` (lines 62-74) 检查错误消息是否包含 `jsonErrKeywords` (lines 39-46) 中的关键词
- 如果分类为 JSON 解析错误且已发送过内容 (`startSent`)，合成 `Stop` 事件 — 优雅降级
- **GAP**：如果 `stopSent` 已经为 true（finish_reason 在 JSON 错误前到达），不合成 Stop（line 172: `if !stopSent`）。来自 `handleChoice` 的现有 Stop 保持有效。
- **GAP**：如果 JSON 错误发生在任何内容之前（`startSent == false`），被视为致命 Error，不降级。

### 3.4 Layer 3: StreamConverter (bamboo/convert.go)

**事件路由** (`convert.go:316-335`)：

- `StreamTypeStart` → `handleStart()` (发出 `message_start`)
- `StreamTypeDelta` → `handleDelta()` (发出 `content_block_start`/`content_block_delta`)
- `StreamTypeStop` → `recordFinishReason()` 仅记录，**缓冲** finish reason，不发出事件
- `StreamTypeDone` → `handleStop()` (发出 `content_block_stop` + `message_delta` + `message_stop`)
- `StreamTypeError` → `handleError()` (发出 `error` + 自动 flush stop 序列)

**优先级 FinishReason** (`convert.go:343-363`)：

`tool_use(2) > max_tokens(1) > end_turn(0)` — 防止 stop 覆盖 tool_use。

**错误自动 flush** (`convert.go:796-812`)：

```go
func (sc *StreamConverter) handleError(err *xError.Error) []StreamEvent {
    events := []StreamEvent{{Type: EventError, Error: ...}}
    if sc.started && !sc.stopHandled {
        events = append(events, sc.handleStop()...)  // 自动发出 stop 序列
    }
    return events
}
```

即使出错，下游也会收到完整的 block 生命周期（content_block_stop + message_delta + message_stop）。

### 3.5 Layer 4: Bamboo Chat 门面 (bamboo/bamboo.go)

**Peek 首事件** (`bamboo.go:87-102`)：

```go
select {
case firstEvent, hasFirst = <-providerCh:
case <-ctx.Done(): return nil, ...
}
if firstEvent.Type == Error { return nil, error }
if firstEvent.Type == Done { return nil, "stream closed with no content" }
```

防止空流挂起：如果 provider 立即出错，同步返回。

**Converter goroutine** (`bamboo.go:108-183`)：

- `out := make(chan StreamEvent, 64)` — 缓冲输出 channel
- `pingTicker := time.NewTicker(10s)` — 空闲时发送 `EventPing`（line 155-158），防止代理 idle timeout
- `writeEvent` (line 130-138): select on `out <-` or `<-ctx.Done()`
- provider channel 关闭时 (line 161-164): 合成 `StreamTypeDone` flush converter
- ctx 中途取消 (line 166-176): 合成 `StreamTypeError` + flush converter

## 四、已发现的断流风险点

### 风险点 A：GLM Issue #66 — SSE JSON 帧截断/粘连

> 来源：[zai-org/GLM-5#66](https://github.com/zai-org/GLM-5/issues/66)

GLM-5.1 在长会话（50+ tool calls，~100k tokens 上下文）中间歇性出现 JSON 被截断在字段中间：

```
JSON parsing failed: Text: {"id":"20260430204055297b97348c8e426c","created":1777552855,"object":"chat.c
data: {"id":"2026043020420635c6abc56a2741ac","created":1777552926,"object":"chat.completion.chunk","model":"glm-5.1","choices":[{"index":0,"delta":{"role":"assistant","content":"11"}}]}
Error message: JSON Parse error: Expected '}'
```

第一个 JSON 对象在 `"object":"chat.c` 处被截断，然后下一个 `data:` 帧立即开始。

**根因**（来自 issue 报告者）：

1. 服务端 SSE framing bug — buffer flush 边界在 JSON 中间切分
2. JSON 序列化器写入部分输出后未等对象完成就 flush
3. Token 级 chunking 与 JSON 结构边界不匹配

**触发条件**：长会话（50+ tool calls，~100k tokens 上下文）。不是每次都触发，但频率足以成为可靠性阻断器。

**SDK 缓解措施**（`chat.go:62-74, 171-181`）：

- `classifyStreamError` 检查错误消息是否包含 JSON 解析关键词
- 如果是 JSON 解析错误且已发送过内容（`startSent == true`），合成 Stop 事件优雅终止

**缓解措施的 GAP**：

1. ❌ 如果 JSON 错误发生在**首个 chunk**（`startSent == false`），直接返回致命 Error
2. ❌ `jsonErrKeywords` 使用**子字符串匹配**，如果 GLM 的错误消息措辞不在列表中，被分类为 `errKindFatal`
3. ❌ 未使用 `errors.As(err, &json.SyntaxError{})` 等类型断言，仅靠消息文本匹配

### 风险点 B：GLM 不关闭 TCP 连接 after [DONE]

openai-go SDK 的 `ssestream.go:181-185` 在收到 `data: [DONE]` 后：

```go
if bytes.HasPrefix(s.decoder.Event().Data, []byte("[DONE]")) {
    s.done = true
    continue  // 不 break，继续等待连接关闭
}
```

SDK 设置 `s.done = true` 后继续循环，等待 `s.scn.Scan()` 返回 false（即 EOF/连接关闭）。如果 GLM 不关闭 TCP 连接，`Scan()` 永久阻塞。

**SDK 缓解措施**（`chat.go:13-22, 110-124`）：

- `streamDrainTimeout = 5 * time.Second`
- 在 `finish_reason` 到达后启动 5s 定时器，超时后强制 `stream.Close()`

**缓解措施的 GAP**：

1. ❌ 定时器仅在 `stopSent == true` 时启动 — 如果 JSON 错误发生在 `finish_reason` 之前，`stopSent` 永远不为 true，定时器不启动
2. ⚠️ 5 秒可能不够 — 在高延迟网络（SDK → newapi → GLM）下，usage chunk 和 [DONE] 的到达可能超过 5s

### 风险点 C：非标准 `finish_reason` 值

> 来源：[BerriAI/litellm#23386](https://github.com/BerriAI/litellm/issues/23386)

GLM 在推理过程中异常终止时，不返回 HTTP 错误码，而是在 `finish_reason` 中返回非标准值：

| GLM 值           | OpenAI 等价       | SDK 处理                        |
| ---------------- | ----------------- | ------------------------------- |
| `network_error`  | `stop`            | 取决于 `mapFinishReason` 实现   |
| `sensitive`      | `content_filter`  | 取决于 `mapFinishReason` 实现   |

如果 `mapFinishReason` 不识别这些值，可能返回空字符串或错误值，导致下游处理异常。

### 风险点 D：newapi 的 `choices:[]` usage 帧 + `: keep-alive` 注释

newapi 中转将 `finish_reason` 和 `usage` 拆分为两个独立 SSE 帧，usage 帧的 `choices` 为空数组 `[]`。

SDK 处理（`stream.go:14-30`）：

```go
if chunk.Usage.TotalTokens > 0 || ... {
    events = append(events, /* UsageDelta */)  // 正确提取 usage
}
for _, choice := range chunk.Choices {         // choices 为空，循环不执行
    events = append(events, p.handleChoice(choice, ...))
}
```

表面看没问题，但存在时序问题：

1. `finish_reason:stop` 帧 → `stopSent = true` → 启动 5s drain 定时器
2. usage 帧（`choices:[]`）→ 只提取 usage，不影响 `stopSent`
3. `[DONE]` → SDK 设置 `s.done = true`，继续等待连接关闭
4. 如果 newapi 在 `[DONE]` 后不关闭连接 → 5s 后强制关闭

但：如果 `: keep-alive` 注释行被 openai-go SDK 的 `bufio.Scanner` 误解为数据行，可能导致 JSON 解析错误（`: keep-alive` 不是合法 JSON）。这会触发风险点 A 的降级逻辑。

### 风险点 E：无 HTTP 超时配置

`provider.go` 中未设置任何 HTTP 客户端超时：

- 无 `http.Client.Timeout`
- 无 `http.Transport.ResponseHeaderTimeout`
- 无 `http.Transport.IdleConnTimeout`
- 唯一的超时是 `streamDrainTimeout`（5s，仅在 `finish_reason` 后生效）

如果 GLM 在 thinking 阶段长时间无输出（数分钟），SDK 没有任何超时保护。只有 `bamboo.go` 的 10s ping ticker 防止上游代理 idle timeout。

### 风险点 F：`stream_options` 不兼容

`params.go:226-232`：

```go
func (p *CompletionsProvider) buildStreamOptions() openai.ChatCompletionStreamOptionsParam {
    if p.legacyCompat {
        return openai.ChatCompletionStreamOptionsParam{}  // Legacy: 省略
    }
    return openai.ChatCompletionStreamOptionsParam{
        IncludeUsage: openai.Bool(true),  // 默认: 发送
    }
}
```

如果用户未启用 `WithLegacyCompat()`，SDK 发送 `stream_options.include_usage: true`。GLM 直连端点不支持此参数，返回 **400 code:1210**。

中转 newapi 可能兼容 `stream_options`（会忽略或转换），所以通过 newapi 可能不会触发此问题。这解释了为什么直连 GLM 和中转 GLM 的行为不同。

### 风险点 G：Tool Call arguments 裸文本泄漏

> 来源：[Eric's Blog - GLM-5 流式 Tool Call 异常输出](https://wsdjeg.net/glm5-streaming-toolcall-analysis/)

GLM 在流式 Tool Call 场景下，arguments 的 token 片段绕过 SSE 封装层，直接以裸文本写入 HTTP 响应流：

```
data: {"id":"chat-456","choices":[{"delta":{"tool_calls":[{"id":"call_1","function":{"name":"read_file","arguments":""},"index":0}]}]}
ts from you under this License for any purpose whatsoever, with or without
data: {"id":"chat-456","choices":[{"delta":{"tool_calls":[{"function":{"arguments":"\"--filepath\""},"index":0}]}]}
```

第 2-3 行是没有 `data:` 前缀的裸文本。openai-go SDK 的 `bufio.Scanner` 会将裸文本行视为非 `data:` 行，跳过或导致解析异常。

**触发条件**：每次 Tool Call 必现（100% 复现率）。

## 五、根因判定

### 5.1 为什么 GLM 断流而讯飞不断流

| 因素                         | GLM               | 讯飞       |
| ---------------------------- | ----------------- | ---------- |
| SSE JSON 帧截断（Issue #66） | ✅ 有此 Bug       | ❌ 无      |
| 非标准 finish_reason         | ✅ 有             | ❌ 无      |
| Tool Call 裸文本泄漏         | ✅ 有此 Bug       | ❌ 无      |
| 不关闭 TCP after [DONE]      | ✅ 有此行为       | ❌ 正常关闭|
| thinking 阶段长时间无输出    | ✅ 数分钟         | ❌ 较短    |
| `reasoning_content` 非标准   | ✅ GLM 独有       | ❌ 无      |
| `stream_options` 不兼容      | ✅ 400 code:1210  | ❌ 兼容    |

讯飞之所以不断流，是因为讯飞的 SSE 输出严格遵循 OpenAI 标准格式：每个 `data:` 行包含完整 JSON、`finish_reason` 使用标准枚举值、Tool Call arguments 正确封装在 SSE 帧内、连接在 `[DONE]` 后正常关闭。

### 5.2 最可能的断流根因（按概率排序）

| 排名 | 根因                                        | 概率 | 触发条件                           |
| ---- | ------------------------------------------- | ---- | ---------------------------------- |
| 1    | GLM Issue #66：SSE JSON 帧截断/粘连         | 高   | 长会话（50+ tool calls）、高负载   |
| 2    | `streamDrainTimeout` 不覆盖 JSON 错误场景   | 中高 | JSON 错误在 `finish_reason` 之前   |
| 3    | `classifyStreamError` 关键词不匹配          | 中   | GLM 错误消息措辞变化               |
| 4    | GLM 不关闭 TCP after [DONE] + 超时不够      | 中   | 网络延迟、newapi 缓冲              |
| 5    | `stream_options` 不兼容（未启用 legacy）    | 低   | 直连 GLM 未设 legacyCompat         |
| 6    | Tool Call 裸文本泄漏                        | 低   | 仅 Tool Call 场景                  |
| 7    | 非标准 finish_reason 导致映射失败           | 低   | 推理异常终止时                     |

### 5.3 关键证据链

1. **curl 直连 GLM 和 newapi 都不断流** → 问题不在网络层，在 SDK 解析层
2. **SDK 已有 `streamDrainTimeout` 和 `classifyStreamError` 两个 GLM 专用缓解措施** → 说明此前已遇到过这些问题
3. **缓解措施有 GAP**（`startSent` 条件、关键词匹配、定时器启动条件） → 间歇性断流说明某些场景未被覆盖
4. **GLM Issue #66 是服务端 Bug** → 间歇性、长会话触发 → 与用户描述的"断流"高度吻合
5. **讯飞无此类服务端 Bug** → 解释了讯飞不断流

### 5.4 newapi 中转引入的额外问题

newapi 中转在 GLM 和 SDK 之间增加了一层转换，引入了新的不兼容：

1. **`finish_reason` 和 `usage` 拆分**：newapi 将 GLM 的合并帧拆为两个帧，usage 帧的 `choices` 为空数组。虽然 SDK 的 `handleChunk` 能处理（先提取 usage，再循环 choices），但这改变了 `stopSent` 的时序 — `finish_reason` 先到达触发 drain timer，usage 帧到达时 drain timer 已在运行。

2. **`: keep-alive` 注释行**：newapi 注入 SSE 注释行维持连接。openai-go SDK 的 `bufio.Scanner` 通常会跳过非 `data:` 行，但如果注释行出现在 `data:` 帧中间（帧粘连场景），可能导致解析异常。

3. **`model` 字段变化**：GLM 直连返回 `glm-4.5`，newapi 返回 `xopglm52`（内部映射名）。这不影响流式解析，但可能影响下游日志和计费。

4. **chunks 数量暴增**：GLM 直连 ~640 chunks，newapi ~2170 chunks。newapi 对内容做了更细粒度的分片（每帧 1-2 个字符），这增加了 SDK 的处理开销和帧粘连的概率。

## 六、SDK 兼容性问题清单

### 6.1 确认存在的兼容性问题

| #    | 问题                                                        | 文件:行号              | 严重度 | 状态             |
| ---- | ----------------------------------------------------------- | ---------------------- | ------ | ---------------- |
| C1   | `classifyStreamError` 使用子字符串匹配而非类型断言          | `chat.go:62-74`        | 高     | 已有缓解但有 GAP |
| C2   | JSON 错误降级仅在 `startSent == true` 时生效                | `chat.go:171`          | 高     | 首帧错误无法降级 |
| C3   | `streamDrainTimeout` 仅在 `stopSent == true` 时启动         | `chat.go:144-146`      | 高     | finish_reason 前错误无超时 |
| C4   | 无全局 HTTP 超时                                            | `provider.go`          | 中     | thinking 阶段可能永久阻塞 |
| C5   | `streamDrainTimeout` 固定 5 秒不可配置                      | `chat.go:22`           | 中     | 高延迟场景不够   |
| C6   | `mapFinishReason` 可能不识别 `network_error`/`sensitive`    | `stream.go:84`         | 中     | 需确认实现       |
| C7   | 无 SSE 注释行 (`: keep-alive`) 显式处理                     | openai-go SDK          | 低     | 通常 bufio.Scanner 会跳过 |
| C8   | 无 Tool Call 裸文本行过滤                                   | openai-go SDK          | 低     | 仅 Tool Call 场景 |

### 6.2 上游不合理行为导致的 SDK 不可用场景

| 场景                      | 上游行为                          | SDK 影响                       | 当前缓解                              |
| ------------------------- | --------------------------------- | ------------------------------ | ------------------------------------- |
| 长会话 GLM 流式           | SSE JSON 帧截断                   | `json.Unmarshal` 失败 → 流中断 | `classifyStreamError` 降级（有 GAP）  |
| GLM `[DONE]` 后不关 TCP   | 连接保持打开                      | `stream.Next()` 永久阻塞       | `streamDrainTimeout` 5s（仅限 finish_reason 后） |
| GLM thinking 阶段长时间无输出 | 数分钟无 SSE 帧                   | 无超时保护，goroutine 挂起     | `bamboo.go` 10s ping ticker（仅防代理 timeout） |
| GLM 推理异常终止          | `finish_reason: "network_error"`  | `mapFinishReason` 可能返回无效值 | 无                                    |
| GLM Tool Call 流式        | arguments 裸文本泄漏              | 行解析失败 / 数据丢失          | 无                                    |
| GLM Coding 端点           | 不支持 `stream_options`           | 400 code:1210                  | `legacyCompat` 模式（需用户手动启用） |

## 七、建议修复方向

### 7.1 短期修复（高优先级）

1. **改进 `classifyStreamError`**：使用 `errors.As(err, &json.SyntaxError{})` 和 `errors.As(err, &json.UnmarshalTypeError{})` 替代子字符串匹配
2. **移除 `startSent` 条件**：JSON 解析错误在任何情况下都应降级为合成 Stop（而非致命 Error）
3. **启动 drain timer 的条件改为 `startSent`**：不仅限于 `stopSent`，只要开始接收数据就启动超时
4. **使 `streamDrainTimeout` 可配置**：通过 Option 允许用户设置更长的超时

### 7.2 中期修复

5. **添加全局 HTTP 超时**：在 `provider.go` 中设置 `http.Client.Timeout` 或通过 Option 配置
6. **扩展 `mapFinishReason`**：处理 `network_error` → `FinishReasonStop`，`sensitive` → `FinishReasonContentFilter`
7. **添加 SSE 注释行过滤**：在 openai-go SDK 之上或通过拦截器过滤 `:` 开头的行

### 7.3 长期修复

8. **实现 SSE 帧重组缓冲**：在 JSON 解析失败时，将当前帧与下一帧拼接后重试解析（处理 Issue #66 的帧粘连）
9. **添加 Tool Call 裸文本检测**：跳过不以 `data:` 开头的行（处理 GLM Tool Call 泄漏）
10. **自动检测 GLM 端点并启用 legacyCompat**：基于 BaseURL 模式匹配自动启用

## 八、参考链接

### 官方文档

- [流式消息 - 智谱AI开放文档](https://docs.bigmodel.cn/cn/guide/capabilities/streaming) — SSE 格式规范
- [OpenAI API 兼容 - 智谱AI开放文档](https://docs.bigmodel.cn/cn/guide/develop/openai/introduction) — 兼容性说明
- [对话补全 API - 智谱AI开放文档](https://docs.bigmodel.cn/api-reference/模型-api/对话补全) — OpenAPI 规范
- [使用概述 - 智谱AI开放文档](https://docs.bigmodel.cn/cn/api/introduction) — Coding 端点说明
- [Thinking Mode 文档](https://zhipu-32152247.mintlify.app/guides/capabilities/thinking-mode) — Preserved Thinking
- [Z.AI 英文流式文档](https://docs.z.ai/guides/capabilities/streaming) — 国际版
- [阿里云百炼 GLM 文档](https://help.aliyun.com/zh/model-studio/glm-zhipu) — `tool_stream` 参数

### Bug 报告

- [zai-org/GLM-5#66](https://github.com/zai-org/GLM-5/issues/66) — **SSE JSON 截断**（最关键）
- [BerriAI/litellm#23386](https://github.com/BerriAI/litellm/issues/23386) — **非标准 finish_reason**
- [can1357/oh-my-pi#1494](https://github.com/can1357/oh-my-pi/issues/1494) — **流式超时 stall**
- [Eric's Blog](https://wsdjeg.net/glm5-streaming-toolcall-analysis/) — **Tool Call 裸文本泄漏**
- [vllm-project/vllm#39757](https://github.com/vllm-project/vllm/issues/39757) — **流式 tool name 截断**
- [songquanpeng/one-api#1451](https://github.com/songquanpeng/one-api/issues/1451) — **流式回答截断**
- [Vercel AI#11682](https://github.com/vercel/ai/issues/11682) — **reasoning_content 未解析**
- [Hermes Agent#16533](https://github.com/NousResearch/hermes-agent/issues/16533) — **thinking 参数格式差异**

### 第三方集成参考

- [Mastra - Zhipu AI Coding Plan](https://mastra.ai/models/providers/zhipuai-coding-plan) — 模型列表
- [eliteai.tools - GLM Coding skill](https://eliteai.tools/agent-skills/glm-coding) — 端点格式说明
- [apidog.com - GLM-5.1 API Guide](https://apidog.com/blog/how-to-use-glm-5-1-api/) — 兼容性确认

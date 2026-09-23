# 0001 · OpenAI Responses API WebSocket 模式技术调研（核验修订版）

> 调研与核验日期：2026-09-24  
> 状态：文献核验完成；服务端行为和性能待实测  
> 范围：当前官方在线文档描述的 Responses WebSocket 模式；工程可行性见 [0002](./0002-research-websocket-sse-transcoding.md)。

## 背景与证据口径

核心问题是：Responses WebSocket 提供哪些传输和会话能力，使用它需要承担哪些状态管理责任？以 Responses HTTP/SSE 为对照，收敛到接口行为、缓存、并发、恢复及 Steering。

本文区分三类结论：**确定**用于直接可核对的规范关系或源码事实；**倾向**用于单源权威文档和工程推论；**存疑**用于未经复现的性能收益或覆盖范围。OpenAI 的多个页面属于同一发布主体，页面间一致性校验不能充当独立性能背书。动态页面未展示发布日期或版本号时，仅记录访问日期。

## 结论速览

1. **倾向（官方文档明确）**：Responses WebSocket 在持久连接上以 `response.create` 发起每轮请求，以 `previous_response_id` 续接历史，当前文档还支持 `stream_id` 多路复用。[O1]
2. **倾向（官方文档明确）**：连接本地缓存兼容 `store=false` 和 Zero Data Retention（ZDR，零数据保留）；恢复能力取决于历史是否仍可解析，应用需准备完整上下文或压缩窗口。[O1][O3]
3. **存疑（收益未独立复现）**：官方宣称在 20 次以上工具调用的工作流中观察到“up to roughly 40% faster end-to-end execution”；这是特定工作负载的上限描述，不能推定本项目固定降低 40% 耗时。[O1]

## 分维度发现

### 1. 连接与请求

官方以 `/v1/responses` 为持久连接入口。每轮发送 JSON `response.create`，载荷沿用 Responses 创建请求的主要字段；WebSocket 请求省略 `stream` 和 `background`，`stream_id` 仅用于 WebSocket。[O1][O2]

普通两轮交互如下（模型和 ID 为示意，未执行）：

```text
建立 WebSocket
  → response.create(model, input, tools, store=false, stream_id="main")
  ← response.created / 内容与工具增量 / response.completed
执行客户端工具，保存其结果
  → response.create(previous_response_id=上一轮ID, input=新增工具结果,
                    model, tools, store=false, stream_id="main")
  ← 下一轮事件
```

`function_call_output.call_id` 对应上轮函数调用的 `call_id`。下一轮携带需要的 tools、instructions 等请求配置，不能仅凭历史 ID 假设所有配置自动继承。Steering 自动继任响应有单独的继承规则。[O1][O4]

`generate:false` 可预热请求状态，不产生模型输出，并返回可供后续引用的响应 ID。其收益、计费和具体延迟需要单独验证。[O1]

当前指南给出的安装示例为 Python `openai[realtime]>=3.8.0`、JavaScript `openai@^7.10.0 ws` 及 Ruby `openai async-websocket`。这些是页面当前示例依赖约束；其他语言的 SDK 支持情况需要检查具体版本仓库，本文撤回“Go SDK 尚无实现”的未核实结论。[O1]

### 2. 多路复用与容量

以下均来自当前指南，置信度为倾向（单源权威）。[O1]

| 项目 | 当前文档行为 |
| --- | --- |
| `stream_id` | 1–256 字符，允许字母、数字、下划线、连字符、句点；空字符串无效 |
| 默认通道 | 省略 `stream_id`；返回事件也省略该字段 |
| 同通道请求 | FIFO 串行执行 |
| 不同通道请求 | 可以并行，返回事件交织 |
| 活跃响应上限 | 单连接 16 个，涵盖命名通道及默认通道；更多请求排队 |
| 命名通道上限 | 单连接累计 32 个不同名称，默认通道不计入 |
| 连接时长 | 最长 60 分钟，需要重连 |

`stream_id` 决定调度和事件路由，`previous_response_id` 决定历史关系。复用通道名称并省略历史 ID，会开始新的响应链。单一读取循环按通道分发是文档给出的使用方式；工程实现还需保证写入串行化、队列有界和慢消费者隔离。

### 3. 缓存、分叉与恢复

活跃连接内保留近期响应状态，每个通道保留最新缓存响应。文档没有承诺零延迟，也没有公开所有内部存储与调度路径。[O1]

| 场景 | 行为 |
| --- | --- |
| 父响应在缓存中 | 可以利用连接本地状态续接 |
| 缓存未命中，`store=true` | 在持久状态仍可用时，可能恢复历史 |
| 缓存未命中，`store=false` / ZDR | 返回 `previous_response_not_found` |
| 同通道续接返回 4xx/5xx | 驱逐被引用的父响应缓存 |
| 跨通道分叉出错 | 保留共享父响应，使源通道可继续 |
| 连接断开 | 全部通道的连接本地缓存消失 |

跨通道分叉使用新 `stream_id` 引用已完成的父响应。在 `store=false` 下，等待分支收到 `response.in_progress` 后再推进源通道，可避免排队期间父缓存被替换的竞争；失败时可以使用完整上下文启动新链。[O1]

恢复输入需要保存原始输入、可重放输出、工具调用及结果、推理条目与相关元数据。仅存聊天可见文本可能丢失恢复所需信息。网关自行存储的日志、历史和缓存也需满足自己的合规要求；OpenAI 的 ZDR 兼容声明不能覆盖整条网关链路。[O1][O3]

### 4. 上下文压缩

- **服务端压缩**：`context_management` 配置 `compact_threshold`，后续继续引用最新响应 ID并发送新增输入。[O1]
- **独立压缩端点**：`/responses/compact` 返回压缩后的输入窗口；使用完整返回窗口加新输入启动新链，省略或置空 `previous_response_id`，保留返回窗口内容。[O1]

这两条路径需要分开实现。压缩窗口不能被解释成已完成响应 ID。

### 5. Mid-turn Steering 的完整生命周期

当前指南将 Steering 支持范围描述为 GPT-6 系列，GPT-5.6 及更早模型不支持；具体模型和请求参数组合仍可能返回 `steering_not_supported`。[O4]

收到目标响应的 `response.created` 后，可在同一连接发送 `response.steer`，仅包含 `type`、`previous_response_id` 和 `input`。输入限用户消息及支持的内容类型。Steering 保留已完成工作，不撤销已经执行的动作。[O2][O4]

```text
response.steer
  → response.steer.accepted：输入已排队
  → 原响应结束：可能 incomplete(reason="steered")，也可能正常 completed
  → 自动继任响应 response.created：已接收输入的提交点
  → 继任响应生成与完成
```

若需要客户端工具结果或审批，可能先发 `response.steer.pending`，附 `required_input`。客户端按同一父响应及通道提交一次 `response.create`，补齐结果；服务端按顺序合并已接受的 Steering 输入。已接受输入无需重复发送，同一父响应的多个 pending 也不意味着重复执行工具。若结果已先送达，pending 通知可能省略。[O2][O4]

`response.steer.failed` 返回未提交输入，便于纠正后重试。断线或缺少确认属于结果未知，需要核对历史，避免重复提交。自动继任继承原请求设置，显式续接使用自己的设置。处理循环需识别继任响应，不能在第一个 completed/incomplete 后无条件结束。[O4]

### 6. 错误与运行治理

当前指南列出 `previous_response_not_found`、`invalid_stream_id`、`websocket_stream_limit_reached`、`websocket_connection_limit_reached`。[O1]

有命名通道关联的请求错误带 `stream_id`，其他通道可以继续；缺少该字段的错误也可能是无效通道参数，不能仅凭字段缺失判为连接致命错误。

工程考虑点包括：握手超时、写入超时、消息大小限制、按租户隔离、断连后的未知执行结果、工具结果去重和排空连接策略。具体轮换时间、Ping 周期及代理空闲超时取决于部署环境，本文撤回固定“50–55 分钟”和“所有代理默认 60 秒”的泛化建议。

## 对比矩阵

| 维度 | Responses HTTP/SSE | Responses WebSocket |
| --- | --- | --- |
| 请求入口 | 每轮 HTTP 请求；底层连接可复用 | 持久连接中反复发送 `response.create` |
| 返回模型 | Responses 流式事件 | 复用基础事件模型，含通道信息及专用控制事件 |
| 续接 | `previous_response_id` 或手动历史 | 同样支持，并有连接本地缓存路径 |
| 并发 | 多个 HTTP 请求 | 当前指南支持通道并发 |
| 中途指令 | 需要另行设计控制入口 | 支持模型可用 `response.steer` |
| 成本与性能 | 需按场景测量 | 增量上传减少续接开销，实际收益需测量 |

HTTP 的连接复用意味着每轮必定新建 TCP/TLS 连接的说法不成立。官方也未公开 HTTP 每轮必定读磁盘的实现。`previous_response_id` 链中历史输入仍参与输入 token 计费，网络传输减少不等于历史 token 免费。[O3]

## 核验修订记录

- 将“降低约 40% 耗时（确定）”改为官方有条件的性能自述，撤回本项目收益保证。
- 删除“零延迟命中”“HTTP 每轮必读持久存储”和完整网关天然满足 ZDR 的结论。
- 补齐跨通道错误保留父缓存、Steering 提交点及 pending 可省略等边界。
- 删除缺少证据的 Realtime 能力否定、Go SDK 覆盖断言及固定实施建议。
- 来源日期改为访问日期；原报告中的“2026-09”不能作为网页发布时间证据。

## 证据索引与局限

所有页面访问日期为 **2026-09-24**，在线指南及参考未提供固定文档版本、发布日期。O1 本轮重新抓取；O2–O4依据同会话已抓取正文核对。以下均为官方一手信息，仍属同一发布主体。

| 编号 | 来源 | 用途 |
| --- | --- | --- |
| O1 | [WebSocket Mode](https://developers.openai.com/api/docs/guides/websocket-mode) | 请求、并发、缓存、恢复及性能自述 |
| O2 | [WebSocket Events](https://developers.openai.com/api/reference/resources/responses/websocket-events) | 事件结构、Steering 提交语义 |
| O3 | [Conversation State](https://developers.openai.com/api/docs/guides/conversation-state) | 历史重放、缓存和计费 |
| O4 | [Mid-turn Steering](https://developers.openai.com/api/docs/guides/steering) | 模型范围、自动继任、工具结果及失败恢复 |

本次完成文献核对，没有执行 OpenAI 真实 WebSocket 会话、ZDR 账户验证或性能基准。多路复用首次上线日期、各语言 SDK 精确支持版本、模型逐项兼容性及部署代理限制仍待专项取证。

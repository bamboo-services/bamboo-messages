# 0002 · Responses WebSocket 发布时间与双向 SSE 转换可行性（核验修订版）

> 调研与核验日期：2026-09-24  
> 状态：文献与源码核验完成；端到端互操作及性能待验证  
> 源码基线：本地 HEAD `0aeec95`；本次核验只修订文档  
> 承接：[0001 · WebSocket 模式技术调研](./0001-research-openai-websocket-mode.md)

## 背景与问题范围

回答三个问题：何时发布 Responses WebSocket；能否将 OpenAI WebSocket 上游转换成 SSE 下游；能否使用本项目将其他厂商 SSE 上游转换成对外 Responses WebSocket 服务。

约定：**上游**是模型服务，**下游**是调用本网关的客户端。SSE 是 HTTP 事件流的封装规范，Responses、Chat Completions、Anthropic 和 Gemini 则各有载荷语义。传输封装、事件转换和会话状态三层分别核验。

置信度沿用 0001：确定（规范/源码直接事实）、倾向（单源权威或工程推论）、存疑（缺少证据/实测）。文献可行性与实现完成度分别报告。

## 结论速览

1. **倾向，单源权威明确记录**：OpenAI Changelog 在 **2026-02-23** 写明上线 Responses WebSocket 模式；当前多路复用能力的首次发布日期尚未查明。[O1]
2. **倾向，协议层可行**：OpenAI WebSocket → Responses SSE 可以转封装，转换到 Anthropic/Completions SSE 还需语义映射；项目尚缺 WebSocket 客户端传输与会话管理。[O2][O3][L1][L2]
3. **倾向，有限能力可实现**：其他厂商 SSE → 对外 Responses WebSocket 可复用本项目的基础转换链；完整兼容还需 WebSocket 服务端、会话历史、ID映射、调度、控制事件及兼容性测试。[L1–L5]

## 1. 发布时间核验

| 结论 | 证据与裁定 |
| --- | --- |
| Responses WebSocket 发布于 2026-02-23 | O1 对应日期明确写有 “Launched WebSocket mode for the Responses API”；保留 |
| `stream_id` 多路复用在首发当天就支持 | O1 首发条目未列该能力；O2 仅证明当前支持；撤回首发归因 |
| 2026-09-03 引入 Mid-turn Steering | O1 当日条目明确列出；保留 |
| 同日 Async Tool Calling 等于 WebSocket 独占升级 | O1 将其列为 Responses 新能力；仅凭该条目无法推出传输独占性；撤回 |
| 2026-09-22 Sol/Luna 发布等于当天扩展全部 WS 控制能力 | O1 确认模型发布；精确能力组合需另查；撤回 |

以上日期针对 Responses API 的 WebSocket 产品能力。当前指南是动态文档，访问日期为 2026-09-24；不能将当前内容倒推成首发能力清单。Assistants 下线等旁支时间线从本文移除。

## 2. 三条链路必须分别评估

```text
A. HTTP客户端 → 网关 → OpenAI Responses WebSocket
   HTTP客户端 ← Responses或其他格式SSE ← 网关 ← WebSocket事件

B. Responses WebSocket客户端 → 网关 → Anthropic / Completions / Gemini HTTP接口
   Responses WebSocket客户端 ← Responses JSON事件 ← 网关 ← 厂商SSE事件

C. Anthropic / Completions HTTP客户端 → 网关 → OpenAI Responses WebSocket
   客户端 ← 原协议SSE ← 网关 ← WebSocket事件
```

用户第二个转换问题主要对应 B。旧稿主要展开 C，遗漏了 B 的 WebSocket 服务端与历史管理责任，本版补齐。

| 路径 | 协议可行性 | 当前项目支持程度 | 核心新增工作 |
| --- | --- | --- | --- |
| A：OpenAI WS → SSE | 基础响应可转封装/映射 | 具备部分事件转换，无 WS 传输 | WS 客户端、请求关联、输出路由、错误及断连治理 |
| B：其他 SSE → Responses WS | 文本/函数工具等共同能力可适配 | 具备上游 Provider 和 Responses SSE 序列化器 | WS 服务端、会话存储、事件出口、能力拒绝策略 |
| C：其他请求 → OpenAI WS | 共同请求字段可映射 | 请求 codec 与 Responses 参数构建可参考 | WS 上游、显式会话映射、可选安全增量化 |

所有路径均处于“工程上可实现，尚未完成 WS 互操作验证”。撤回百分比复用度和周数估算。

## 3. 路径 A：OpenAI WebSocket 上游转为 SSE

### 3.1 同协议基础事件转换

Responses 基础响应事件在两种传输中复用事件模型；WebSocket 还有 `stream_id` 和 Steering 控制事件。[O2]

对选定通道、选定响应的普通事件，可以将 JSON 用 UTF-8 写成 SSE：

```text
event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_demo","output_index":0,"content_index":0,"delta":"Hello","sequence_number":3}

```

示例是封装说明。真实转换需保留完整原始载荷字段；专用于上游路由的 `stream_id` 可以在拆分到独立 HTTP 响应后按接口契约移除。一个 SSE 响应只接收归属自己的事件，先完成分流再写出。[O2][O3]

若目标是 Chat Completions 或 Anthropic，下游期待各自事件模型：工具调用需要 ID、名称、参数增量及结束原因；`response.completed` 也可能包含工具调用，不能一律映射成 `finish_reason:"stop"`。这需要有状态转换，并逐项核对元数据。[L2][L5]

### 3.2 HTTP 与客户端约束

- HTTP 返回 `Content-Type: text/event-stream`，事件以空行分隔，及时刷新，核验反向代理缓冲。[O3]
- 原生浏览器 `EventSource` 构造器提供 URL 和凭据选项，不能直接承载带 JSON 请求体、自定义 Authorization 的 POST；通常需使用 Fetch 流式读取或另设创建/订阅接口。[O3]
- `sequence_number` 是 JSON 业务字段。SSE `id:` / `Last-Event-ID` 的续传需要自己的事件记录与重放契约。[O3]
- 响应头提交前可返回 HTTP 错误；提交后需发目标协议错误事件并结束该响应。
- SSE 下游断开只取消/解绑对应任务；直接关闭共享上游 WS 会影响其他任务。对上游仍在执行的任务需要排空或隔离策略，不能无依据假定存在取消帧。
- 每个响应要处理 completed、failed、incomplete、协议 error 及提前断线。Steering 自动继任需要单独生命周期，普通“一次 HTTP 请求对应一个响应”的契约需明确限制或扩展。

因此“所有客户端零改造”“唯一限制是单向”均撤回。基础兼容条件是下游理解目标载荷、认证、错误和终态。

## 4. 路径 B：其他厂商 SSE 上游转为 Responses WebSocket 服务

### 4.1 可复用链路与新增边界

```text
新增 WS Handler：认证、Upgrade、读取 response.create、验证字段
  → 新增会话层：解析 previous_response_id、补齐历史、分配响应ID
  → 已有 Responses request codec：共同能力转 RelayRequest
  → 已有 BambooClient / Provider：调用选定厂商 HTTP/SSE
  → 已有 Provider事件转换 → Bamboo StreamConverter
  → 已有 Responses 序列化逻辑（需要适配事件出口与终态）
  → 新增 WS Writer：逐条 JSON 事件、通道字段与发送队列
```

`RelayStream` 当前入参是请求 JSON、Provider 和格式；出参是 SSE 字节 channel。它没有接受任意上游 SSE 字节流的通用反序列化入口，输入 SSE 的解析在具体 Provider 中完成。若调用者已有外部 SSE 流，还需暴露或增加对应解析入口。[L1][L2]

Responses 序列化器一次 `Serialize` 可能拼接多个 SSE 事件。概念验证可用可靠 SSE 解析器提取每个 `data` JSON，再逐事件发送 WS；长期考虑点是让事件构建与 SSE/WS 封装分开。简单去掉 `data:` 前缀会丢失多事件边界、注释及多行数据语义。[O3][L3]

### 4.2 历史与 ID 是可续接服务的必要条件

其他上游没有能力解析网关自行生成的 `resp_*`。网关必须拥有：

1. 按认证主体隔离的 `response_id → 父响应、输入、输出、工具调用、结果、模型路由` 记录。
2. `previous_response_id` 所指完整历史的还原和权限检查；未找到或已过期时明确报错。
3. 对通用 Provider 的全量消息重建；按各厂商工具格式转换，并验证 `call_id` 对应关系。
4. 分叉时复用不可变父快照，分别记录两个分支输出，避免共享可变历史串扰。
5. 自己明确的存储与恢复契约，包括 `store=false` 内存生命周期、容量、过期、重连策略。

可以先实现“单通道、基础文本/函数工具、全量输入”的有限服务；只有完成历史管理后，才可声明支持增量 `previous_response_id`。生成合法外形的 ID 不代表历史已经可检索。

### 4.3 可提供与需另行实现的能力

| 能力 | 核验结论 |
| --- | --- |
| 文本增量、普通函数工具调用与结果 | 已有映射基础，需 WS 多轮互操作测试 |
| usage、停止原因、工具 ID | 有通用字段，终态与来源含义需校验 |
| 明文推理、摘要 | 有映射路径，但语义和输出范围可能有损 |
| 加密推理内容、跨厂商签名 | 只能在适用来源链路中保留；无法由代理生成等价模型内部状态 |
| `stream_id` FIFO/并发 | 可由网关调度实现；当前没有该层 |
| `generate:false` | 可保存网关请求状态；是否符合客户端期待的预热语义需验证 |
| 原生托管工具、压缩、Steering | 需上游支持或独立编排实现，不能只换事件名称 |
| `store=true`、检索、持久恢复 | 需明确服务范围并增加存储/接口 |
| 官方连接本地推理加速 | 其他 SSE 上游仍使用自己的计算与缓存路径，WS 下游不会自动获得 OpenAI 的内部优化 |

未知能力进入执行链前需要验证并明确拒绝；现有 codec 忽略未知字段/条目的行为不足以充当完整兼容服务的能力校验。

## 5. 本项目源码核验结果

以下是 `0aeec95` 工作区源码直接事实；链接指向相对文件，符号名提供复查入口。

| 来源 | 已有能力 | 可行性限制 |
| --- | --- | --- |
| [L1：codec.go](../../../bamboo/codec/codec.go)，`Codec` / `StreamSerializer` | 解析请求、序列化统一响应/SSE | 无通用入站响应流解码方法，无双向会话 API |
| [L2：relay.go](../../../bamboo/relay/relay.go)，`RelayStream` | ParseRequest → BambooClient.Chat → serializer | 无 WS Upgrade、连接池、历史库；每轮是独立调用 |
| [L3：responses/stream.go](../../../bamboo/codec/responses/stream.go)，`marshalSSE*` | 创建 Response 事件，维护 ID、序号与输出项 | 写死 SSE 封装；一轮内有多个事件；没有 WS 控制事件处理 |
| [L4：responses/request.go](../../../bamboo/codec/responses/request.go)，`responsesRequest` / `parseInput` | 解析 `previous_response_id`、`store`、函数工具、部分推理 | 无 `stream_id`、`generate`、`context_management` 字段；未知 input 类型警告后跳过；工具仅保留 function |
| [L5：provider stream.go](../../../provider/openai/responses/stream.go)，`handleStreamEvent` | 文本、函数参数、部分推理与终态映射 | 未识别事件返回 nil；没有 WS `error`、Steering、通道路由分支 |
| [L6：provider chat.go](../../../provider/openai/responses/chat.go)，`ChatWithSystem` | HTTP POST + SSEScanner | 没有 WS Dial 或帧处理 |
| [L7：provider.go](../../../provider/provider.go)，`Provider` | 单次 Chat/Complete 接口 | 无会话连接、Steer 或双向输入接口 |
| [L8：go.mod](../../../go.mod) | Go 1.25，当前依赖 bamboo-base-go 模块 | 当前项目并非先前知识库所述纯标准库；未声明 WS 库依赖 |

### 会影响 WS 兼容承诺的具体差异

1. L3 `handleMessageDelta` 在 max_tokens 情况下设置 `response.status="incomplete"`，但发出的事件名仍为 `response.completed`。官方事件模型区分 `response.incomplete`；WS 适配前需补齐契约验证。
2. L3 `ensureCreated` 合成 `response.created`，未合成官方分叉协调示例依赖的 `response.in_progress`。因此现成序列化器不能直接满足该流程。
3. L4 `inputItem` 没有 `phase`；消息级 commentary/final_answer 信息无法通过此结构完整往返。
4. L4 `textConfig` 仅保存 format.type；严格结构化输出的完整 schema 和具体工具选择信息不能据此宣称无损。
5. L5 switch 的 default 返回 nil，意味着原始事件流经过统一类型后会丢失未覆盖事件。原始同协议转发与通用类型重建需要分开考虑。

上述为静态核验发现，本次未修改 Go 实现。已有测试通过也仅证明其覆盖范围，不能覆盖尚无实现的 WS 功能。

## 6. 路径 C 与增量优化的安全条件

外部 Anthropic/Completions 请求经 codec 调用新的 OpenAI WS Provider，在共同能力上可行。`buildParams`、私有 DTO 和事件转换可作为复用基础，需要移除 HTTP `stream` 等传输字段、验证参数支持范围并增加路由。私有函数的复用还取决于实现放置位置。[L4–L7]

旧稿的 `messages[0:len-1]` 比较方案存在逻辑错误：下一轮历史通常包含“上轮输入 + 上轮 assistant 输出/工具调用 + 本轮工具结果/用户输入”；仅比较上轮请求无法识别这个边界，最后一个元素也可能只是多个新增结果之一。

增量化的考虑点：

- 显式会话身份绑定租户、认证信息、上游端点、连接代次与父响应；内容哈希仅用于等价校验。
- 保存上游真实输出及客户端可见历史的对应关系，验证新增输入前缀与父上下文一致；跨协议转换发生信息损失时可能无法证明等价。
- 比较全部有意义的历史条目，并保留全部新增项；模型、工具、指令等变化需要独立兼容规则。
- 引用父 ID 时只发送确认为新增的输入，避免将完整历史再叠加到父上下文造成重复。
- 找不到父缓存时，在确认尚未产生有效输出且工具执行可去重的条件下，才考虑全量恢复；部分输出和结果未知的断线不能无条件透明重试。

省略 `previous_response_id` 会启动新链；它仍可能享受其他类型的缓存或连接复用收益。旧稿将其概括为“所有缓存都无法命中”过度泛化。[O2]

## 7. 连接池、资源和性能约束

### 7.1 会话亲和性

官方当前每通道保留最新响应。把结束生成的通道立即借给其他会话，可能替换前一个会话的父缓存；`store=false` 的下一轮续接随后会失败。[O2]

因此活动请求容量和会话缓存保留是两种资源。固定通道池可以复用名称，但缓存归属、淘汰、分叉和恢复必须一起定义。跨连接轮换同样会丢失连接本地缓存；“Drain 后透明切换”需要历史恢复方案支撑。

### 7.2 隔离与背压

考虑按认证主体、上游凭据和端点隔离连接与历史。采用单读取循环、受控写入、每任务有界队列；慢消费者不能无限占用内存或阻塞全部通道。请求级错误可局部处理，连接断开影响全部关联任务。默认通道与缺少 `stream_id` 的参数错误需分别关联。[O2]

### 7.3 收益证据

官方原文为 20+ 次工具调用工作流 “up to roughly 40% faster end-to-end execution”。本次没有独立基准，保留为官方自述。SSE 桥接的排队、序列化、网络、工具时间和恢复都会改变收益，其他 SSE 上游更无法据此推导固定提升。[O2]

考虑点是以同模型、同任务、同工具、同并发和同网络条件对比 HTTP/SSE 与 WS，记录首事件/首文本延迟、端到端时长、P50/P95、错误与重试率、usage；区分热续接、冷连接和恢复路径。`previous_response_id` 也不意味着历史输入 token 免费。[O4]

## 8. 验证门槛与剩余工作

| 验证场景 | 通过条件 |
| --- | --- |
| 单轮文本与分片边界 | 完整 JSON 事件逐条输出，UTF-8和 SSE 多行/多事件解析正确 |
| 多个函数调用及结果续接 | call_id、参数、结果和全部新增输入完整且顺序正确 |
| max_tokens / failed / error / 提前 EOF | 终态匹配协议；无假成功、无等待不结束 |
| 双通道交织、同通道排队 | 无串流；同通道保序；每响应独立状态 |
| response_id 跨租户访问 | 拒绝引用其他主体历史 |
| 通道复用、父淘汰与断连恢复 | 明确未找到错误或正确全量恢复；无重复副作用 |
| Steering 及自动继任 | 明确支持范围；未支持时拒绝；支持时跟踪提交与pending |
| 原始输出重放 | 保留受支持 phase、推理、工具和多模态信息 |
| 慢客户端/取消 | 有界资源占用，其他任务继续运行 |
| 官方或实际目标客户端 | 解析、工具往返、错误与重连端到端通过 |

本次执行 `go test ./bamboo/codec/responses ./bamboo/relay ./provider/openai/responses`，三个包全部通过；结果仅用于现有实现核验。没有进行真实 WS 端到端连接、账户 ZDR 或性能实验。

## 9. 核验修订清单

| 旧稿说法 | 本版处置 |
| --- | --- |
| 首发即多路复用 | 撤回；首次能力日期未知 |
| WS 与 SSE 全部完全同构 | 收窄到基础事件模型；保留专用控制和路由差异 |
| 只换封装即可完整兼容 | 增加生命周期、历史、错误、ID、背压与能力校验 |
| 转换后固定获得约40%收益 | 撤回；官方特定工作流自述，项目待测 |
| 100%复用/90%复用及1–4周工期 | 删除缺少分解与测量的数字 |
| SSEScanner直接解析WS帧 | 明确两类输入分别处理；仅SSE字节可交SSE解析器 |
| 按最后一条消息做增量提取 | 撤回；改为完整父上下文等价校验 |
| 释放通道即可保留续接收益 | 补充最新父缓存被替换风险 |
| Go官方SDK没有实现且只能用指定库 | 撤回；精确版本覆盖待核验，库选择尚未决策 |
| 响应completed统一映射stop | 撤回；结合工具调用和停止原因 |

## 证据索引与局限

访问日期统一为 **2026-09-24**。O1 的发布日期取自具体日志条目；O2–O5 是动态页面，未给固定版本或发布日期。O2、O3 本轮抓取；O1、O4、O5采用同会话已获取正文核对。L1–L8 直接核查本地源码。

| 编号 | 来源 | 支撑范围 |
| --- | --- | --- |
| O1 | [OpenAI Changelog](https://developers.openai.com/api/docs/changelog) | 2026-02-23 WS 发布、2026-09-03 Steering 发布 |
| O2 | [WebSocket Mode](https://developers.openai.com/api/docs/guides/websocket-mode) | 当前传输、缓存、多路复用和恢复 |
| O3 | [WHATWG HTML §9.2 Server-sent events](https://html.spec.whatwg.org/multipage/server-sent-events.html) | UTF-8、事件边界、多行 data、EventSource 和 Last-Event-ID |
| O4 | [Conversation State](https://developers.openai.com/api/docs/guides/conversation-state) | 原始历史重放、phase及输入token计费 |
| O5 | [WebSocket Events](https://developers.openai.com/api/reference/resources/responses/websocket-events) | Responses和Steering事件结构 |

独立来源交叉范围：WHATWG验证SSE封装，项目源码验证现有实现，OpenAI文档验证上游契约。具体发布日期和性能数字仍只有OpenAI单一发布主体，不能宣称独立复现。此前搜索工具失败未形成证据；移除的工期、复用比例、内部存储路径及SDK缺失断言均无可靠支持。

最终收敛：**两种方向具备有限能力桥接的工程可行性；本项目提供转换基础，完整 Responses WebSocket 兼容服务仍需要新增有状态传输层并通过上述验证。**

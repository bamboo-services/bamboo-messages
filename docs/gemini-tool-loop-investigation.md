# Gemini 工具调用循环调查报告

> **调查日期**: 2026-09-13
> **涉及范围**: bamboo-messages 的 Gemini 原生协议链路（codec 请求解析、provider 出站、codec 出站序列化、门面转换）
> **结论性质**: 强假设，已获离线回归支持；未做线上部署验证

## 一、症状

在 Gemini 原生端点上，上层客户端（User-Agent 含 `opencode/`）出现同一工具被反复调用、历史结果无法关联的现象：模型每次发起新的调用 ID，而返回的结果无法与历史调用配对，观察到客户端反复重发同样的结构化工具请求。

远端日志样本（2026-09-13 18:11:52 至 18:14:30 CST，会话 `ses_f65bfa3c0ffeDtECQDHqM7Sclf`，UA `opencode/1.18.30`，原生路由 `/v1beta/models/gemini-3.8-flash:streamGenerateContent`，上游映射 `gemini-3.8-flash-high`）显示 12 次结构相同的工具调用，每次携带新的调用 ID。相关日志行：40145、40161、40266、40272、40335。UA 与原生路由信息对定位链路有帮助，但不单独构成因果证据。

需要明确：newapi 侧的 `bambooToolResponses` 日志只记录**最后一条 user 消息**里的工具结果，不是完整对话历史（见相邻 new-api 仓库 `service/log_record.go:216-297`、`relay/bamboo/bridge.go:627-659`，只读引用）。因此日志中"只看到一条结果"不能等同于"历史丢失"，必须回到请求边界比对。本地 new-api 源码与线上部署版本的一致性未做指纹核验，以上行号仅为只读参考。

## 二、脱敏合成示例

以下为完全虚构的良性示例，不含任何真实会话内容。签名串为占位假值。

第一轮模型输出（Gemini 原生格式）：

```json
{"candidates":[{"content":{"role":"model","parts":[
  {"functionCall":{"id":"call_A","name":"inspect_state","args":{"round":"A"}}}
]}}]}
```

客户端执行工具后回传结果（`functionResponse` 只有 `name` 和 `response`，没有 `id`）：

```json
{"role":"user","parts":[
  {"functionResponse":{"name":"inspect_state","response":{"output":"result_A"}}}
]}
```

第二轮模型再次输出 `{"functionCall":{"id":"call_B","name":"inspect_state","args":{"round":"B"}}}`，客户端再回 `result_B`。

修复前的缺陷：解析侧对缺失 `id` 的 `functionResponse` 用工具名当作关联键，与历史显式 ID `call_A` 永远对不上；共享清洗器随后丢弃这批结果，下一轮请求里工具交换整段消失，从而**可能**导致模型重新调用（离线红测试证明 request2 缺少历史，而非确定性的模型决策或已部署的因果链）。

## 三、修复后的配对契约

### 3.1 请求解析：轮次内关联（bamboo/codec/gemini）

`correlateToolHistory`（`tool_history.go`）在单次请求内完成关联，规则如下：

- **只在当前 model 轮次内配对**。遇到下一条 model content 或不含 `functionResponse` 的纯 user content，之前的活动调用组作废；更早轮次和未来声明的调用都不参与匹配。
- **完整结果段、显式 ID 优先**。先收集活动 model 组之后连续的一整段含结果的 user/function content，再统一分配：显式 ID 的结果先按 ID 匹配（要求名字一致，缺省名字从调用侧回填）；匹配不上的显式 ID 不做按名回退。
- **缺省 ID 按声明顺序按名配对**。没有 ID 的结果按原始 content/part 顺序，依次绑定到同名且未被消费的调用声明上。同名调用按位置区分，不按结果内容猜测。
- **显式调用 ID 原样保留**；缺省调用 ID 合成 `gemini_call_<名字>_<序号>` 形态，预扫描整份请求的显式**调用** ID 只为避免合成碰撞（结果 ID 不预占），不用于跨轮匹配。
- **非法输入产出空 ToolUseID**。未匹配、重复消费、名字不符的结果得到空 `ToolUseID` 的 `ToolResultBlock`，由门面层既有过滤丢弃（`bamboo/convert.go` 对空 `tool_use_id` 的过滤），不静默绑定到历史或未来。
- **唯一例外**：整份请求没有任何 model content 时（独立结果解析的既有兼容场景），显式孤儿结果的 ID 保留原样；它仍是孤儿，共享清洗器照样拒绝。缺省 ID 的孤儿始终为空，不按名回退。
- 关联状态是请求局部的，Codec 单例不保存跨请求状态，不修改输入 DTO。

### 3.2 出站调用 ID（provider/gemini）

`toolCallIDs`（`tool_call_id.go`）为**每次响应**分配独立的调用 ID：

- 每次 Chat/Complete 响应使用新的随机命名空间（`crypto/rand.Text()`）加单调序号，形态为 `gemini_call_<命名空间>_<序号>`，懒初始化。独立抽取的命名空间提供**抗碰撞性**，不是跨无关响应的数学唯一性保证；不引入全局注册表，未来上游显式 ID 无法跨流 chunk 预占。
- 输入历史里出现过的调用 ID（含 `ToolCallID`）进入排除集，合成 ID 不会与之重复。
- 上游显式给出的 ID 原样透传。Chat 与 Complete 使用同一套策略，各自的计数器归单次请求所有，Provider 上不存可变的跨请求状态。
- 同一响应里多个缺省 ID 的完整 `functionCall` Part 视为不同的调用，不按名合并。

### 3.3 终止与错误帧（provider/gemini）

- 空 `finishReason` 和 `FINISH_REASON_UNSPECIFIED` 不产生 Stop 事件；真正的 STOP 在出现过工具调用时映射为内部 `tool_calls`，纯文本 STOP 保持 `stop`。
- HTTP 200 的 SSE 帧若成功解码出错误信封且 `error` 为非空对象（含 `{}`，区别于 `error:null`），优先于同帧任何 `candidates`：发出一个携带上游 code/message 的错误事件后直接结束，不再排空响应体、不补成功 Stop/Done。code 缺失或非法时状态码保留 0，不伪装成 HTTP 200 成功。注意：不兼容的 `message` 类型（数字/布尔/对象/数组）会导致整个 `geminiErrorResponse` 解码失败，则不进入此显式错误分支；`message` 缺失或为 JSON null 会被接受为空消息，不妨碍错误分支。"不再发 Start"指错误优先帧自身不创建新的 Start；此前帧可能已发出 Start 和工具增量，本错误帧不产生后续成功 Stop/Done。
- 可选的 `status` 字段不参与错误对象的存在性判断，其 JSON 类型（数字、布尔、对象等）不能否决一个 code/message 合法的显式错误。

### 3.4 出站流式序列化（bamboo/codec/gemini）

`geminiStreamSerializer`（`stream.go`）按 Bamboo 事件的 `Index` 独立追踪每个调用：

- 参数增量只追加到同 Index 的调用；Stop 只关闭对应 Index；重复或不相关的 Stop 不产出任何内容。
- 完成的调用按首次出现的顺序输出，后完成的调用等前面的调用关闭后才发出，每个调用恰好发一次。
- 空参数输出 `{}`；非空参数必须是合法 JSON 对象才允许发出，否则走既有的 codec 错误路径，绝不编造可执行调用。未知 Index 的参数增量直接报错。
- **签名作用域**：紧邻调用之前、已完结且为空的 ThinkingBlock 所携带的 Gemini 签名是待挂元数据，挂到下一个调用（或没有调用介入时的下一个文本 Part）上，只挂一次。原生非空带签名 thinking 保留自己的签名，不会被挪用给调用。普通 thinking 文本已发出后迟到的签名，保留为末尾的空文本签名 Part，不指派给无关工具。消息终止或 Flush 时，未挂出的纯签名元数据以显式空文本 Part（带 `thoughtSignature`，不伪造 `thought=true`）发出一次。外来签名字段省略，不合并多个独立签名，签名不跨序列化器或请求携带。
- **粘性失败**：首个畸形、不完整或未知 Index 错误将序列化器标记为失败并清空待处理调用与签名，错误只向调用方返回一次；此后 Serialize/Flush 幂等抑制后续调用和成功终止帧。已发出的合法调用不可撤回。
- `EventError` 同样标记失败，丢弃所有未发调用，发出一个既有 Gemini 错误帧，忽略转换器随后合成的 stop/completion 序列。正常成功只发一个 finishReason/usage 帧，Flush 幂等。

### 3.5 错误标志透传（bamboo 门面）

`bamboo/convert.go` 在 `ToolResultBlock` 转换时把 `IsError` 原样复制到 `provider.Message.IsError`。Gemini 的 `functionResponse` 没有通用布尔错误字段，成功载荷保持原样，失败载荷走既有错误包装；不根据结果文本内容启发式重分类。

## 四、工具名与调用 ID 的区别

Gemini 历史配对以 `functionResponse.name` 对应 `functionCall.name`（函数名）为契约，`id` 是可选的调用标识。本库内部统一以调用 ID 关联调用与结果：解析时把 `functionCall.id`（显式或合成）记入 `ToolUseBlock.ID`，把配对结果记入 `ToolResultBlock.ToolUseID`，`ToolResultBlock.ToolName` 始终保存函数名。出站时 `functionResponse` 同时携带 `id`（= 调用 ID）和 `name`（= 函数名），两者不得混用。这一约定在**覆盖到的合法 Gemini 历史与受限转换范围**内保持调用与结果的 ID/名字关联；不做全协议无损保真承诺（见第七节残余限制）。

## 五、调用方责任边界

本库**不**新增工具执行循环、重试逻辑、新的 `continue` 注入或 Gemini `TOOL_CALLS` 线协议结束原因，也没有新增的公开配置项。调用方负责执行工具并发起下一次请求；库的修复只在**覆盖到的合法 Gemini 历史与受限转换范围**内保证请求与响应在各协议边界上的关联关系和保真度，签名与取消路径存在明确边界（见第七节）。

## 六、回归测试

行为修复均先写失败测试再修复。主要测试族（均可在离线环境运行）：

| 测试族 | 位置 | 覆盖点 |
|--------|------|--------|
| `TestGeminiToolHistory*` | `bamboo/codec/gemini/tool_history_test.go` | 显式/缺省/混合 ID、同名多轮与并行、拆分结果段、错误 ID/名字、重复与多余结果、孤儿、并发复用、输入不可变 |
| `TestGeminiToolLoopMissingResponseID` | `bamboo/relay/gemini_tool_loop_test.go` | 三轮真实协议往返：显式调用 + 缺省 ID 结果在第二、三个请求体中正确配对 |
| `TestGeminiToolCallIdentity*` | `provider/gemini/tool_call_identity_test.go`、`tool_call_id_test.go` | 连续响应 ID 不重复、并发 Provider、跨 chunk 多个 Part、显式 ID 保留、历史 ID 排除、确定性序号预留 |
| `TestGeminiUnspecifiedFinish*` | `provider/gemini/tool_call_identity_test.go` | UNSPECIFIED 不提前终止，后续工具与文本 STOP 正常 |
| `TestGeminiSSEErrorEnvelope*` | `provider/gemini/sse_error_envelope_test.go` | 错误优先于候选、同帧候选被否决、缺失/非法 code、held-open 响应体、可选 `status` 字段各种 JSON 类型 |
| `TestGeminiStreamIndexedCalls*` | `bamboo/codec/gemini/stream_indexed_test.go` | 交错 Index 参数、正逆关闭顺序、错位 Stop、空参数、畸形 JSON、未知 Index、粘性失败、幂等终止 |
| `TestGeminiStreamSignatureAttachment*` | `bamboo/codec/gemini/stream_signature_test.go` | 纯签名挂首个调用、原生带签名 thinking、迟到签名、外来签名、终末空文本、EventError 抑制合成成功 |
| `TestToolResultErrorPropagation*` | `bamboo/tool_result_error_test.go` | true/false 标志精确透传到真实 HTTP 请求体，孤儿错误结果仍被过滤 |
| `TestGeminiToolLoop*`（扩展族） | `bamboo/relay/gemini_tool_loop_{wire,rounds,parallel,failure,responses,wrapper}_test.go` | 多轮/并行/拆分/失败载荷/原生错误/畸形序列化/Responses 对照/包装器路由 |
| `TestChatCancelPending*` | `bamboo/chat_cancel_pending_test.go` | 取消时挂起调用不再产出可执行 Part（T7 取消修复） |

任务 5 的 relay 端到端测试曾暴露取消缺陷：早期 `-race -count=10` 运行中 `TestGeminiToolLoopCancelledStream` 在取消时挂起调用仍产出可执行 Part（pending_A），3/10 失败（保留于历史证据）。T7 针对性修复 `bamboo/bamboo.go` 的取消路径（挂起调用/已关闭 channel）并新增 `TestChatCancelPending*` 测试。该修复与任务 5 完整集成覆盖已经独立代码验证（见 `wave-2-code-verify.md`）：任务 5 `-race -count=10` 为 410/410 通过；两个取消测试各 100 次重复（共 200 次执行）通过。相关命令族（含验收用的 `-race -count=10`）：

```bash
env -u ANTHROPIC_API_KEY go test -count=1 ./provider/gemini/... ./bamboo/codec/gemini/... ./bamboo ./bamboo/relay/...
env -u ANTHROPIC_API_KEY go test -race -count=1 ./provider/gemini/... ./bamboo/codec/gemini/... ./bamboo ./bamboo/relay/...
env -u ANTHROPIC_API_KEY go test -count=1 -v ./bamboo/relay -run '^TestGeminiToolLoop'
env -u ANTHROPIC_API_KEY go test -race -count=10 -v ./bamboo/relay -run '^TestGeminiToolLoop'
```

上述任务 5 / T7 结果以 `wave-2-code-verify.md` 的独立验证为准；本地回归证据本身不构成最终验收或生产部署。

## 七、残余限制

- **签名保真作用域有限**。修复覆盖"紧邻调用之前的纯签名"这一可确定关联的场景；标量 IR 无法重建任意原始 Part 的血统和顺序，不承诺完整的原始 Part 保真。"携带非空原生签名的空文本 Part"不等于非法，官方允许空文本 Part 携带签名，本文不做"空文本签名 Part 一律无效"的断言。空签名**值**本身被序列化器忽略（`stream.go` 对 `Signature == ""` 直接返回），测试也不校验空凭据。非流式路径仅保留既有"调用前纯签名挂 functionCall"行为（`response.go` 标量 `pendingSig`），不是任意非流式终末签名保真；`bamboo/convert.go` 会拼接 thinking 文本且只保留最后一个签名，这些 IR 层损失不在本次修复范围。
- **事件原因只是强假设**。证据支持"缺省 ID 结果回退为名字导致与显式调用 ID 不匹配，共享清洗器丢弃结果，引发重复调用"这一机制；但完整的原始入站与上行请求体未取得，ID 丢失的确切源头与这些日志记录上清洗器的实际执行情况未被证明。不能据此断言 Gemini 服务端存在严格 ID 校验，也不能声称线上已修复。
- **日志字段语义**。`bambooToolResponses` 等字段在本 SDK 源码中没有定义，其完整性以 new-api 侧源码为准；本地源码与线上二进制未做指纹核验。
- **样本边界**。报告引用的 18:11:52-18:14:30 CST 会话是调查前流量；调查自身的流量（18:41 之后的样本）不计入事件证据。对照的 Responses 路由会话只是观察性比较，不证明客户端版本因果，也不保证所有 Responses 请求都成功。
- **无线上验证**。本次修复未在任何生产部署上验证，未使用真实模型端点；部署验证是独立事项。
- **取消路径边界**。任务 5 的 `-race -count=10` 曾暴露 `TestGeminiToolLoopCancelledStream` 取消时挂起调用产出可执行 Part（pending_A），3/10 失败（历史证据保留）。T7 修复经独立验证后，任务 5 race10 达 410/410 通过、两个取消测试各 100 次重复通过。残余限制：不保证全局原子取消或撤回，已入队的值仍可被读取，忽略取消的自定义 provider 不在重新设计范围；既有取消控制只检验有界关闭与无重复终止事件，注入的 EventError 测试不覆盖所有 context-cancel 路径。

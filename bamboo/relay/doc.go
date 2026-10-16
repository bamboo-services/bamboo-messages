// Package relay 提供跨 AI 对话协议的高性能函数式中继与转码网关能力。
//
// 本包是 [bamboo/codec] 与底层 [provider.Provider] 的组合枢纽。通过无缝互转管道，
// 允许下游客户端使用任意协议格式（如 OpenAI、Anthropic、Gemini、Responses、Bamboo）发起请求，
// 并透明路由至任意异构上游 Provider，输出目标协议所期望的响应或事件流。
//
// # 核心 API 与调用路径
//
// 本包提供三大核心互转入口：
//
//   - [Relay]：非流式请求互转。解析 inFormat 原始请求体，调用上游 Complete，并由 outFormat 序列化为目标协议响应 JSON；
//   - [RelayStream]：流式请求互转。实时消费上游事件并经 outFormat 序列化为 SSE 帧写入输出 channel，具备纯透传、零缓冲特性；
//   - [RelayResponsesWebSocket]：WebSocket 服务端网关。将 HTTP 请求升级为 OpenAI Responses WebSocket 会话，支持 stream_id 多路复用与跨协议多轮历史自动续接。
//
// # 跨协议多轮续接与状态恢复
//
// 当下游使用有状态的 OpenAI Responses WebSocket 协议发送增量请求（仅携带 previous_response_id 与新增输出），
// 而上游为无状态的 Anthropic、Gemini 或 OpenAI Chat Completions 时：
//
//   - 内置的 [ResponsesSessionStore] 自动记录每轮对话的完整上下文快照（包含文本、思维签名及工具调用）；
//   - 在后续轮次中，自动依据 previous_response_id 还原完整历史并拼接本轮工具结果发往无状态上游；
//   - 若找不到目标父响应，严格按规范向客户端回执 previous_response_not_found 错误事件。
//
// # 典型用法
//
// 以下展示如何将 OpenAI 格式的 HTTP POST 请求代理至 Anthropic Claude 上游并返回 Anthropic 格式：
//
//	func handleOpenAIToAnthropic(ctx context.Context, p provider.Provider, reqBody []byte) ([]byte, error) {
//		outJSON, err := relay.Relay(
//			ctx,
//			p,
//			reqBody,
//			codec.FormatOpenAI,
//			codec.FormatAnthropic,
//			relay.WithUsageCallback(func(u bamboo.Usage) {
//				log.Printf("Token consumed: in=%d out=%d", u.InputTokens, u.OutputTokens)
//			}),
//		)
//		return outJSON, err
//	}
//
// # 错误处理与协议规范化
//
// 当上游发生故障或请求解析失败时，[Relay] 会提取 [*pkgErrors.BambooError] 并自动调用 outCodec.SerializeError，
// 将错误信息规范化为目标协议客户端原生支持的 JSON 错误信封，确保下游客户端 SDK 可以无缝解析。
//
// # 并发与生命周期
//
// [Relay] 与 [RelayStream] 自身是无状态且并发安全的。[ResponsesSessionStore] 内部采用读写锁保护，
// 支持跨 WebSocket 连接与并发请求安全共享。流式输出 channel 会在处理完毕或上下文取消时由发送方安全关闭。
package relay

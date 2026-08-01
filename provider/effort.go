package provider

// NormalizeReasoningEffort 将第三方扩展的 effort 值归一化到 BambooMessages 标准值域
// (none/minimal/low/medium/high/xhigh)。
//
// "max" 是 Anthropic / GLM-5.2 等端点的扩展值，语义为最高推理强度；
// 但 qwen 等上游只接受标准值域，收到 "max" 会直接拒绝请求。
// 统一映射到标准值域的最大档 "xhigh"：语义完全等价，且对所有接受
// 扩展值域的上游（GLM 服务端会将 xhigh 兼容映射为 max）均安全。
//
// 其余值原样返回，不做白名单过滤，保持透传行为。
func NormalizeReasoningEffort(effort string) string {
	if effort == "max" {
		return "xhigh"
	}
	return effort
}

package bamboo

import "encoding/json"

// FinishReason 响应结束原因。
//
// 标识 AI 模型生成响应的停止原因，如正常结束、达到最大 token 数、工具调用等。
type FinishReason string

const (
	// FinishReasonEndTurn 正常结束，模型完成了完整的回复。
	FinishReasonEndTurn FinishReason = "end_turn"

	// FinishReasonMaxTokens 达到最大 token 数限制而停止。
	FinishReasonMaxTokens FinishReason = "max_tokens"

	// FinishReasonToolUse 模型请求调用工具而暂停。
	FinishReasonToolUse FinishReason = "tool_use"

	// FinishReasonStopSequence 遇到指定的停止序列而停止。
	FinishReasonStopSequence FinishReason = "stop_sequence"

	// FinishReasonPauseTurn 暂停等待用户输入。
	FinishReasonPauseTurn FinishReason = "pause_turn"

	// FinishReasonRefusal 模型拒绝生成内容。
	FinishReasonRefusal FinishReason = "refusal"

	// FinishReasonServerToolUse 服务器端工具调用。
	FinishReasonServerToolUse FinishReason = "server_tool_use"
)

// Response 非流式对话的完整结构化响应。
//
// # 字段体系
//
// 包含协议标准字段与 Bamboo 链路追踪扩展字段：
//   - ProviderType: 实际承载该请求的底层协议适配器标识（如 "anthropic"、"openai-completions"）；
//   - RequestID: 本次请求在网关或上游链路中的唯一追踪 ID；
//   - ResponseID: 对应 OpenAI Responses API 的 response_id，用于多轮对话链路追踪与 previous_response_id 续接；
//   - ReasoningID: 推理项独立 ID（对应 OpenAI Responses reasoning item，如 "rs_xxx"）；
//   - StopReason: 生成停止原因（[FinishReasonEndTurn]、[FinishReasonMaxTokens]、[FinishReasonToolUse] 等）。
type Response struct {
	ID           string         `json:"id"`                      // 消息唯一标识（由 AI 服务商生成）
	Type         string         `json:"type"`                    // 消息类型，固定为 "message"
	Role         MessageRole    `json:"role"`                    // 消息角色，固定为 "assistant"
	Content      []ContentBlock `json:"content"`                 // 响应内容块列表（文本、思考、工具调用等）
	Model        string         `json:"model"`                   // 实际响应的模型标识
	StopReason   FinishReason   `json:"stop_reason"`             // 停止原因
	StopSequence string         `json:"stop_sequence,omitempty"` // 触发停止的序列（未触发时为空）
	Usage        Usage          `json:"usage"`                   // Token 用量统计

	// ---- Bamboo 扩展追踪字段 ----

	ProviderType string `json:"provider_type"`          // 底层协议类型（如 "anthropic"、"openai-completions"）
	RequestID    string `json:"request_id,omitempty"`   // 请求追踪 ID
	ResponseID   string `json:"response_id,omitempty"`  // 响应 ID（用于会话状态续接）
	ReasoningID  string `json:"reasoning_id,omitempty"` // reasoning item ID（如 "rs_xxx"）
	CreatedAt    int64  `json:"created_at,omitempty"`   // 响应创建时间的 Unix 时间戳
}

// UnmarshalJSON 自定义 JSON 反序列化，使用 ContentBlocks 包装类型
// 根据 type 字段分派到具体的 ContentBlock 实现。
func (r *Response) UnmarshalJSON(data []byte) error {
	type alias Response
	tmp := struct {
		*alias
		Content ContentBlocks `json:"content"`
	}{
		alias: (*alias)(r),
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	r.Content = []ContentBlock(tmp.Content)
	return nil
}

// Usage Token 使用量统计。
//
// 记录本次 AI 对话的 Token 消耗情况，包括常规输入输出、思考推理以及 Prompt Caching 缓存统计。
//
// # 缓存与推理字段说明
//
//   - CacheCreationInputTokens: 写入/创建缓存所消耗的输入 Token（如 Anthropic 5m 缓存创建）；
//   - CacheReadInputTokens: 命中已有缓存读取的输入 Token（享受折扣计费）；
//   - ReasoningTokens: 模型用于显式或隐式思考推理消耗的 Token（属于 OutputTokens 的子集）。
type Usage struct {
	InputTokens              int64 `json:"input_tokens"`                          // 输入 token 数量
	OutputTokens             int64 `json:"output_tokens"`                         // 输出 token 数量
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"` // 缓存创建消耗的输入 token（可选）
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`     // 缓存命中读取的输入 token（可选）
	ReasoningTokens          int64 `json:"reasoning_tokens,omitempty"`            // 思考/推理 token 数量（可选）
}

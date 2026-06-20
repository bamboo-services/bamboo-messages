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
)

// Response 非流式对话的完整响应。
//
// 包含 Anthropic 原生字段和 Bamboo 扩展字段。
// ProviderType 和 RequestID 等 Bamboo 扩展字段用于追踪请求链路。
type Response struct {
	ID           string         `json:"id"`                      // 消息唯一标识（由 AI 服务商生成）
	Type         string         `json:"type"`                    // 消息类型，固定为 "message"
	Role         MessageRole    `json:"role"`                    // 消息角色，固定为 "assistant"
	Content      []ContentBlock `json:"content"`                 // 响应内容块列表
	Model        string         `json:"model"`                   // 使用的模型名称
	StopReason   FinishReason   `json:"stop_reason"`             // 停止原因
	StopSequence string         `json:"stop_sequence,omitempty"` // 触发停止的序列（可选）
	Usage        Usage          `json:"usage"`                   // Token 用量统计

	// ---- Bamboo 扩展字段 ----

	ProviderType string `json:"provider_type"`        // 底层协议类型（如 "anthropic"、"openai-completions"）
	RequestID    string `json:"request_id,omitempty"` // 请求追踪 ID
	CreatedAt    int64  `json:"created_at,omitempty"` // 响应创建时间的 Unix 时间戳
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
// 记录本次 AI 对话的 Token 消耗情况，包括输入、输出和缓存相关的 Token 使用量。
type Usage struct {
	InputTokens              int64 `json:"input_tokens"`                          // 输入 token 数量
	OutputTokens             int64 `json:"output_tokens"`                         // 输出 token 数量
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"` // 缓存创建消耗的输入 token 数量（可选）
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`     // 缓存命中读取的输入 token 数量（可选）
}

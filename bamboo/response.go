package bamboo

// FinishReason 响应结束原因。
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
	// ID 消息唯一标识（由 AI 服务商生成）
	ID string `json:"id"`

	// Type 消息类型，固定为 "message"
	Type string `json:"type"`

	// Role 消息角色，固定为 "assistant"
	Role MessageRole `json:"role"`

	// Content 响应内容块列表
	Content []ContentBlock `json:"content"`

	// Model 使用的模型名称
	Model string `json:"model"`

	// StopReason 停止原因
	StopReason FinishReason `json:"stop_reason"`

	// StopSequence 触发停止的序列（可选）
	StopSequence string `json:"stop_sequence,omitempty"`

	// Usage Token 用量统计
	Usage Usage `json:"usage"`

	// ---- Bamboo 扩展字段 ----

	// ProviderType 底层协议类型（如 "anthropic"、"openai-completions"）
	ProviderType string `json:"provider_type"`

	// RequestID 请求追踪 ID
	RequestID string `json:"request_id,omitempty"`

	// CreatedAt 响应创建时间的 Unix 时间戳
	CreatedAt int64 `json:"created_at,omitempty"`
}

// Usage Token 使用量统计。
type Usage struct {
	// InputTokens 输入 token 数量
	InputTokens int64 `json:"input_tokens"`

	// OutputTokens 输出 token 数量
	OutputTokens int64 `json:"output_tokens"`

	// CacheCreationInputTokens 缓存创建消耗的输入 token 数量（可选）
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`

	// CacheReadInputTokens 缓存命中读取的输入 token 数量（可选）
	CacheReadInputTokens int64 `json:"cache_read_input_tokens,omitempty"`
}

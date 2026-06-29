package bamboo

// StreamEventType 流事件类型标识。
//
// 用于标识流式传输过程中的不同事件类型，如消息开始、内容块增量、消息结束等。
type StreamEventType string

const (
	// EventMessageStart 消息开始事件，携带完整消息初始状态
	EventMessageStart StreamEventType = "message_start"

	// EventContentBlockStart 内容块开始事件，标记新内容块起始
	EventContentBlockStart StreamEventType = "content_block_start"

	// EventContentBlockDelta 内容块增量事件，携带文本/思考/工具调用的增量数据
	EventContentBlockDelta StreamEventType = "content_block_delta"

	// EventContentBlockStop 内容块结束事件，标记当前内容块传输完成
	EventContentBlockStop StreamEventType = "content_block_stop"

	// EventMessageDelta 消息增量事件，携带用量统计和停止原因
	EventMessageDelta StreamEventType = "message_delta"

	// EventMessageStop 消息结束事件，标记整条消息传输完成
	EventMessageStop StreamEventType = "message_stop"

	// EventPing 心跳事件，用于保持连接活跃
	EventPing StreamEventType = "ping"

	// EventError 错误事件，携带错误详情
	EventError StreamEventType = "error"
)

// StreamDeltaType 流增量数据类型标识。
//
// 用于标识不同类型的流增量数据，如文本增量、思考过程增量、工具调用参数增量等。
type StreamDeltaType string

const (
	// DeltaTextDelta 文本增量
	DeltaTextDelta StreamDeltaType = "text_delta"

	// DeltaThinkingDelta 思考过程增量
	DeltaThinkingDelta StreamDeltaType = "thinking_delta"

	// DeltaInputJSON 工具调用参数 JSON 增量
	DeltaInputJSON StreamDeltaType = "input_json_delta"

	// DeltaSignature 思考签名增量
	DeltaSignature StreamDeltaType = "signature_delta"
)

// StreamDelta 流增量数据，用于内容块增量事件中携带具体的增量内容。
//
// 不同增量类型对应不同字段：文本增量使用 Text 字段、思考过程增量使用 Thinking 字段、
// 工具调用参数增量使用 PartialJSON 字段、思考签名增量使用 Signature 字段。
type StreamDelta struct {
	Type        StreamDeltaType `json:"type"`                   // 增量类型
	Text        string          `json:"text,omitempty"`         // 文本增量内容（Type 为 DeltaTextDelta 时使用）
	Thinking    string          `json:"thinking,omitempty"`     // 思考过程增量内容（Type 为 DeltaThinkingDelta 时使用）
	Signature   string          `json:"signature,omitempty"`    // 思考签名增量（Type 为 DeltaSignature 时使用）
	PartialJSON string          `json:"partial_json,omitempty"` // 工具调用参数增量（Type 为 DeltaInputJSON 时使用）
}

// MessageDelta 消息增量，用于 message_delta 事件中携带停止原因和用量统计。
//
// 在消息传输结束时触发，提供完整的停止原因（如正常结束、达到最大 token 数、工具调用等）
// 和最终的 Token 用量统计。
//
// Metadata 字段（ResponseID/ReasoningID/EncryptedContent）由 provider 层的
// MetadataDelta 在流式过程中收集，最终在 message_delta 中统一输出，
// 供上层用于多轮对话的上下文关联（如 OpenAI Responses 的 response ID 链路追踪）。
type MessageDelta struct {
	StopReason   FinishReason `json:"stop_reason"`                       // 停止原因
	StopSequence string       `json:"stop_sequence,omitempty"`           // 触发停止的序列（可选）
	ResponseID   string       `json:"response_id,omitempty"`             // 响应 ID（OpenAI Responses response.id）
	ReasoningID  string       `json:"reasoning_id,omitempty"`            // reasoning item ID（如 "rs_xxx"）
	EncryptedContent string   `json:"encrypted_content,omitempty"`       // reasoning 加密内容（不透明 token）
}

// StreamEvent 流事件，由流式对话的 channel 逐步返回。
//
// 不同事件类型使用不同的字段组合：
//   - message_start: Message + Usage
//   - content_block_start: Index + ContentBlock
//   - content_block_delta: Index + Delta（StreamDelta 类型）
//   - content_block_stop: Index
//   - message_delta: Delta（MessageDelta 类型） + Usage
//   - message_stop: （无额外字段）
//   - ping: （无额外字段）
//   - error: Error
//
// Delta 字段使用 any 类型以兼容 StreamDelta 和 MessageDelta 两种类型，
// 调用方可通过类型断言获取具体类型。
type StreamEvent struct {
	Type         StreamEventType `json:"type"`                    // 事件类型
	Message      *BambooMessage  `json:"message,omitempty"`       // 完整消息（仅 message_start 事件使用）
	Index        int             `json:"index,omitempty"`         // 内容块索引（content_block_start/delta/stop 事件使用）
	ContentBlock ContentBlock    `json:"content_block,omitempty"` // 内容块（仅 content_block_start 事件使用）
	Delta        any             `json:"delta,omitempty"`         // 增量数据，可为 *StreamDelta 或 *MessageDelta（通过 any 兼容两种类型）
	Usage        *Usage          `json:"usage,omitempty"`         // Token 用量统计（message_start 和 message_delta 事件使用）
	Error        *BambooError    `json:"error,omitempty"`         // 错误详情（仅 error 事件使用）
}

package bamboo

// StreamEventType 流事件类型标识。
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
type StreamDelta struct {
	// Type 增量类型
	Type StreamDeltaType `json:"type"`

	// Text 文本增量内容（Type 为 DeltaTextDelta 时使用）
	Text string `json:"text,omitempty"`

	// Thinking 思考过程增量内容（Type 为 DeltaThinkingDelta 时使用）
	Thinking string `json:"thinking,omitempty"`

	// Signature 思考签名增量（Type 为 DeltaSignature 时使用）
	Signature string `json:"signature,omitempty"`

	// PartialJSON 工具调用参数增量（Type 为 DeltaInputJSON 时使用）
	PartialJSON string `json:"partial_json,omitempty"`
}

// MessageDelta 消息增量，用于 message_delta 事件中携带停止原因和用量统计。
type MessageDelta struct {
	// StopReason 停止原因
	StopReason FinishReason `json:"stop_reason"`

	// StopSequence 触发停止的序列（可选）
	StopSequence string `json:"stop_sequence,omitempty"`
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
	// Type 事件类型
	Type StreamEventType `json:"type"`

	// Message 完整消息（仅 message_start 事件使用）
	Message *BambooMessage `json:"message,omitempty"`

	// Index 内容块索引（content_block_start/delta/stop 事件使用）
	Index int `json:"index,omitempty"`

	// ContentBlock 内容块（仅 content_block_start 事件使用）
	ContentBlock *ContentBlock `json:"content_block,omitempty"`

	// Delta 增量数据，可为 *StreamDelta 或 *MessageDelta（通过 any 兼容两种类型）
	Delta any `json:"delta,omitempty"`

	// Usage Token 用量统计（message_start 和 message_delta 事件使用）
	Usage *Usage `json:"usage,omitempty"`

	// Error 错误详情（仅 error 事件使用）
	Error *BambooError `json:"error,omitempty"`
}

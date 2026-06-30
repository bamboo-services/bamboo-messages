package anthropic

import "encoding/json"

// messageCreateRequest 表示 Anthropic Messages API 的流式/非流式请求体。
//
// 所有字段严格对齐 Anthropic 官方 Messages 请求结构，支持文本、工具、缓存控制等特性。
type messageCreateRequest struct {
	Model         string           `json:"model"`                    // 模型 ID
	MaxTokens     int              `json:"max_tokens"`               // 最大输出 token 数
	Messages      []map[string]any `json:"messages"`                 // 对话消息列表
	System        any              `json:"system,omitempty"`         // 系统提示：字符串或带 cache_control 的块数组
	Stream        bool             `json:"stream,omitempty"`         // 是否启用流式响应
	Temperature   *float64         `json:"temperature,omitempty"`    // 采样温度（指针区分未设置）
	TopP          *float64         `json:"top_p,omitempty"`          // 核采样参数
	TopK          *int             `json:"top_k,omitempty"`          // Top-K 采样参数
	StopSequences []string         `json:"stop_sequences,omitempty"` // 停止序列列表
	Tools         []map[string]any `json:"tools,omitempty"`          // 工具定义列表
	ToolChoice    any              `json:"tool_choice,omitempty"`    // 工具选择策略
	Thinking      *thinkingConfig  `json:"thinking,omitempty"`       // 思考/推理配置
	Metadata      *metadata        `json:"metadata,omitempty"`       // 请求元数据
}

// thinkingConfig 表示 Anthropic 思考模式配置。
type thinkingConfig struct {
	Type         string `json:"type"`                    // 思考类型："adaptive" 或 "enabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // 思考 token 预算
}

// metadata 表示 Anthropic 请求级元数据。
type metadata struct {
	UserID string `json:"user_id,omitempty"` // 用户标识
}

// messageStreamEvent 表示 Anthropic 流式响应中的统一事件结构。
//
// 通过 Type 字段区分不同事件类型（message_start、content_block_start 等），
// 不同事件类型使用对应字段接收数据，Delta 字段保留原始 JSON 以实现灵活解析。
type messageStreamEvent struct {
	Type         string           `json:"type"`                    // 事件类型
	Delta        json.RawMessage  `json:"delta,omitempty"`         // 增量数据原始 JSON
	Message      *messageResponse `json:"message,omitempty"`       // message_start 事件的消息对象
	Index        *int             `json:"index,omitempty"`         // content_block_start/stop 的块索引
	ContentBlock *contentBlock    `json:"content_block,omitempty"` // content_block_start 的内容块
	Error        *anthropicError  `json:"error,omitempty"`         // error 事件的错误信息
}

// contentBlock 表示 Anthropic 响应中的内容块。
//
// 支持 text、thinking、tool_use 等类型，字段按需使用。
type contentBlock struct {
	Type      string          `json:"type"`                // 内容块类型：text、thinking、tool_use
	Text      string          `json:"text,omitempty"`      // 文本内容
	Thinking  string          `json:"thinking,omitempty"`  // 思考过程内容
	Signature string          `json:"signature,omitempty"` // 思考签名
	ID        string          `json:"id,omitempty"`        // 工具调用 ID
	Name      string          `json:"name,omitempty"`      // 工具名称
	Input     json.RawMessage `json:"input,omitempty"`     // 工具调用输入（JSON 原始数据）
}

// contentBlockDelta 表示 content_block_delta 事件中的增量数据。
//
// Type 字段区分文本、思考、签名、工具输入 JSON 等增量类型。
type contentBlockDelta struct {
	Type        string `json:"type"`                   // 增量类型：text_delta、thinking_delta、signature_delta、input_json_delta
	Text        string `json:"text,omitempty"`         // 文本增量
	Thinking    string `json:"thinking,omitempty"`     // 思考增量
	Signature   string `json:"signature,omitempty"`    // 签名增量
	PartialJSON string `json:"partial_json,omitempty"` // 工具输入 JSON 片段
}

// messageDeltaData 表示 message_delta 事件中 stop 相关的增量数据。
type messageDeltaData struct {
	StopReason   *string `json:"stop_reason,omitempty"`   // 停止原因
	StopSequence *string `json:"stop_sequence,omitempty"` // 触发停止的序列
}

// messageResponse 表示 Anthropic 非流式响应的完整消息结构。
type messageResponse struct {
	ID           string          `json:"id,omitempty"`            // 消息 ID
	Type         string          `json:"type,omitempty"`          // 消息类型："message"
	Role         string          `json:"role,omitempty"`          // 消息角色："assistant"
	Content      []contentBlock  `json:"content,omitempty"`       // 响应内容块列表
	Model        string          `json:"model,omitempty"`         // 使用的模型
	StopReason   *string         `json:"stop_reason,omitempty"`   // 停止原因
	StopSequence *string         `json:"stop_sequence,omitempty"` // 触发停止的序列
	Usage        *anthropicUsage `json:"usage,omitempty"`         // Token 用量统计
}

// anthropicUsage 表示 Anthropic 的 Token 用量统计。
//
// 保留 cache_creation_input_tokens 和 cache_read_input_tokens，用于 Prompt Caching 统计。
type anthropicUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`                // 输入 token 数
	OutputTokens             int `json:"output_tokens,omitempty"`               // 输出 token 数
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"` // 缓存创建输入 token 数
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`     // 缓存读取输入 token 数
}

// anthropicError 表示 Anthropic 返回的错误详情。
type anthropicError struct {
	Type    string `json:"type,omitempty"`    // 错误类型
	Message string `json:"message,omitempty"` // 错误信息
}

// anthropicErrorResponse 表示 Anthropic API 错误响应的外层包装。
type anthropicErrorResponse struct {
	Type  string          `json:"type,omitempty"`  // 响应类型："error"
	Error *anthropicError `json:"error,omitempty"` // 错误详情
}

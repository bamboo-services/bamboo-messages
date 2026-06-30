package responses

// responseCreateRequest 表示 OpenAI Responses API 的创建请求体。
type responseCreateRequest struct {
	// Model 指定要使用的模型 ID。
	Model string `json:"model"`
	// Input 为请求输入，可以是字符串或消息数组（[]map[string]any）。
	Input any `json:"input"`
	// Stream 是否使用流式响应。
	Stream bool `json:"stream,omitempty"`
	// Instructions 作为系统级提示词注入请求。
	Instructions string `json:"instructions,omitempty"`
	// MaxOutputTokens 最大输出 token 数。
	MaxOutputTokens *int `json:"max_output_tokens,omitempty"`
	// Temperature 采样温度。
	Temperature *float64 `json:"temperature,omitempty"`
	// TopP 核采样参数。
	TopP *float64 `json:"top_p,omitempty"`
	// Tools 工具定义列表。
	Tools []map[string]any `json:"tools,omitempty"`
	// ToolChoice 工具选择策略。
	ToolChoice any `json:"tool_choice,omitempty"`
	// Reasoning 推理/思考配置。
	Reasoning *reasoningConfig `json:"reasoning,omitempty"`
	// Metadata 附加元数据。
	Metadata map[string]any `json:"metadata,omitempty"`
	// User 终端用户标识。
	User string `json:"user,omitempty"`
	// PreviousResponseID 上一轮响应 ID，用于多轮对话。
	PreviousResponseID string `json:"previous_response_id,omitempty"`
	// Store 是否在服务端存储响应。
	Store *bool `json:"store,omitempty"`
	// Truncation 上下文截断策略。
	Truncation string `json:"truncation,omitempty"`
	// Modalities 输出模态。
	Modalities any `json:"modalities,omitempty"`
	// Include 指定响应中需要包含的附加数据。
	Include []string `json:"include,omitempty"`
}

// reasoningConfig 表示 OpenAI Responses API 的推理配置。
type reasoningConfig struct {
	// Effort 推理强度（none / low / medium / high）。
	Effort string `json:"effort,omitempty"`
	// Summary 推理摘要级别（concise / auto / detailed）。
	Summary string `json:"summary,omitempty"`
}

// responseStreamEvent 表示 OpenAI Responses API 的流式事件。
//
// 使用统一结构体承载所有事件类型，通过 Type 字段区分。
type responseStreamEvent struct {
	// Type 事件类型，如 response.created、response.output_text.delta 等。
	Type string `json:"type"`
	// SequenceNumber 事件序列号。
	SequenceNumber int `json:"sequence_number,omitempty"`
	// ResponseID 响应 ID。
	ResponseID string `json:"response_id,omitempty"`
	// Item 输出项对象。
	Item *responseObjectItem `json:"item,omitempty"`
	// OutputIndex 输出项索引。
	OutputIndex *int `json:"output_index,omitempty"`
	// ContentIndex 内容块索引。
	ContentIndex *int `json:"content_index,omitempty"`
	// Text 文本增量内容（output_text.delta 使用）。
	Text string `json:"text,omitempty"`
	// Arguments 函数调用参数增量（function_call_arguments.delta 使用）。
	Arguments string `json:"arguments,omitempty"`
	// Name 工具名称。
	Name string `json:"name,omitempty"`
	// CallID 工具调用 ID。
	CallID string `json:"call_id,omitempty"`
	// Response 完整响应对象（created/completed/incomplete 使用）。
	Response *responseObject `json:"response,omitempty"`
	// EncryptedContent 服务端加密的推理内容。
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// responseObject 表示 OpenAI Responses API 的非流式完整响应。
type responseObject struct {
	// ID 响应 ID。
	ID string `json:"id,omitempty"`
	// Object 对象类型。
	Object string `json:"object,omitempty"`
	// Status 响应状态。
	Status string `json:"status,omitempty"`
	// Model 使用的模型 ID。
	Model string `json:"model,omitempty"`
	// Output 输出项列表。
	Output []responseObjectItem `json:"output,omitempty"`
	// Usage Token 用量统计。
	Usage *responsesUsage `json:"usage,omitempty"`
	// CreatedAt 响应创建时间戳。
	CreatedAt float64 `json:"created_at,omitempty"`
}

// responseObjectItem 表示响应中的一个输出项。
type responseObjectItem struct {
	// Type 输出项类型：message / function_call / reasoning。
	Type string `json:"type,omitempty"`
	// ID 输出项 ID。
	ID string `json:"id,omitempty"`
	// Role 消息角色（message 类型使用）。
	Role string `json:"role,omitempty"`
	// Status 输出项状态。
	Status string `json:"status,omitempty"`
	// Content 消息内容块列表（message 类型使用）。
	Content []responseContentPart `json:"content,omitempty"`
	// Name 工具名称（function_call 类型使用）。
	Name string `json:"name,omitempty"`
	// CallID 工具调用 ID（function_call 类型使用）。
	CallID string `json:"call_id,omitempty"`
	// Arguments 工具调用参数（function_call 类型使用）。
	Arguments string `json:"arguments,omitempty"`
	// Summary 推理摘要文本（reasoning 类型使用）。
	Summary string `json:"summary,omitempty"`
	// EncryptedContent 服务端加密的推理内容（reasoning 类型使用）。
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// responseContentPart 表示消息输出项中的内容块。
type responseContentPart struct {
	// Type 内容块类型。
	Type string `json:"type,omitempty"`
	// Text 文本内容。
	Text string `json:"text,omitempty"`
}

// responsesUsage 表示 OpenAI Responses API 的 Token 用量统计。
type responsesUsage struct {
	// InputTokens 输入 token 数量。
	InputTokens int `json:"input_tokens,omitempty"`
	// OutputTokens 输出 token 数量。
	OutputTokens int `json:"output_tokens,omitempty"`
	// TotalTokens 总 token 数量。
	TotalTokens int `json:"total_tokens,omitempty"`
	// InputTokensDetails 输入 token 详细统计。
	InputTokensDetails *struct {
		// CachedTokens 缓存命中 token 数量。
		CachedTokens int `json:"cached_tokens,omitempty"`
	} `json:"input_tokens_details,omitempty"`
}

// openaiError 表示上游 OpenAI 兼容端点返回的错误结构。
type openaiError struct {
	Error struct {
		// Message 错误信息。
		Message string `json:"message,omitempty"`
		// Type 错误类型。
		Type string `json:"type,omitempty"`
		// Code 错误码。
		Code string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

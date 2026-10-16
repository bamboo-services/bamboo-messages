package responses

import "encoding/json"

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
	// Text 文本增量内容。官方 SSE 字段名为 delta，见 UnmarshalJSON。
	Text string `json:"text,omitempty"`
	// Arguments 函数调用参数增量。官方 SSE 字段名为 delta，见 UnmarshalJSON。
	Arguments string `json:"arguments,omitempty"`
	// Name 工具名称。
	Name string `json:"name,omitempty"`
	// CallID 工具调用 ID。
	CallID string `json:"call_id,omitempty"`
	// Response 完整响应对象（created/completed/incomplete 使用）。
	Response *responseObject `json:"response,omitempty"`
	// Error 流式错误事件负载（type="error" 时携带）。
	Error *responseErrorDetail `json:"error,omitempty"`
	// Message 顶层错误消息（部分兼容实现直接放在顶层）。
	Message string `json:"message,omitempty"`
	// Code 顶层错误码（部分兼容实现直接放在顶层）。
	Code string `json:"code,omitempty"`
	// EncryptedContent 服务端加密的推理内容。
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// UnmarshalJSON 兼容官方 Responses 流式帧：增量正文在 delta 字段，
// 而不是 text / arguments。Grok 与 OpenAI 原生 SSE 均使用 delta。
func (e *responseStreamEvent) UnmarshalJSON(data []byte) error {
	type alias responseStreamEvent
	aux := struct {
		Delta string `json:"delta"`
		*alias
	}{alias: (*alias)(e)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Delta == "" {
		return nil
	}
	if e.Text == "" {
		e.Text = aux.Delta
	}
	if e.Arguments == "" {
		e.Arguments = aux.Delta
	}
	return nil
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
	// Error 响应失败时的错误详情（response.failed 携带）。
	Error *responseErrorDetail `json:"error,omitempty"`
}

// responseErrorDetail 表示 Responses 响应或流事件中的错误详情。
type responseErrorDetail struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
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
	// Summary 推理摘要。官方为 [{type,text}] 数组，部分代理发 string。
	// 使用 RawMessage 避免数组解进 string 导致整帧 Unmarshal 失败、
	// 从而丢掉 encrypted_content / id。
	Summary json.RawMessage `json:"summary,omitempty"`
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
	// OutputTokensDetails 输出 token 细分（含 reasoning_tokens）。
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	} `json:"output_tokens_details,omitempty"`
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

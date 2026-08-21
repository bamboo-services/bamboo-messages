package completions

import "encoding/json"

// chatCompletionChunk 表示 OpenAI Chat Completions 流式响应中的单个 chunk。
type chatCompletionChunk struct {
	ID      string                      `json:"id,omitempty"`
	Object  string                      `json:"object,omitempty"`
	Created int64                       `json:"created,omitempty"`
	Model   string                      `json:"model,omitempty"`
	Choices []chatCompletionChunkChoice `json:"choices,omitempty"`
	Usage   *chunkUsage                 `json:"usage,omitempty"`
}

// chatCompletionChunkChoice 表示流式 chunk 中的单个 choice。
type chatCompletionChunkChoice struct {
	Index        int                 `json:"index,omitempty"`
	Delta        chatCompletionDelta `json:"delta,omitempty"`
	FinishReason *string             `json:"finish_reason,omitempty"` // 指针类型以区分 null 与空字符串
}

// chatCompletionDelta 表示流式响应中的增量消息数据。
type chatCompletionDelta struct {
	Role             string               `json:"role,omitempty"`
	Content          string               `json:"content,omitempty"`
	ReasoningContent json.RawMessage      `json:"reasoning_content,omitempty"` // 可能是字符串或对象
	Reasoning        json.RawMessage      `json:"reasoning,omitempty"`         // 部分服务商使用 reasoning 字段
	ToolCalls        []chunkDeltaToolCall `json:"tool_calls,omitempty"`
}

// chunkDeltaToolCall 表示流式增量中的工具调用片段。
type chunkDeltaToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// chunkUsage 表示流式 chunk 中的 Token 用量统计。
type chunkUsage struct {
	PromptTokens        int `json:"prompt_tokens,omitempty"`
	CompletionTokens    int `json:"completion_tokens,omitempty"`
	TotalTokens         int `json:"total_tokens,omitempty"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens,omitempty"`
	} `json:"prompt_tokens_details,omitempty"`
}

// chatCompletionResponse 表示非流式完整响应。
type chatCompletionResponse struct {
	ID      string                         `json:"id,omitempty"`
	Object  string                         `json:"object,omitempty"`
	Created int64                          `json:"created,omitempty"`
	Model   string                         `json:"model,omitempty"`
	Choices []chatCompletionResponseChoice `json:"choices,omitempty"`
	Usage   *chunkUsage                    `json:"usage,omitempty"`
}

// chatCompletionResponseChoice 表示非流式响应中的单个 choice。
type chatCompletionResponseChoice struct {
	Index        int                   `json:"index,omitempty"`
	Message      chatCompletionMessage `json:"message,omitempty"`
	FinishReason *string               `json:"finish_reason,omitempty"`
}

// chatCompletionMessage 表示非流式响应中的 assistant 消息。
type chatCompletionMessage struct {
	Role             string             `json:"role,omitempty"`
	Content          string             `json:"content,omitempty"`
	ReasoningContent json.RawMessage    `json:"reasoning_content,omitempty"`
	Reasoning        json.RawMessage    `json:"reasoning,omitempty"`
	ToolCalls        []responseToolCall `json:"tool_calls,omitempty"`
}

// responseToolCall 表示非流式响应中的工具调用结果。
type responseToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// openaiError 表示上游 OpenAI 兼容端点返回的错误结构。
type openaiError struct {
	Error struct {
		Message string `json:"message,omitempty"`
		Type    string `json:"type,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

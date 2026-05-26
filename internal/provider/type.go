package provider

// ============================================
// 常量类型相关的定义
// ============================================

// ProviderType AI 对话协议类型标识
type ProviderType string

const (
	ProviderAnthropic         ProviderType = "anthropic"          // Anthropic Messages 协议
	ProviderOpenAIResponses   ProviderType = "openai-responses"   // OpenAI Responses 协议
	ProviderOpenAICompletions ProviderType = "openai-completions" // OpenAI Chat Completions 协议
)

// ProviderExtra 键常量
const (
	ProviderExtraKeyTopK             = "top_k"
	ProviderExtraKeyFrequencyPenalty = "frequency_penalty"
	ProviderExtraKeyPresencePenalty  = "presence_penalty"
	ProviderExtraKeySeed             = "seed"
	ProviderExtraKeyToolChoice       = "tool_choice"
	ProviderExtraKeyResponseFormat   = "response_format"
)

// MessageRole 消息角色
type MessageRole string

const (
	RoleSystem    MessageRole = "system"    // 系统提示
	RoleUser      MessageRole = "user"      // 用户消息
	RoleAssistant MessageRole = "assistant" // 助手响应
	RoleTool      MessageRole = "tool"      // 工具响应
)

// FinishReason 完成原因
type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"       // 正常结束
	FinishReasonLength    FinishReason = "length"     // 达到最大长度
	FinishReasonToolCalls FinishReason = "tool_calls" // 工具调用
)

// CompletionResult 非流式调用的完整响应结果
type CompletionResult struct {
	Content      string        `json:"content"`                // 文本响应内容
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`   // 工具调用列表
	FinishReason FinishReason  `json:"finish_reason"`          // 结束原因
	Usage        UsageData     `json:"usage"`                  // Token 用量统计
}

// ============================================
// 消息相关结构体
// ============================================

// Message 对话消息
type Message struct {
	Role       MessageRole `json:"role"`                   // 消息角色
	Content    string      `json:"content,omitempty"`      // 消息内容
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // 助手发起的工具调用
	ToolCallID string      `json:"tool_call_id,omitempty"` // 工具响应的调用 ID
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string       `json:"id"`       // 调用 ID
	Type     string       `json:"type"`     // 类型，通常为 "function"
	Function FunctionCall `json:"function"` // 函数调用详情
}

// FunctionCall 函数调用详情
type FunctionCall struct {
	Name      string `json:"name"`      // 函数名
	Arguments string `json:"arguments"` // JSON 格式的参数
}

// ============================================
// 工具定义相关结构体
// ============================================

// Tool 工具定义
type Tool struct {
	Type     string      `json:"type"`     // 类型，通常为 "function"
	Function FunctionDef `json:"function"` // 函数定义
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string         `json:"name"`                  // 函数名
	Description string         `json:"description,omitempty"` // 函数描述
	Parameters  map[string]any `json:"parameters,omitempty"`  // JSON Schema 格式的参数定义
}

// ============================================
// 配置相关结构体
// ============================================

// ThinkingConfig 思考/推理配置，统一 Anthropic Thinking 和 OpenAI Reasoning 参数
type ThinkingConfig struct {
	Enabled         *bool  `json:"enabled,omitempty"`          // 是否启用思考/推理模式
	BudgetTokens    *int64 `json:"budget_tokens,omitempty"`    // Anthropic: 思考 token 预算
	ReasoningEffort string `json:"reasoning_effort,omitempty"` // OpenAI: none/low/medium/high
	Summary         string `json:"summary,omitempty"`          // OpenAI Responses: auto/concise/detailed
}

// ChatConfig 聊天请求配置
type ChatConfig struct {
	Model          string            `json:"model,omitempty"`          // 模型名称
	Temperature    *float64          `json:"temperature,omitempty"`    // 温度参数 (0-2)
	TopP           *float64          `json:"top_p,omitempty"`          // Top-p 采样
	MaxTokens      int64             `json:"max_tokens,omitempty"`     // 最大生成 token 数
	Stop           []string          `json:"stop,omitempty"`           // 停止词
	Tools          []Tool            `json:"tools,omitempty"`          // 可用工具列表
	Metadata       map[string]string `json:"metadata,omitempty"`       // 附加元数据
	ThinkingConfig *ThinkingConfig   `json:"thinking_config,omitempty"` // 思考/推理配置
	ProviderExtra  map[string]any    `json:"provider_extra,omitempty"`  // Provider 特有参数
}

// GetExtraFloat64 从 ProviderExtra 中安全获取 float64 值
func GetExtraFloat64(extra map[string]any, key string) (float64, bool) {
	if extra == nil {
		return 0, false
	}
	v, ok := extra[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// GetExtraInt64 从 ProviderExtra 中安全获取 int64 值
func GetExtraInt64(extra map[string]any, key string) (int64, bool) {
	if extra == nil {
		return 0, false
	}
	v, ok := extra[key]
	if !ok {
		return 0, false
	}
	i, ok := v.(int64)
	return i, ok
}

// GetExtraString 从 ProviderExtra 中安全获取 string 值
func GetExtraString(extra map[string]any, key string) (string, bool) {
	if extra == nil {
		return "", false
	}
	v, ok := extra[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetExtraAny 从 ProviderExtra 中获取任意类型值
func GetExtraAny(extra map[string]any, key string) (any, bool) {
	if extra == nil {
		return nil, false
	}
	v, ok := extra[key]
	return v, ok
}

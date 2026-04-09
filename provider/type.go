package provider

// ============================================
// 常量类型相关的定义
// ============================================

// ProviderType AI 服务提供商类型
type ProviderType string

const (
	ProviderAnthropic         ProviderType = "anthropic"          // Claude 系列
	ProviderOpenAIResponses   ProviderType = "openai-responses"   // GPT 系列 (Responses API)
	ProviderOpenAICompletions ProviderType = "openai-completions" // GPT 系列 (Chat Completions API)
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

// ChatConfig 聊天请求配置
type ChatConfig struct {
	Model       string            `json:"model,omitempty"`       // 模型名称
	Temperature *float64          `json:"temperature,omitempty"` // 温度参数 (0-2)
	TopP        *float64          `json:"top_p,omitempty"`       // Top-p 采样
	MaxTokens   int64             `json:"max_tokens,omitempty"`  // 最大生成 token 数
	Stop        []string          `json:"stop,omitempty"`        // 停止词
	Tools       []Tool            `json:"tools,omitempty"`       // 可用工具列表
	Metadata    map[string]string `json:"metadata,omitempty"`    // 附加元数据
}

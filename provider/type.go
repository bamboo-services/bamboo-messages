package provider

// ============================================
// 常量类型相关的定义
// ============================================

// ProviderType AI 对话协议的类型标识。
//
// 用于区分不同的 AI 对话协议实现，支持 Anthropic Messages、
// OpenAI Chat Completions 和 OpenAI Responses 等协议。
type ProviderType string

const (
	ProviderAnthropic         ProviderType = "anthropic"          // Anthropic Messages 协议
	ProviderOpenAIResponses   ProviderType = "openai-responses"   // OpenAI Responses 协议
	ProviderOpenAICompletions ProviderType = "openai-completions" // OpenAI Chat Completions 协议
)

// MessageRole 消息角色。
//
// 定义了对话中不同参与者的角色类型，包括系统提示、
// 用户消息、助手响应和工具响应。
type MessageRole string

const (
	RoleSystem    MessageRole = "system"    // 系统提示
	RoleUser      MessageRole = "user"      // 用户消息
	RoleAssistant MessageRole = "assistant" // 助手响应
	RoleTool      MessageRole = "tool"      // 工具响应
)

// FinishReason 完成原因。
//
// 表示 AI 对话结束的具体原因，如正常结束、达到最大长度
// 或触发工具调用等。
type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"       // 正常结束
	FinishReasonLength    FinishReason = "length"     // 达到最大长度
	FinishReasonToolCalls FinishReason = "tool_calls" // 工具调用
)

// CacheControlEphemeralTTL 缓存过期时间。
//
// 用于 Anthropic prompt caching 的 TTL 设置。
const (
	CacheTTL5m CacheControlEphemeralTTL = "5m" // 5 分钟（默认）
	CacheTTL1h CacheControlEphemeralTTL = "1h" // 1 小时
)

type CacheControlEphemeralTTL string

// CacheControl 缓存控制标记。
//
// 用于在请求中标记哪些内容块需要被缓存。
// Anthropic 使用显式断点标记（放在 system/messages/tools 上）；
// OpenAI 自动缓存，PromptCacheKey 仅作为路由粘性键；
// Gemini 通过外部 CachedContent 资源引用。
type CacheControl struct {
	Type string                   `json:"type"`          // 缓存类型，目前仅支持 "ephemeral"
	TTL  CacheControlEphemeralTTL `json:"ttl,omitempty"` // 缓存过期时间，仅 Anthropic 支持
}

// NewEphemeralCacheControl 创建一个 ephemeral 类型的缓存控制标记。
//
// ttl 默认为 5m，可传 CacheTTL1h 设置 1 小时。
func NewEphemeralCacheControl(ttl ...CacheControlEphemeralTTL) *CacheControl {
	t := CacheTTL5m
	if len(ttl) > 0 {
		t = ttl[0]
	}
	return &CacheControl{Type: "ephemeral", TTL: t}
}

// CompletionResult 非流式调用的完整响应结果。
//
// 包含 AI 模型返回的文本内容、工具调用列表、结束原因
// 和 Token 用量统计。适用于不需要流式输出的同步请求场景。
type CompletionResult struct {
	Content           string       `json:"content"`                      // 文本响应内容
	Thinking          string       `json:"thinking,omitempty"`           // 思考过程内容（如 Claude extended thinking）
	ThinkingSignature string       `json:"thinking_signature,omitempty"` // 推理签名/加密内容（OpenAI encrypted_content 透传）
	ToolCalls         []ToolCall   `json:"tool_calls,omitempty"`         // 工具调用列表
	FinishReason      FinishReason `json:"finish_reason"`                // 结束原因
	Usage             UsageData    `json:"usage"`                        // Token 用量统计
	ResponseID        string       `json:"response_id,omitempty"`        // 响应 ID（OpenAI Responses API 用于多轮对话循环追踪）
	ReasoningID       string       `json:"reasoning_id,omitempty"`       // 推理项 ID（OpenAI Responses API 的 reasoning item ID，如 "rs_xxx"，独立于 ThinkingSignature）
}

// ============================================
// 消息相关结构体
// ============================================

// Message 对话消息。
//
// 表示对话中的一条消息，包含角色、文本内容、多媒体内容块、
// 工具调用信息和工具响应的调用 ID。
// 当 ContentBlocks 不为空时，ContentBlocks 优先于 Content。
type Message struct {
	Role              MessageRole    `json:"role"`                         // 消息角色
	Content           string         `json:"content,omitempty"`            // 消息文本内容（向后兼容）
	ContentBlocks     []ContentBlock `json:"content_blocks,omitempty"`     // 多媒体内容块（优先于 Content）
	ThinkingContent   string         `json:"thinking_content,omitempty"`   // 思考过程内容（用于多轮对话中保留 thinking block）
	ThinkingSignature string         `json:"thinking_signature,omitempty"` // 思考过程签名（Anthropic extended thinking 验证签名）
	ReasoningID       string         `json:"reasoning_id,omitempty"`       // 推理项 ID（OpenAI Responses API 的 reasoning item ID，如 "rs_xxx"，独立于 ThinkingSignature）
	ToolCalls         []ToolCall     `json:"tool_calls,omitempty"`         // 助手发起的工具调用
	ToolCallID        string         `json:"tool_call_id,omitempty"`       // 工具响应的调用 ID
	ToolName          string         `json:"tool_name,omitempty"`          // 工具响应的函数名（Gemini FunctionResponse 需要）
	IsError           bool           `json:"is_error,omitempty"`           // 工具响应是否为错误
	CacheControl      *CacheControl  `json:"cache_control,omitempty"`      // 缓存控制标记（Anthropic prompt caching）
}

// ToolCall 工具调用。
//
// 表示 AI 模型发起的工具调用，包含调用 ID、类型
// 和函数调用详情。
type ToolCall struct {
	ID       string       `json:"id"`       // 调用 ID
	Type     string       `json:"type"`     // 类型，通常为 "function"
	Function FunctionCall `json:"function"` // 函数调用详情
}

// FunctionCall 函数调用详情。
//
// 包含函数名称和 JSON 格式的参数，用于描述工具调用的
// 具体内容。
type FunctionCall struct {
	Name      string `json:"name"`      // 函数名
	Arguments string `json:"arguments"` // JSON 格式的参数
}

// ============================================
// 工具定义相关结构体
// ============================================

// Tool 工具定义。
//
// 定义了可用工具的类型和函数规格，供 AI 模型在对话中调用。
type Tool struct {
	Type         string        `json:"type"`                    // 类型，通常为 "function"
	Function     FunctionDef   `json:"function"`                // 函数定义
	CacheControl *CacheControl `json:"cache_control,omitempty"` // 缓存控制标记（Anthropic prompt caching）
}

// FunctionDef 函数定义。
//
// 描述函数的名称、描述和参数结构，使用 JSON Schema 格式
// 定义参数类型和约束。
type FunctionDef struct {
	Name        string         `json:"name"`                  // 函数名
	Description string         `json:"description,omitempty"` // 函数描述
	Parameters  map[string]any `json:"parameters,omitempty"`  // JSON Schema 格式的参数定义
}

// ============================================
// 多媒体内容块相关类型
// ============================================

// ContentBlock 多媒体内容块接口。
//
// 定义内容块的统一访问方法，支持文本、图片、文档等多种类型。
type ContentBlock interface {
	BlockType() string // 返回内容块类型标识
}

// ImageContentBlock 图片内容块。
//
// 用于在对话中传递图片数据，支持 base64 编码和 URL 两种来源方式。
type ImageContentBlock struct {
	Source ImageSource `json:"source"` // 图片来源
}

// BlockType 实现 ContentBlock 接口，返回 "image"。
func (b ImageContentBlock) BlockType() string { return "image" }

// ImageSource 图片来源。
//
// 图片的来源信息，支持 base64 编码内联数据和远程 URL 两种方式。
// Type 为 "base64" 时使用 Data + MediaType；Type 为 "url" 时使用 URL。
type ImageSource struct {
	Type      string `json:"type"`                 // 来源类型："base64" | "url"
	MediaType string `json:"media_type,omitempty"` // MIME 类型，如 "image/png"
	Data      string `json:"data,omitempty"`       // base64 编码的图片数据
	URL       string `json:"url,omitempty"`        // 图片远程地址
}

// DocumentContentBlock 文档内容块。
//
// 用于在对话中传递文档数据，支持 base64 编码和 URL 两种来源方式。
type DocumentContentBlock struct {
	Source DocumentSource `json:"source"` // 文档来源
}

// BlockType 实现 ContentBlock 接口，返回 "document"。
func (b DocumentContentBlock) BlockType() string { return "document" }

// DocumentSource 文档来源。
//
// 文档的来源信息，支持 base64 编码内联数据和远程 URL 两种方式。
// Type 为 "base64" 时使用 Data + MediaType；Type 为 "url" 时使用 URL。
type DocumentSource struct {
	Type      string `json:"type"`                 // 来源类型："base64" | "url"
	MediaType string `json:"media_type,omitempty"` // MIME 类型，如 "application/pdf"
	Data      string `json:"data,omitempty"`       // base64 编码的文档数据
	URL       string `json:"url,omitempty"`        // 文档远程地址
}

// ============================================
// 配置相关结构体
// ============================================

// ThinkingConfig 思考/推理配置。
//
// Effort 统一控制所有 Provider 的思考/推理强度，支持 none/low/medium/high。
// 由各适配器根据 Effort 值映射到 Provider 特有参数：
//   - Anthropic: effort 值用于 adaptive thinking 模式
//   - OpenAI Completions: 映射为 ReasoningEffort
//   - OpenAI Responses: 映射为 ReasoningEffort，Summary 自动推导 (none→""、low→"concise"、medium→"auto"、high→"detailed")
type ThinkingConfig struct {
	Effort string `json:"effort,omitempty"` // 思考/推理强度: none/low/medium/high
}

// ChatConfig 聊天请求配置。
//
// 包含模型选择、温度参数、Token 限制、工具定义、
// 用户标识、工具选择策略、响应格式、并行工具调用、
// 思考配置和 Provider 特有参数等完整请求配置。
type ChatConfig struct {
	Model              string            `json:"model,omitempty"`                // 模型名称
	Temperature        *float64          `json:"temperature,omitempty"`          // 温度参数 (0-2)
	TopP               *float64          `json:"top_p,omitempty"`                // Top-p 采样
	MaxTokens          int64             `json:"max_tokens,omitempty"`           // 最大生成 token 数
	Stop               []string          `json:"stop,omitempty"`                 // 停止词
	Tools              []Tool            `json:"tools,omitempty"`                // 可用工具列表
	Metadata           map[string]string `json:"metadata,omitempty"`             // 附加元数据
	UserID             string            `json:"user_id,omitempty"`              // 用户标识
	ToolChoice         string            `json:"tool_choice,omitempty"`          // 工具选择策略
	ResponseFormat     string            `json:"response_format,omitempty"`      // 响应格式
	ParallelToolCalls  bool              `json:"parallel_tool_calls,omitempty"`  // 并行工具调用
	ThinkingConfig     *ThinkingConfig   `json:"thinking_config,omitempty"`      // 思考/推理配置
	SystemCacheControl *CacheControl     `json:"system_cache_control,omitempty"` // system prompt 的缓存标记
	PromptCacheKey     string            `json:"prompt_cache_key,omitempty"`     // OpenAI prompt cache 路由键
	ProviderExtra      map[string]any    `json:"provider_extra,omitempty"`       // Provider 特有参数
}

// GetExtraFloat64 从 ProviderExtra 中安全获取 float64 值。
//
// 参数:
//   - extra - ProviderExtra 映射表，为 nil 时返回 (0, false)
//   - key - 要查找的键名
//
// 返回值中的 bool 表示是否成功找到并完成类型断言。
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

// GetExtraBool 从 ProviderExtra 中安全获取 bool 值。
//
// 参数:
//   - extra - ProviderExtra 映射表，为 nil 时返回 (false, false)
//   - key - 要查找的键名
//
// 返回值中的 bool 表示是否成功找到并完成类型断言。
func GetExtraBool(extra map[string]any, key string) (bool, bool) {
	if extra == nil {
		return false, false
	}
	v, ok := extra[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

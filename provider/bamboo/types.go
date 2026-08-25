// Package bamboo 提供 bamboo 原生协议的 wire DTO 与转换函数。
//
// 本包定义与 bamboo facade 包（github.com/bamboo-services/bamboo-messages/bamboo）
// JSON 格式 1:1 对应的独立 wire 类型，避免 provider/bamboo 导入上层 facade 包
// （防止循环依赖）。所有 wire 类型字段名、JSON tag、omitempty 策略均严格镜像
// facade 包的对应类型。
package bamboo

import "encoding/json"

// wireRequest bamboo 原生协议请求信封。
//
// 镜像 bamboo facade 的请求 JSON 顶层结构：messages 数组 + system 字符串 +
// config 请求配置 + stream 流式标记。
type wireRequest struct {
	Messages []wireMessage      `json:"messages"`         // 消息列表
	System   string             `json:"system,omitempty"` // 系统提示（可选）
	Config   *wireRequestConfig `json:"config,omitempty"` // 请求配置（可选）
	Stream   bool               `json:"stream,omitempty"` // 流式标记（由 chat.go/complete.go 设置）
}

// wireMessage bamboo 原生协议消息。
//
// 镜像 bamboo.BambooMessage 的 JSON 结构：role + content 内容块数组 + reasoning_id。
type wireMessage struct {
	Role        string             `json:"role"`                   // 消息角色：user / assistant
	Content     []wireContentBlock `json:"content"`                // 内容块列表
	ReasoningID string             `json:"reasoning_id,omitempty"` // 推理项 ID（OpenAI Responses reasoning item ID）
}

// wireContentBlock bamboo 原生协议内容块扁平联合类型。
//
// 通过 Type 字段区分 7 种内容块类型（text/thinking/tool_use/tool_result/
// image/document/redacted_thinking），所有可选字段均带 omitempty 以确保
// JSON 序列化时仅输出当前类型相关字段。镜像 bamboo/content.go 中 7 种
// ContentBlock 实现的全部字段并集。
type wireContentBlock struct {
	Type         string          `json:"type"`                    // 内容块类型标识（必填）
	Text         string          `json:"text,omitempty"`          // text: 文本内容
	Thinking          string          `json:"thinking,omitempty"`           // thinking: 思考过程内容
	Signature         string          `json:"signature,omitempty"`          // thinking: 验证签名
	SignatureProvider string          `json:"signature_provider,omitempty"` // thinking: 签名血统
	ID           string          `json:"id,omitempty"`            // tool_use: 调用 ID
	Name         string          `json:"name,omitempty"`          // tool_use / tool_result: 函数名
	Input        json.RawMessage `json:"input,omitempty"`         // tool_use: 参数 JSON
	ToolUseID    string          `json:"tool_use_id,omitempty"`   // tool_result: 对应的 tool_use ID
	ToolName     string          `json:"tool_name,omitempty"`     // tool_result: 函数名
	Content      string          `json:"content,omitempty"`       // tool_result: 结果内容
	IsError      bool            `json:"is_error,omitempty"`      // tool_result: 是否为错误
	Source       *wireSource     `json:"source,omitempty"`        // image / document: 来源
	CacheControl json.RawMessage `json:"cache_control,omitempty"` // 缓存控制标记（所有类型通用）
	Data         string          `json:"data,omitempty"`          // redacted_thinking: 加密数据
}

// wireSource bamboo 原生协议内容来源。
//
// 镜像 bamboo.ContentSource：统一来源类型，通过 Type 字段区分 base64/url/text。
type wireSource struct {
	Type      string `json:"type"`                 // 来源类型：base64 / url / text
	MediaType string `json:"media_type,omitempty"` // MIME 类型
	Data      string `json:"data,omitempty"`       // base64 编码数据
	URL       string `json:"url,omitempty"`        // 远程地址
	Content   string `json:"content,omitempty"`    // 纯文本内容（document text 类型）
}

// wireRequestConfig bamboo 原生协议请求配置。
//
// 镜像 bamboo.RequestConfig 的全部字段，JSON tag 和 omitempty 策略完全一致。
type wireRequestConfig struct {
	Model              string              `json:"model"`                          // 模型名称
	MaxTokens          int64               `json:"max_tokens"`                     // 最大生成 token 数
	Temperature        *float64            `json:"temperature,omitempty"`          // 温度参数
	TopP               *float64            `json:"top_p,omitempty"`                // Top-p 采样
	StopSequences      []string            `json:"stop_sequences,omitempty"`       // 停止序列
	Tools              []wireTool          `json:"tools,omitempty"`                // 工具列表
	Metadata           map[string]string   `json:"metadata,omitempty"`             // 元数据
	UserID             string              `json:"user_id,omitempty"`              // 用户标识
	ToolChoice         string              `json:"tool_choice,omitempty"`          // 工具选择策略
	ResponseFormat     string              `json:"response_format,omitempty"`      // 响应格式
	ParallelToolCalls  bool                `json:"parallel_tool_calls,omitempty"`  // 并行工具调用
	ThinkingConfig     *wireThinkingConfig `json:"thinking_config,omitempty"`      // 思考/推理配置
	SystemCacheControl json.RawMessage     `json:"system_cache_control,omitempty"` // system 缓存标记
	PromptCacheKey     string              `json:"prompt_cache_key,omitempty"`     // prompt cache 粘性键
	ProviderExtra      map[string]any      `json:"provider_extra,omitempty"`       // Provider 特有参数
}

// wireTool bamboo 原生协议工具定义。
//
// 与 OpenAI 风格的 {type:"function", function:{...}} 不同，
// bamboo 工具定义为扁平结构 {name, description, input_schema, cache_control}。
// InputSchema 为 json.RawMessage 原样透传 JSON Schema，无 omitempty
// （镜像 bamboo.Tool 的 tag `json:"input_schema"`）。
type wireTool struct {
	Name         string          `json:"name"`                    // 工具名称
	Description  string          `json:"description,omitempty"`   // 工具描述
	InputSchema  json.RawMessage `json:"input_schema"`            // 参数 JSON Schema（必填）
	CacheControl json.RawMessage `json:"cache_control,omitempty"` // 缓存控制标记
}

// wireResponse bamboo 原生协议非流式响应。
//
// 镜像 bamboo.Response 的全部字段（含 Bamboo 扩展字段）。
type wireResponse struct {
	ID           string             `json:"id"`                      // 消息 ID
	Type         string             `json:"type"`                    // 消息类型（固定 "message"）
	Role         string             `json:"role"`                    // 消息角色（固定 "assistant"）
	Content      []wireContentBlock `json:"content"`                 // 内容块列表
	Model        string             `json:"model"`                   // 模型名称
	StopReason   string             `json:"stop_reason"`             // 停止原因
	StopSequence string             `json:"stop_sequence,omitempty"` // 触发停止的序列
	Usage        wireUsage          `json:"usage"`                   // Token 用量
	ProviderType string             `json:"provider_type"`           // Provider 类型
	RequestID    string             `json:"request_id,omitempty"`    // 请求追踪 ID
	ResponseID   string             `json:"response_id,omitempty"`   // 响应 ID（多轮链路）
	CreatedAt    int64              `json:"created_at,omitempty"`    // 创建时间戳
}

// wireUsage bamboo 原生协议 Token 用量统计。
//
// 镜像 bamboo.Usage 的全部字段（含缓存统计）。
type wireUsage struct {
	InputTokens              int64 `json:"input_tokens"`                          // 输入 token 数
	OutputTokens             int64 `json:"output_tokens"`                         // 输出 token 数
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"` // 缓存创建 token 数
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`     // 缓存读取 token 数
}

// wireThinkingConfig bamboo 原生协议思考/推理配置。
//
// 镜像 provider.ThinkingConfig（bamboo.ThinkingConfig 为其类型别名）。
type wireThinkingConfig struct {
	Effort  string `json:"effort,omitempty"`  // 思考强度：none/low/medium/high
	Display string `json:"display,omitempty"` // 显示模式：summarized/omitted
}

// wireErrorPayload bamboo 原生协议错误事件载荷。
//
// 用于流式 error 事件的 JSON 结构。
type wireErrorPayload struct {
	Type  string        `json:"type"`  // 事件类型（固定 "error"）
	Error wireErrorBody `json:"error"` // 错误体
}

// wireErrorBody bamboo 原生协议错误体。
//
// 镜像 pkg/errors.BambooError 的 JSON 结构。
type wireErrorBody struct {
	Category   string `json:"category"`              // 错误分类
	Message    string `json:"message"`               // 错误描述
	StatusCode int    `json:"status_code,omitempty"` // HTTP 状态码
}

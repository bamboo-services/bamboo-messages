package gemini

import "encoding/json"

// generateContentRequest 是 Gemini 流式 / 非流式生成内容请求的 DTO。
//
// 包含对话历史、系统提示、生成配置、工具声明与工具调用策略，
// 直接对应 Gemini generateContent / streamGenerateContent 端点的 JSON 结构。
type generateContentRequest struct {
	Contents          []geminiContent   `json:"contents"`
	SystemInstruction *geminiContent    `json:"systemInstruction,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
	Tools             []geminiTool      `json:"tools,omitempty"`
	ToolConfig        *toolConfig       `json:"toolConfig,omitempty"`
}

// generationConfig 是 Gemini 的生成配置，控制采样、输出长度、思考模式与安全策略等。
type generationConfig struct {
	Temperature      *float64          `json:"temperature,omitempty"`
	TopP             *float64          `json:"topP,omitempty"`
	TopK             *float64          `json:"topK,omitempty"`
	MaxOutputTokens  *int              `json:"maxOutputTokens,omitempty"`
	StopSequences    []string          `json:"stopSequences,omitempty"`
	ThinkingConfig   *thinkingConfig   `json:"thinkingConfig,omitempty"`
	ResponseMimeType string            `json:"responseMimeType,omitempty"`
	SafetySettings   []safetySetting   `json:"safetySettings,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	CachedContent    string            `json:"cachedContent,omitempty"`
}

// thinkingConfig 是 Gemini 思考 / 推理配置。
//
// ThinkingLevel 取值通常为 "low" / "medium" / "high"，
// ThinkingBudget 用于控制思考阶段的最大 token 消耗。
type thinkingConfig struct {
	IncludeThoughts bool   `json:"includeThoughts,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
	ThinkingBudget  *int   `json:"thinkingBudget,omitempty"`
}

// safetySetting 是 Gemini 的安全策略设置，按危害类别设定屏蔽阈值。
type safetySetting struct {
	Category  string `json:"category,omitempty"`
	Threshold string `json:"threshold,omitempty"`
}

// toolConfig 用于配置工具调用行为，例如是否强制 / 禁用函数调用。
type toolConfig struct {
	FunctionCallingConfig *functionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// functionCallingConfig 定义工具调用模式：AUTO / NONE / ANY。
type functionCallingConfig struct {
	Mode string `json:"mode,omitempty"`
}

// geminiContent 表示 Gemini 的一条消息内容，包含角色与 Part 数组。
//
// Role 取值通常为 "user"（用户）或 "model"（模型），
// Parts 是内容的具体片段（文本、思考、工具调用、工具响应等）。
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart 是 Gemini 内容的最小单元，同一字段只能表达一种类型。
//
// 普通文本 / 推理文本使用 Text 字段；Thought 为 true 表示这是推理内容。
// FunctionCall 与 FunctionResponse 分别表示工具调用与工具响应。
// InlineData 用于内联图片、文档等多媒体数据。
type geminiPart struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
}

// functionCall 表示模型发起的一次工具调用，包含工具名与参数。
//
// ID 用于关联后续的工具响应，Args 使用 json.RawMessage 保留参数原始 JSON，
// 避免在 DTO 层做具体类型假设。
type functionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// functionResponse 表示一次工具调用的返回结果。
//
// ID 与 Name 均用于匹配对应的 function_call；Response 存放原始 JSON 返回值。
type functionResponse struct {
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

// inlineData 表示内联二进制数据（图片、文档等），Data 为 base64 编码字符串。
type inlineData struct {
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
}

// geminiTool 声明一个工具，包含若干函数声明。
type geminiTool struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations,omitempty"`
}

// functionDeclaration 声明一个可调用的函数，包括名称、描述与参数 Schema。
//
// Parameters 使用 json.RawMessage，允许直接透传 JSON Schema 而不引入外部类型。
type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// generateContentResponse 是 Gemini 生成内容响应的 DTO，流式与非流式共享同一结构。
//
// Candidates 包含模型生成的候选内容，UsageMetadata 记录 Token 用量，
// ModelVersion 标识实际服务模型版本。
type generateContentResponse struct {
	Candidates    []geminiCandidate `json:"candidates,omitempty"`
	UsageMetadata *usageMetadata    `json:"usageMetadata,omitempty"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
}

// geminiCandidate 是 generateContentResponse 中的单个候选结果。
//
// Content 包含模型返回的消息内容，FinishReason 表示生成结束原因。
type geminiCandidate struct {
	Content       *geminiContent `json:"content,omitempty"`
	FinishReason  string         `json:"finishReason,omitempty"`
	Index         *int           `json:"index,omitempty"`
	SafetyRatings []safetyRating `json:"safetyRatings,omitempty"`
}

// safetyRating 是 Gemini 安全评估结果，说明内容在某一类别上的风险概率。
type safetyRating struct {
	Category    string `json:"category,omitempty"`
	Probability string `json:"probability,omitempty"`
}

// usageMetadata 记录 Gemini 请求的 Token 使用统计，包括缓存命中统计。
type usageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount    int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount         int `json:"totalTokenCount,omitempty"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
}

// geminiErrorResponse 是 Gemini API 返回的错误响应外层结构。
type geminiErrorResponse struct {
	Error *geminiErrorDetail `json:"error,omitempty"`
}

// geminiErrorDetail 是 Gemini 错误响应的具体错误信息。
type geminiErrorDetail struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Status  string `json:"status,omitempty"`
}

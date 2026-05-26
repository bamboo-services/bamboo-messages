package bamboo

import "encoding/json"

// ContentBlockType 内容块类型标识。
//
// 标识内容块的具体类型，用于区分文本、思考过程、工具调用、图片和文档等不同形式。
type ContentBlockType string

const (
	// ContentBlockText 纯文本内容块
	ContentBlockText ContentBlockType = "text"

	// ContentBlockThinking 思考过程内容块（如 Claude Extended Thinking）
	ContentBlockThinking ContentBlockType = "thinking"

	// ContentBlockToolUse 工具调用内容块
	ContentBlockToolUse ContentBlockType = "tool_use"

	// ContentBlockToolResult 工具调用结果内容块
	ContentBlockToolResult ContentBlockType = "tool_result"

	// ContentBlockImage 图片内容块
	ContentBlockImage ContentBlockType = "image"

	// ContentBlockDocument 文档内容块
	ContentBlockDocument ContentBlockType = "document"
)

// ContentSource 统一来源类型，合并图片和文档的来源字段。
//
// 不同类型的来源通过 Type 字段区分：
//   - "base64": base64 编码的内联数据（Data + MediaType）
//   - "url": 远程 URL 地址
//   - "text": 纯文本内容（仅文档类型使用 Content 字段）
type ContentSource struct {
	Type      string `json:"type"`                // 来源类型："base64" | "url" | "text"
	MediaType string `json:"media_type,omitempty"` // MIME 类型，如 "image/png"、"application/pdf"
	Data      string `json:"data,omitempty"`      // base64 编码的数据（Type 为 "base64" 时使用）
	URL       string `json:"url,omitempty"`       // 远程资源地址（Type 为 "url" 时使用）
	Content   string `json:"content,omitempty"`   // 纯文本内容（仅 document 类型且 Type 为 "text" 时使用）
}

// ContentBlock 消息内容块，支持文本、思考过程、工具调用、图片和文档等多种类型。
//
// 通过 Type 字段区分内容块类型，不同类型使用不同的字段组合。
// 未使用的字段应保持零值，JSON 序列化时会被 omitempty 忽略。
type ContentBlock struct {
	Type ContentBlockType `json:"type"` // 内容块类型

	// ---- text 类型字段 ----

	Text string `json:"text,omitempty"` // 文本内容（Type 为 "text" 时使用）

	// ---- thinking 类型字段 ----

	Thinking  string `json:"thinking,omitempty"`  // 思考过程文本（Type 为 "thinking" 时使用）
	Signature string `json:"signature,omitempty"` // 思考签名（Type 为 "thinking" 时使用，用于验证）

	// ---- tool_use 类型字段 ----

	ID    string          `json:"id,omitempty"`    // 工具调用唯一标识（Type 为 "tool_use" 时使用）
	Name  string          `json:"name,omitempty"`  // 工具名称（Type 为 "tool_use" 时使用）
	Input json.RawMessage `json:"input,omitempty"` // 工具调用参数，JSON 格式（Type 为 "tool_use" 时使用）

	// ---- tool_result 类型字段 ----

	ToolUseID     string `json:"tool_use_id,omitempty"` // 关联的工具调用 ID（Type 为 "tool_result" 时使用）
	IsError       bool   `json:"is_error,omitempty"`    // 标记工具结果是否为错误（Type 为 "tool_result" 时使用）
	ResultContent string `json:"content,omitempty"`     // 工具结果的文本内容（Type 为 "tool_result" 时使用）

	// ---- image / document 类型字段 ----

	Source *ContentSource `json:"source,omitempty"` // 统一来源描述（Type 为 "image" 或 "document" 时使用）
}

// NewTextBlock 创建文本内容块。
//
// 适用于纯文本对话场景。
func NewTextBlock(text string) ContentBlock {
	return ContentBlock{
		Type: ContentBlockText,
		Text: text,
	}
}

// NewThinkingBlock 创建思考过程内容块。
//
// thinking 为思考过程文本，signature 为用于验证的签名。
func NewThinkingBlock(thinking, signature string) ContentBlock {
	return ContentBlock{
		Type:      ContentBlockThinking,
		Thinking:  thinking,
		Signature: signature,
	}
}

// NewToolUseBlock 创建工具调用内容块。
//
// input 参数会被序列化为 JSON RawMessage。若序列化失败，Input 会被设为空的 JSON 对象。
func NewToolUseBlock(id, name string, input any) ContentBlock {
	var raw json.RawMessage
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			raw = json.RawMessage(`{}`)
		} else {
			raw = json.RawMessage(data)
		}
	} else {
		raw = json.RawMessage(`{}`)
	}

	return ContentBlock{
		Type:  ContentBlockToolUse,
		ID:    id,
		Name:  name,
		Input: raw,
	}
}

// NewToolResultBlock 创建工具调用结果内容块。
//
// 用于将工具调用的执行结果返回给模型。
func NewToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return ContentBlock{
		Type:          ContentBlockToolResult,
		ToolUseID:     toolUseID,
		ResultContent: content,
		IsError:       isError,
	}
}

// NewImageBlock 创建图片内容块。
//
// 支持通过 base64 编码或远程 URL 传递图片数据。
func NewImageBlock(source ContentSource) ContentBlock {
	return ContentBlock{
		Type:   ContentBlockImage,
		Source: &source,
	}
}

// NewDocumentBlock 创建文档内容块。
//
// 支持通过 base64 编码、远程 URL 或纯文本传递文档数据。
func NewDocumentBlock(source ContentSource) ContentBlock {
	return ContentBlock{
		Type:   ContentBlockDocument,
		Source: &source,
	}
}

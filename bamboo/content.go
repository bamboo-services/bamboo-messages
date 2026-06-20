package bamboo

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bamboo-services/bamboo-messages/provider"
)

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

// ContentBlock 消息内容块接口，支持文本、思考过程、工具调用、图片和文档等多种类型。
type ContentBlock interface {
	BlockType() ContentBlockType
}

// ── 编译时接口合规检查 ──

var _ ContentBlock = (*TextBlock)(nil)
var _ ContentBlock = (*ThinkingBlock)(nil)
var _ ContentBlock = (*ToolUseBlock)(nil)
var _ ContentBlock = (*ToolResultBlock)(nil)
var _ ContentBlock = (*ImageBlock)(nil)
var _ ContentBlock = (*DocumentBlock)(nil)

// TextBlock 纯文本内容块。
type TextBlock struct {
	Type         ContentBlockType      `json:"type"`
	Text         string                `json:"text,omitempty"`
	CacheControl *provider.CacheControl `json:"cache_control,omitempty"`
}

func (b TextBlock) BlockType() ContentBlockType { return ContentBlockText }

// ThinkingBlock 思考过程内容块（如 Claude Extended Thinking）。
type ThinkingBlock struct {
	Type         ContentBlockType      `json:"type"`
	Thinking     string                `json:"thinking,omitempty"`
	Signature    string                `json:"signature,omitempty"`
	CacheControl *provider.CacheControl `json:"cache_control,omitempty"`
}

func (b ThinkingBlock) BlockType() ContentBlockType { return ContentBlockThinking }

// ToolUseBlock 工具调用内容块。
type ToolUseBlock struct {
	Type         ContentBlockType      `json:"type"`
	ID           string                `json:"id,omitempty"`
	Name         string                `json:"name,omitempty"`
	Input        json.RawMessage       `json:"input,omitempty"`
	CacheControl *provider.CacheControl `json:"cache_control,omitempty"`
}

func (b ToolUseBlock) BlockType() ContentBlockType { return ContentBlockToolUse }

// ToolResultBlock 工具调用结果内容块。
type ToolResultBlock struct {
	Type         ContentBlockType      `json:"type"`
	ToolUseID    string                `json:"tool_use_id,omitempty"`
	Content      string                `json:"content,omitempty"`
	IsError      bool                  `json:"is_error,omitempty"`
	CacheControl *provider.CacheControl `json:"cache_control,omitempty"`
}

func (b ToolResultBlock) BlockType() ContentBlockType { return ContentBlockToolResult }

// ImageBlock 图片内容块。
type ImageBlock struct {
	Type         ContentBlockType      `json:"type"`
	Source       *ContentSource        `json:"source,omitempty"`
	CacheControl *provider.CacheControl `json:"cache_control,omitempty"`
}

func (b ImageBlock) BlockType() ContentBlockType { return ContentBlockImage }

// DocumentBlock 文档内容块。
type DocumentBlock struct {
	Type         ContentBlockType      `json:"type"`
	Source       *ContentSource        `json:"source,omitempty"`
	CacheControl *provider.CacheControl `json:"cache_control,omitempty"`
}

func (b DocumentBlock) BlockType() ContentBlockType { return ContentBlockDocument }

// ── 内容块类型注册表 ──

var (
	blockTypeRegistry   = map[ContentBlockType]func() ContentBlock{}
	blockTypeRegistryMu sync.RWMutex
)

// RegisterBlockType 注册自定义内容块类型。
//
// 允许用户注册自定义的 ContentBlock 实现，使 JSON 反序列化时
// 能够根据 type 字段分派到正确的具体类型。
func RegisterBlockType(ct ContentBlockType, factory func() ContentBlock) {
	blockTypeRegistryMu.Lock()
	defer blockTypeRegistryMu.Unlock()
	blockTypeRegistry[ct] = factory
}

func init() {
	RegisterBlockType(ContentBlockText, func() ContentBlock { return &TextBlock{} })
	RegisterBlockType(ContentBlockThinking, func() ContentBlock { return &ThinkingBlock{} })
	RegisterBlockType(ContentBlockToolUse, func() ContentBlock { return &ToolUseBlock{} })
	RegisterBlockType(ContentBlockToolResult, func() ContentBlock { return &ToolResultBlock{} })
	RegisterBlockType(ContentBlockImage, func() ContentBlock { return &ImageBlock{} })
	RegisterBlockType(ContentBlockDocument, func() ContentBlock { return &DocumentBlock{} })
}

// ContentBlocks 内容块切片，支持 JSON 反序列化时根据 type 字段分派到具体类型。
type ContentBlocks []ContentBlock

// UnmarshalJSON 从 JSON 反序列化内容块数组。
func (cbs *ContentBlocks) UnmarshalJSON(data []byte) error {
	type blockWithType struct {
		Type ContentBlockType `json:"type"`
	}
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(data, &rawBlocks); err != nil {
		return err
	}
	result := make([]ContentBlock, 0, len(rawBlocks))
	for _, raw := range rawBlocks {
		var bwt blockWithType
		if err := json.Unmarshal(raw, &bwt); err != nil {
			return err
		}
		blockTypeRegistryMu.RLock()
		factory, ok := blockTypeRegistry[bwt.Type]
		blockTypeRegistryMu.RUnlock()
		if !ok {
			return fmt.Errorf("bamboo: unknown content block type %q", bwt.Type)
		}
		block := factory()
		if err := json.Unmarshal(raw, block); err != nil {
			return err
		}
		result = append(result, block)
	}
	*cbs = result
	return nil
}

// ── 构造函数 ──

// NewTextBlock 创建文本内容块。
//
// 适用于纯文本对话场景。
func NewTextBlock(text string) ContentBlock {
	return &TextBlock{Type: ContentBlockText, Text: text}
}

// NewTextBlockWithCache 创建带缓存标记的文本内容块。
func NewTextBlockWithCache(text string, cc *provider.CacheControl) ContentBlock {
	return &TextBlock{Type: ContentBlockText, Text: text, CacheControl: cc}
}

// NewThinkingBlock 创建思考过程内容块。
//
// thinking 为思考过程文本，signature 为用于验证的签名。
func NewThinkingBlock(thinking, signature string) ContentBlock {
	return &ThinkingBlock{Type: ContentBlockThinking, Thinking: thinking, Signature: signature}
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

	return &ToolUseBlock{Type: ContentBlockToolUse, ID: id, Name: name, Input: raw}
}

// NewToolUseBlockWithCache 创建带缓存标记的工具调用内容块。
func NewToolUseBlockWithCache(id, name string, input any, cc *provider.CacheControl) ContentBlock {
	block := NewToolUseBlock(id, name, input)
	if tub, ok := block.(*ToolUseBlock); ok {
		tub.CacheControl = cc
		return tub
	}
	return block
}

// NewToolResultBlock 创建工具调用结果内容块。
//
// 用于将工具调用的执行结果返回给模型。
func NewToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return &ToolResultBlock{Type: ContentBlockToolResult, ToolUseID: toolUseID, Content: content, IsError: isError}
}

// NewImageBlock 创建图片内容块。
//
// 支持通过 base64 编码或远程 URL 传递图片数据。
func NewImageBlock(source ContentSource) ContentBlock {
	return &ImageBlock{Type: ContentBlockImage, Source: &source}
}

// NewDocumentBlock 创建文档内容块。
//
// 支持通过 base64 编码、远程 URL 或纯文本传递文档数据。
func NewDocumentBlock(source ContentSource) ContentBlock {
	return &DocumentBlock{Type: ContentBlockDocument, Source: &source}
}

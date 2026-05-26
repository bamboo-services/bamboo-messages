package bamboo

// MessageRole 消息角色类型，标识消息发送方的身份。
type MessageRole string

const (
	// RoleUser 用户角色，表示由终端用户发送的消息。
	RoleUser MessageRole = "user"

	// RoleAssistant 助手角色，表示由 AI 模型生成的响应消息。
	RoleAssistant MessageRole = "assistant"
)

// BambooMessage 对话消息，包含角色和内容块列表。
//
// 一条消息可包含多个不同类型的内容块（文本、图片、工具调用等），
// 以支持多模态和工具交互场景。
type BambooMessage struct {
	// Role 消息发送方角色
	Role MessageRole `json:"role"`

	// Content 消息内容块列表
	Content []ContentBlock `json:"content"`
}

// NewUserMessage 创建包含单个文本内容块的用户消息。
func NewUserMessage(text string) BambooMessage {
	return BambooMessage{
		Role:    RoleUser,
		Content: []ContentBlock{NewTextBlock(text)},
	}
}

// NewUserMessageBlocks 创建包含多个内容块的用户消息。
//
// 适用于需要同时发送文本和图片、或多个内容块的场景。
func NewUserMessageBlocks(blocks ...ContentBlock) BambooMessage {
	return BambooMessage{
		Role:    RoleUser,
		Content: blocks,
	}
}

// NewAssistantMessage 创建包含单个文本内容块的助手消息。
func NewAssistantMessage(text string) BambooMessage {
	return BambooMessage{
		Role:    RoleAssistant,
		Content: []ContentBlock{NewTextBlock(text)},
	}
}

// NewAssistantMessageBlocks 创建包含多个内容块的助手消息。
//
// 适用于助手需要返回文本和工具调用结果的场景。
func NewAssistantMessageBlocks(blocks ...ContentBlock) BambooMessage {
	return BambooMessage{
		Role:    RoleAssistant,
		Content: blocks,
	}
}

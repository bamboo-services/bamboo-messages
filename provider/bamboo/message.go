package bamboo

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildMessages 将 provider.Message 列表转换为 bamboo 原生协议消息列表。
//
// 这是 bamboo/convert.go messagesToProvider 的逆向转换：
//   - RoleUser/RoleAssistant: 转换为 {role, content} 消息，content 为内容块数组
//   - RoleTool: 转换为 tool_result 块，合并到前一条 user 消息中（Anthropic 风格）
//   - RoleSystem: 不应出现（系统提示通过 system 参数传递），降级为 user
//
// 内容块顺序：thinking → redacted_thinking → text(Content) → image/document(ContentBlocks) → tool_use(ToolCalls)
func buildMessages(messages []provider.Message) []wireMessage {
	result := make([]wireMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleTool:
			block := buildToolResultBlock(msg)
			if tryMergeToolResult(result, block) {
				continue
			}
			result = append(result, wireMessage{
				Role:    string(provider.RoleUser),
				Content: []wireContentBlock{block},
			})
		default:
			result = append(result, buildUserOrAssistantMessage(msg))
		}
	}
	return result
}

// buildUserOrAssistantMessage 构建 user/assistant 角色的 wire 消息。
func buildUserOrAssistantMessage(msg provider.Message) wireMessage {
	blocks := buildWireContentBlocks(msg)
	// 确保至少有一个内容块（与 Anthropic buildMessages 一致）
	if len(blocks) == 0 {
		blocks = []wireContentBlock{{Type: "text", Text: ""}}
	}
	// 应用消息级 CacheControl 到对应类型的块
	applyCacheControl(blocks, msg.CacheControl, msg.CacheControlBlockType)
	wm := wireMessage{
		Role:    wireRole(msg.Role),
		Content: blocks,
	}
	if msg.ReasoningID != "" {
		wm.ReasoningID = msg.ReasoningID
	}
	return wm
}

// buildWireContentBlocks 从 provider.Message 构建内容块列表。
//
// 内容块构建顺序：
//  1. thinking block（ThinkingContent/ThinkingSignature 非空时）
//  2. redacted_thinking block（RedactedThinkingData 非空时）
//  3. text block（Content 字符串非空时）
//  4. ContentBlocks 中的 text/image/document 块
//  5. tool_use blocks（ToolCalls 列表）
func buildWireContentBlocks(msg provider.Message) []wireContentBlock {
	var blocks []wireContentBlock

	// Thinking block（多轮对话中保留 extended thinking 签名）
	if msg.ThinkingContent != "" || msg.ThinkingSignature != "" {
		blocks = append(blocks, wireContentBlock{
			Type:      "thinking",
			Thinking:  msg.ThinkingContent,
			Signature: msg.ThinkingSignature,
		})
	}

	// Redacted thinking block（多轮对话中原样传回加密 data）
	if msg.RedactedThinkingData != "" {
		blocks = append(blocks, wireContentBlock{
			Type: "redacted_thinking",
			Data: msg.RedactedThinkingData,
		})
	}

	// 文本内容（Content 字符串）
	if msg.Content != "" {
		blocks = append(blocks, wireContentBlock{
			Type: "text",
			Text: msg.Content,
		})
	}

	// 多媒体内容块（ContentBlocks，优先级高于 Content 但两者可共存）
	for _, cb := range msg.ContentBlocks {
		switch cb.BlockType() {
		case "text":
			if txt, ok := cb.(provider.TextContentBlock); ok {
				blocks = append(blocks, wireContentBlock{
					Type: "text",
					Text: txt.Text,
				})
			}
		case "image":
			if img, ok := cb.(provider.ImageContentBlock); ok {
				blocks = append(blocks, wireContentBlock{
					Type: "image",
					Source: &wireSource{
						Type:      img.Source.Type,
						MediaType: img.Source.MediaType,
						Data:      img.Source.Data,
						URL:       img.Source.URL,
					},
				})
			}
		case "document":
			if doc, ok := cb.(provider.DocumentContentBlock); ok {
				blocks = append(blocks, wireContentBlock{
					Type: "document",
					Source: &wireSource{
						Type:      doc.Source.Type,
						MediaType: doc.Source.MediaType,
						Data:      doc.Source.Data,
						URL:       doc.Source.URL,
					},
				})
			}
		}
	}

	// 工具调用
	for _, tc := range msg.ToolCalls {
		var input json.RawMessage
		if tc.Function.Arguments != "" {
			input = json.RawMessage(tc.Function.Arguments)
		} else {
			input = json.RawMessage(`{}`)
		}
		blocks = append(blocks, wireContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	return blocks
}

// buildToolResultBlock 从 RoleTool 的 provider.Message 构建 tool_result 块。
func buildToolResultBlock(msg provider.Message) wireContentBlock {
	block := wireContentBlock{
		Type:      "tool_result",
		ToolUseID: msg.ToolCallID,
		Content:   msg.Content,
		IsError:   msg.IsError,
	}
	if msg.ToolName != "" {
		block.ToolName = msg.ToolName
	}
	if msg.CacheControl != nil {
		block.CacheControl = marshalCacheControl(msg.CacheControl)
	}
	return block
}

// tryMergeToolResult 尝试将 tool_result 块合并到 result 中最后一条 user 消息。
//
// 合并条件：result 非空，最后一条消息为 user 角色，且其首个内容块为 tool_result 类型。
// 返回 true 表示合并成功，false 表示需要新建 user 消息。
func tryMergeToolResult(result []wireMessage, block wireContentBlock) bool {
	if len(result) == 0 {
		return false
	}
	last := &result[len(result)-1]
	if last.Role != string(provider.RoleUser) {
		return false
	}
	if len(last.Content) == 0 || last.Content[0].Type != "tool_result" {
		return false
	}
	last.Content = append(last.Content, block)
	return true
}

// applyCacheControl 将消息级 CacheControl 标记应用到内容块上。
//
// 优先将 cache_control 设置到 CacheControlBlockType 指定类型的最后一个块上；
// 若未找到对应类型或 CacheControlBlockType 为空，回退到最后一个块。
func applyCacheControl(blocks []wireContentBlock, cc *provider.CacheControl, blockType string) {
	if cc == nil || len(blocks) == 0 {
		return
	}
	ccBytes := marshalCacheControl(cc)
	// 优先：根据 blockType 找到对应类型的最后一个 block
	if blockType != "" {
		for i := len(blocks) - 1; i >= 0; i-- {
			if blocks[i].Type == blockType {
				blocks[i].CacheControl = ccBytes
				return
			}
		}
	}
	// 回退：应用到最后一个 block
	blocks[len(blocks)-1].CacheControl = ccBytes
}

// wireRole 将 provider.MessageRole 映射为 wire 消息角色字符串。
//
// bamboo 协议仅支持 user/assistant 两种消息角色；system 角色应通过
// wireRequest.System 传递，此处降级为 user。
func wireRole(role provider.MessageRole) string {
	switch role {
	case provider.RoleUser:
		return "user"
	case provider.RoleAssistant:
		return "assistant"
	case provider.RoleSystem:
		return "user"
	default:
		return string(role)
	}
}

// marshalCacheControl 将 provider.CacheControl 序列化为 json.RawMessage。
func marshalCacheControl(cc *provider.CacheControl) json.RawMessage {
	if cc == nil {
		return nil
	}
	data, err := json.Marshal(cc)
	if err != nil {
		return nil
	}
	return data
}

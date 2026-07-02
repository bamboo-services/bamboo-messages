package anthropic

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// buildMessages 将内部消息格式转换为 Anthropic Messages 协议格式。
//
// 根据 Role 构建对应的 Anthropic 消息结构：
//   - RoleUser:     {"role":"user","content":[{"type":"text","text":...}]}
//   - RoleAssistant: 支持文本、thinking block 和 tool_use blocks
//   - RoleTool:     {"role":"user","content":[{"type":"tool_result","tool_use_id":...}]}
//
// Anthropic API 要求同一轮的多个 tool_result 放在同一个 user 消息内，
// 因此当连续出现 RoleTool 时会自动合并而非新建消息。
func (p *Provider) buildMessages(messages []provider.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleTool:
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     msg.Content,
				"is_error":    msg.IsError,
			}
			if msg.ToolName != "" {
				block["name"] = msg.ToolName
			}
			if msg.CacheControl != nil {
				block["cache_control"] = buildCacheControl(msg.CacheControl)
			}
			// 检测前一条是否也是 tool_result（已转为 user），若是则合并到同一 user 消息
			if len(result) > 0 {
				last := result[len(result)-1]
				if last["role"] == "user" {
					if content, ok := last["content"].([]map[string]any); ok && len(content) > 0 {
						if content[0]["type"] == "tool_result" {
							last["content"] = append(content, block)
							continue
						}
					}
				}
			}
			result = append(result, map[string]any{
				"role":    "user",
				"content": []map[string]any{block},
			})
		default:
			result = p.appendMessage(result, msg)
		}
	}
	return result
}

// appendMessage 处理 RoleUser 和 RoleAssistant 消息（从 buildMessages 提取）。
func (p *Provider) appendMessage(result []map[string]any, msg provider.Message) []map[string]any {
	switch msg.Role {
	case provider.RoleUser:
		// ContentBlocks 优先于纯文本 Content
		if len(msg.ContentBlocks) > 0 {
			blocks := make([]map[string]any, 0, len(msg.ContentBlocks)+1)
			if msg.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
			}
			for _, cb := range msg.ContentBlocks {
				switch cb.BlockType() {
				case "text":
					if txt, ok := cb.(provider.TextContentBlock); ok {
						blocks = append(blocks, map[string]any{"type": "text", "text": txt.Text})
					}
				case "image":
					if img, ok := cb.(provider.ImageContentBlock); ok {
						blocks = append(blocks, buildImageBlock(img.Source))
					}
				case "document":
					if doc, ok := cb.(provider.DocumentContentBlock); ok {
						blocks = append(blocks, buildDocumentBlock(doc.Source))
					}
				}
			}
			applyMsgCacheControl(blocks, msg.CacheControl)
			result = append(result, map[string]any{
				"role":    "user",
				"content": blocks,
			})
		} else {
			block := map[string]any{"type": "text", "text": msg.Content}
			if msg.CacheControl != nil {
				block["cache_control"] = buildCacheControl(msg.CacheControl)
			}
			result = append(result, map[string]any{
				"role":    "user",
				"content": []map[string]any{block},
			})
		}

	case provider.RoleAssistant:
		blocks := make([]map[string]any, 0, len(msg.ToolCalls)+2)

		// Thinking block（多轮对话中保留 extended thinking 签名）
		if msg.ThinkingSignature != "" {
			blocks = append(blocks, map[string]any{
				"type":      "thinking",
				"thinking":  msg.ThinkingContent,
				"signature": msg.ThinkingSignature,
			})
		}

		// Redacted thinking block（多轮对话中原样传回加密 data）
		if msg.RedactedThinkingData != "" {
			blocks = append(blocks, map[string]any{
				"type": "redacted_thinking",
				"data": msg.RedactedThinkingData,
			})
		}

		// 文本内容
		if msg.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
		}

		// 工具调用
		for _, tc := range msg.ToolCalls {
			var input any
			if tc.Function.Arguments != "" {
				input = json.RawMessage(tc.Function.Arguments)
			} else {
				input = map[string]any{}
			}
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}

		// 确保至少有一个 content block
		if len(blocks) == 0 {
			blocks = append(blocks, map[string]any{"type": "text", "text": ""})
		}

		applyMsgCacheControl(blocks, msg.CacheControl)
		result = append(result, map[string]any{
			"role":    "assistant",
			"content": blocks,
		})
	}
	return result
}

// applyMsgCacheControl 将消息级别的 CacheControl 标记应用到最后一个 content block。
//
// Anthropic 的 cache_control 是块级断点，provider.Message.CacheControl 表示
// "这条消息的最后一个块需要缓存"，因此将标记设置到最后一个 block 上。
func applyMsgCacheControl(blocks []map[string]any, cc *provider.CacheControl) {
	if cc == nil || len(blocks) == 0 {
		return
	}
	blocks[len(blocks)-1]["cache_control"] = buildCacheControl(cc)
}

// buildImageBlock 根据图片来源构建 Anthropic 图片内容块。
//
// 支持两种来源：base64 内联数据和远程 URL。
func buildImageBlock(src provider.ImageSource) map[string]any {
	block := map[string]any{"type": "image"}
	switch src.Type {
	case "base64":
		block["source"] = map[string]any{
			"type":       "base64",
			"media_type": src.MediaType,
			"data":       src.Data,
		}
	case "url":
		block["source"] = map[string]any{
			"type": "url",
			"url":  src.URL,
		}
	}
	return block
}

// buildDocumentBlock 根据文档来源构建 Anthropic 文档内容块。
//
// 支持两种来源：base64 内联数据和远程 URL。
func buildDocumentBlock(src provider.DocumentSource) map[string]any {
	block := map[string]any{"type": "document"}
	switch src.Type {
	case "base64":
		source := map[string]any{
			"type":       "base64",
			"media_type": src.MediaType,
			"data":       src.Data,
		}
		block["source"] = source
	case "url":
		block["source"] = map[string]any{
			"type": "url",
			"url":  src.URL,
		}
	}
	return block
}

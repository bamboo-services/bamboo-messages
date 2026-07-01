package completions

import (
	"context"
	"fmt"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// buildMessages 将内部消息格式转换为 OpenAI Chat Completions API 消息格式。
//
// 将 provider.Message 映射为 map[string]any 形式的 System/User/Assistant/Tool 消息。
// 当 ContentBlocks 不为空时，用户消息会构造为多模态 content 数组。
func (p *CompletionsProvider) buildMessages(systemPrompt string, messages []provider.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages)+1)

	if systemPrompt != "" {
		result = append(result, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			if len(msg.ContentBlocks) > 0 {
				// 多模态消息：构造 content 数组
				parts := make([]map[string]any, 0, len(msg.ContentBlocks)+1)
				if msg.Content != "" {
					parts = append(parts, map[string]any{
						"type": "text",
						"text": msg.Content,
					})
				}
				for _, cb := range msg.ContentBlocks {
					switch cb.BlockType() {
					case "image":
						if img, ok := cb.(provider.ImageContentBlock); ok {
							if img.Source.Type == "base64" {
								dataURI := "data:" + img.Source.MediaType + ";base64," + img.Source.Data
								parts = append(parts, map[string]any{
									"type": "image_url",
									"image_url": map[string]any{
										"url":    dataURI,
										"detail": "auto",
									},
								})
							} else if img.Source.Type == "url" {
								parts = append(parts, map[string]any{
									"type": "image_url",
									"image_url": map[string]any{
										"url":    img.Source.URL,
										"detail": "auto",
									},
								})
							}
						}
					case "document":
						// 文档内容块：OpenAI Completions 不支持，记录警告后忽略
						if doc, ok := cb.(provider.DocumentContentBlock); ok {
							if doc.Source.Type == "url" || doc.Source.Type == "base64" {
							xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
								fmt.Sprintf("DocumentBlock(source=%q) 不支持，已忽略", doc.Source.Type))
						} else {
							xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
								fmt.Sprintf("DocumentBlock 未知来源类型=%q，已忽略", doc.Source.Type))
							}
						}
					}
				}
				result = append(result, map[string]any{
					"role":    "user",
					"content": parts,
				})
			} else {
				result = append(result, map[string]any{
					"role":    "user",
					"content": msg.Content,
				})
			}
		case provider.RoleAssistant:
			result = append(result, p.buildAssistantMessage(msg))
		case provider.RoleTool:
			result = append(result, map[string]any{
				"role":         "tool",
				"content":      msg.Content,
				"tool_call_id": msg.ToolCallID,
			})
		case provider.RoleSystem:
			// system 角色降级为 user（OpenAI 要求 system 仅在 messages 数组顶部出现一次）
			xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
				"检测到 system 角色消息，降级为 user 角色")
			result = append(result, map[string]any{
				"role":    "user",
				"content": msg.Content,
			})
		}
	}

	return result
}

// buildAssistantMessage 构建助手消息（支持文本和工具调用）。
//
// 将 provider.Message 映射为 map[string]any 形式的 Assistant 消息，包含 Content 和 ToolCalls。
// 仅在存在实际工具调用时填充 tool_calls，避免序列化出空数组 "tool_calls": []。
// 部分第三方 OpenAI 兼容端点（如 Kimi coding API）会将空数组视为无效请求，
// 导致返回 choices=0 的空响应。
func (p *CompletionsProvider) buildAssistantMessage(msg provider.Message) map[string]any {
	m := map[string]any{
		"role": "assistant",
	}

	if msg.Content != "" {
		m["content"] = msg.Content
	}

	// 仅在存在实际工具调用时填充 tool_calls，避免序列化出空数组
	if len(msg.ToolCalls) > 0 {
		toolCalls := make([]map[string]any, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			toolCalls = append(toolCalls, map[string]any{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]any{
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				},
			})
		}
		m["tool_calls"] = toolCalls
	}

	return m
}

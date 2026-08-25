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
//
// 工具消息的邻接约束：Chat Completions 语义要求 tool 消息必须连续紧跟
// 其对应的 assistant(tool_calls) 消息。若消息序列中 user 文本消息夹在两者之间
// （如 anthropic 入口的 user 消息同时含 text + tool_result），上游会以
// "An assistant message with 'tool_calls' must be followed by tool messages"
// 拒绝请求。因此 tool 消息通过声明位置映射被插入到紧跟 assistant 的位置，
// 保持并行工具响应的原始顺序。
func (p *CompletionsProvider) buildMessages(systemPrompt string, messages []provider.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages)+1)

	if systemPrompt != "" {
		result = append(result, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	// tool_call_id → 声明它的 assistant 在 result 中的位置
	declPos := map[string]int{}
	// assistant 位置 → 已插入的 tool 消息数（保证并行响应保持输入顺序）
	toolCountByAnchor := map[int]int{}

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
			am := p.buildAssistantMessage(msg)
			result = append(result, am)
			// 记录该 assistant 声明的所有 tool_call id → 在 result 中的位置
			if len(msg.ToolCalls) > 0 {
				placed := len(result) - 1
				for _, tc := range msg.ToolCalls {
					if tc.ID != "" {
						declPos[tc.ID] = placed
					}
				}
			}
		case provider.RoleTool:
			toolMsg := map[string]any{
				"role":         "tool",
				"content":      msg.Content,
				"tool_call_id": msg.ToolCallID,
			}
			// 将 tool 消息插入到声明它的 assistant 之后，保持邻接约束
			anchor, ok := declPos[msg.ToolCallID]
			if !ok {
				// 防御兜底：sanitizeToolMessages 应已过滤孤儿 tool 消息，
				// 此处追加到末尾保持兼容。
				xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
					fmt.Sprintf("warning: tool 消息(tool_call_id=%q) 无对应 assistant tool_call，追加到末尾", msg.ToolCallID))
				result = append(result, toolMsg)
				continue
			}
			insertPos := anchor + 1 + toolCountByAnchor[anchor]
			toolCountByAnchor[anchor]++
			result = append(result, nil)
			copy(result[insertPos+1:], result[insertPos:])
			result[insertPos] = toolMsg
			// 插入后，所有在 insertPos 及之后的元素后移一格，修正 declPos 位置
			for id, pos := range declPos {
				if pos >= insertPos {
					declPos[id] = pos + 1
				}
			}
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

	if msg.ThinkingContent != "" {
		m["reasoning_content"] = msg.ThinkingContent
	}
	if msg.ThinkingSignature != "" {
		m["thinking_signature"] = msg.ThinkingSignature
	}
	if msg.ThinkingSignatureProvider != "" {
		m["thinking_provider"] = msg.ThinkingSignatureProvider
	}
	if msg.ReasoningID != "" {
		m["reasoning_id"] = msg.ReasoningID
	}

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

	// OpenAI 要求 assistant 消息必须设置 content 或 tool_calls。
	// 当消息仅携带 reasoning_content（如 Responses API 的 reasoning 项转换而来）
	// 而无文本内容和工具调用时，补充空字符串 content 以满足上游校验，
	// 避免 "Invalid assistant message: content or tool_calls must be set" 错误。
	if _, hasContent := m["content"]; !hasContent {
		if _, hasToolCalls := m["tool_calls"]; !hasToolCalls {
			m["content"] = ""
		}
	}

	return m
}

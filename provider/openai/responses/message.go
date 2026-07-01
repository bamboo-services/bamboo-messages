package responses

import (
	"context"
	"fmt"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildInput 将内部消息格式转换为 OpenAI Responses API 输入格式。
//
// 将 provider.Message 数组转换为 Responses API 的 input 数组（[]map[string]any），
// 支持 user、assistant、tool 三种角色的消息。
// system 角色由 params.go 的 instructions 字段处理，此处不再注入。
func (p *ResponsesProvider) buildInput(messages []provider.Message) []map[string]any {
	items := make([]map[string]any, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			items = append(items, p.buildUserItem(msg))
		case provider.RoleAssistant:
			items = append(items, p.buildAssistantItem(msg)...)
		case provider.RoleTool:
			// 工具响应消息 → function_call_output 项
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  msg.Content,
			})
		}
	}

	return items
}

// buildUserItem 构建用户消息项。
//
// 支持纯文本和富文本（ContentBlocks）两种形式：
//   - 纯文本：{"role":"user","content":[{"type":"input_text","text": msg.Content}]}
//   - 富文本：遍历 ContentBlocks，映射到对应类型（input_text / input_image）
func (p *ResponsesProvider) buildUserItem(msg provider.Message) map[string]any {
	// 无内容块时，使用简单文本形式
	if len(msg.ContentBlocks) == 0 {
		return map[string]any{
			"role": "user",
			"content": []map[string]any{
				{
					"type": "input_text",
					"text": msg.Content,
				},
			},
		}
	}

	// 富文本形式：遍历内容块构建 content 数组
	parts := make([]map[string]any, 0, len(msg.ContentBlocks)+1)

	// 如果同时存在 Content 字符串，先添加为文本部分
	if msg.Content != "" {
		parts = append(parts, map[string]any{
			"type": "input_text",
			"text": msg.Content,
		})
	}

	for _, cb := range msg.ContentBlocks {
		switch cb.BlockType() {
		case "image":
			if img, ok := cb.(provider.ImageContentBlock); ok {
				switch img.Source.Type {
				case "base64":
					// base64 编码图片 → data URI 格式
					dataURI := "data:" + img.Source.MediaType + ";base64," + img.Source.Data
					parts = append(parts, map[string]any{
						"type":      "input_image",
						"image_url": dataURI,
					})
				case "url":
					// URL 图片直接传递
					parts = append(parts, map[string]any{
						"type":      "input_image",
						"image_url": img.Source.URL,
					})
				}
			}
		case "document":
			// 文档内容块：Responses API 不支持文档输入，记录警告后忽略
			if doc, ok := cb.(provider.DocumentContentBlock); ok {
				if doc.Source.Type == "url" || doc.Source.Type == "base64" {
					xLog.WithName("provider/openai-responses").SugarWarn(context.Background(),
						fmt.Sprintf("DocumentBlock(source=%q) 不支持，已忽略", doc.Source.Type))
				} else {
					xLog.WithName("provider/openai-responses").SugarWarn(context.Background(),
						fmt.Sprintf("DocumentBlock 未知来源类型=%q，已忽略", doc.Source.Type))
				}
			}
		}
	}

	return map[string]any{
		"role":    "user",
		"content": parts,
	}
}

// buildAssistantItem 构建助手消息项（支持文本、推理和工具调用）。
//
// 将助手消息（可能包含推理内容、文本和工具调用）转换为 Responses API 输入项列表，
// 一个助手消息可能拆分为多个输入项（reasoning + message + function_call）。
func (p *ResponsesProvider) buildAssistantItem(msg provider.Message) []map[string]any {
	items := make([]map[string]any, 0, len(msg.ToolCalls)+2)

	// Reasoning item — 推理内容（多轮对话中保留 thinking block 上下文）
	// 包含 reasoning_text summary 和 encrypted_content 用于服务端加密回传
	if msg.ThinkingContent != "" || msg.ThinkingSignature != "" {
		summary := []map[string]any{}
		if msg.ThinkingContent != "" {
			summary = append(summary, map[string]any{
				"type": "reasoning_text",
				"text": msg.ThinkingContent,
			})
		}
		reasoningItem := map[string]any{
			"type":    "reasoning",
			"summary": summary,
		}
		if msg.ReasoningID != "" {
			reasoningItem["id"] = msg.ReasoningID
		}
		if msg.ThinkingSignature != "" {
			reasoningItem["encrypted_content"] = msg.ThinkingSignature
		}
		items = append(items, reasoningItem)
	}

	// 文本内容 → assistant message item
	if msg.Content != "" {
		items = append(items, map[string]any{
			"role": "assistant",
			"content": []map[string]any{
				{
					"type": "output_text",
					"text": msg.Content,
				},
			},
		})
	}

	// 工具调用 → function_call items
	for _, tc := range msg.ToolCalls {
		items = append(items, map[string]any{
			"type":      "function_call",
			"call_id":   tc.ID,
			"name":      tc.Function.Name,
			"arguments": tc.Function.Arguments,
		})
	}

	return items
}

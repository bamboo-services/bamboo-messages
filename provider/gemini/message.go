package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ============================================

// buildMessages 将内部消息格式转换为 Gemini REST API 消息格式。
//
// 根据 Role 构建对应的 map[string]any（对应 Gemini Content 结构）：
//   - RoleUser:     role="user"，parts 包含文本或图片/文档
//   - RoleAssistant: role="model"，parts 包含文本和/或 functionCall
//   - RoleTool:     role="function"，parts 包含 functionResponse
func (p *Provider) buildMessages(messages []provider.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	toolCallMap := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == provider.RoleAssistant {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" && tc.Function.Name != "" {
					toolCallMap[tc.ID] = tc.Function.Name
				}
			}
		}
	}

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			result = append(result, p.buildUserMessage(msg))
		case provider.RoleAssistant:
			result = append(result, p.buildAssistantMessage(msg))
		case provider.RoleTool:
			result = append(result, p.buildToolMessage(msg, toolCallMap))
		}
	}
	return result
}

// buildUserMessage 构建用户消息。
//
// 当 ContentBlocks 存在时优先构建多 Part 消息（支持 image/document），
// 否则使用纯文本 Part。
func (p *Provider) buildUserMessage(msg provider.Message) map[string]any {
	if len(msg.ContentBlocks) > 0 {
		parts := make([]map[string]any, 0, len(msg.ContentBlocks)+1)
		if msg.Content != "" {
			parts = append(parts, map[string]any{"text": msg.Content})
		}
		for _, cb := range msg.ContentBlocks {
			switch cb.BlockType() {
			case "image":
				if img, ok := cb.(provider.ImageContentBlock); ok {
					if part := buildImagePart(img); part != nil {
						parts = append(parts, part)
					}
				}
			case "document":
				if doc, ok := cb.(provider.DocumentContentBlock); ok {
					if part := buildDocumentPart(doc); part != nil {
						parts = append(parts, part)
					}
				}
			}
		}
		return map[string]any{"role": "user", "parts": parts}
	}
	return map[string]any{
		"role":  "user",
		"parts": []map[string]any{{"text": msg.Content}},
	}
}

// buildAssistantMessage 构建助手消息。
//
// 当存在 ToolCalls 时，构建包含文本和 functionCall 的多 Part 消息；
// 否则构建纯文本消息。FunctionCall 的 ID 在为空时合成。
func (p *Provider) buildAssistantMessage(msg provider.Message) map[string]any {
	parts := make([]map[string]any, 0, len(msg.ToolCalls)+2)
	if thought := buildThoughtPart(msg); thought != nil {
		parts = append(parts, thought)
	}
	if len(msg.ToolCalls) > 0 {
		if msg.Content != "" {
			parts = append(parts, map[string]any{"text": msg.Content})
		}
		for i, tc := range msg.ToolCalls {
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("gemini_call_%s_%d", tc.Function.Name, i)
			}
			// Args 使用 json.RawMessage 保留原始 JSON，空参数时使用空对象
			var args any = json.RawMessage("{}")
			if tc.Function.Arguments != "" {
				args = json.RawMessage(tc.Function.Arguments)
			}
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"id":   id,
					"name": tc.Function.Name,
					"args": args,
				},
			})
		}
		if len(parts) == 0 {
			parts = append(parts, map[string]any{"text": msg.Content})
		}
		return map[string]any{"role": "model", "parts": parts}
	}
	if msg.Content != "" {
		parts = append(parts, map[string]any{"text": msg.Content})
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": msg.Content})
	}
	return map[string]any{"role": "model", "parts": parts}
}

func buildThoughtPart(msg provider.Message) map[string]any {
	if !provider.NativeThinkingCredential(msg.ThinkingSignature, msg.ThinkingSignatureProvider, provider.SignatureProviderGemini) {
		return nil
	}
	part := map[string]any{
		"thought":          true,
		"thoughtSignature": msg.ThinkingSignature,
	}
	if msg.ThinkingContent != "" {
		part["text"] = msg.ThinkingContent
	}
	return part
}

// buildToolMessage 构建工具响应消息。
//
// Gemini 要求 FunctionResponse 放在 role="function" 的 Content 中。
// 优先使用 ToolName（函数名），若为空则尝试从 toolCallMap 中根据 ToolCallID 反查函数名，
// 回退到 ToolCallID；两者都为空时使用 fallback。
// Response 使用 json.RawMessage 保留原始 JSON，避免在 DTO 层做类型假设。
func (p *Provider) buildToolMessage(msg provider.Message, toolCallMap map[string]string) map[string]any {
	name := msg.ToolName
	if name == "" && msg.ToolCallID != "" && toolCallMap != nil {
		if fnName, ok := toolCallMap[msg.ToolCallID]; ok && fnName != "" {
			name = fnName
		}
	}
	if name == "" {
		name = msg.ToolCallID
	}
	if name == "" {
		name = "tool_response"
	}

	// 构建响应体：{output: content} 或 {output: content, error: content}
	responseMap := map[string]any{"output": msg.Content}
	if msg.IsError {
		responseMap["error"] = msg.Content
	}
	responseBytes, _ := json.Marshal(responseMap)

	return map[string]any{
		"role": "function",
		"parts": []map[string]any{{
			"functionResponse": map[string]any{
				"id":       msg.ToolCallID,
				"name":     name,
				"response": json.RawMessage(responseBytes),
			},
		}},
	}
}

// buildImagePart 构建 image Part。
//
// 根据 Source.Type 选择 inline（base64 原样传递）或 file URI 方式。
// 返回 nil 表示无法识别的图片来源，调用方应跳过。
func buildImagePart(img provider.ImageContentBlock) map[string]any {
	if img.Source.Type == "base64" {
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": img.Source.MediaType,
				"data":     img.Source.Data,
			},
		}
	}
	if img.Source.Type == "url" {
		return map[string]any{
			"fileData": map[string]any{
				"fileUri":  img.Source.URL,
				"mimeType": img.Source.MediaType,
			},
		}
	}
	return nil
}

// buildDocumentPart 构建 document Part。
//
// 与 buildImagePart 相同的策略，base64 → inlineData，url → fileData。
func buildDocumentPart(doc provider.DocumentContentBlock) map[string]any {
	if doc.Source.Type == "base64" {
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": doc.Source.MediaType,
				"data":     doc.Source.Data,
			},
		}
	}
	if doc.Source.Type == "url" {
		return map[string]any{
			"fileData": map[string]any{
				"fileUri":  doc.Source.URL,
				"mimeType": doc.Source.MediaType,
			},
		}
	}
	return nil
}

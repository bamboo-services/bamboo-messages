package gemini

import (
	"encoding/base64"
	"fmt"

	"github.com/bamboo-services/bamboo-messages/provider"
	"google.golang.org/genai"
)

// ==============================
// 内部方法
// ============================================

// buildMessages 将内部消息格式转换为 Gemini SDK 消息格式。
//
// 根据 Role 构建对应的 genai.Content：
//   - RoleUser: NewContentFromText 或带 image/document 的多 Part Content
//   - RoleAssistant: model 角色，支持文本 + FunctionCall parts
//   - RoleTool: user 角色，包含 FunctionResponse part
func (p *Provider) buildMessages(messages []provider.Message) []*genai.Content {
	result := make([]*genai.Content, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			result = append(result, p.buildUserMessage(msg))
		case provider.RoleAssistant:
			result = append(result, p.buildAssistantMessage(msg))
		case provider.RoleTool:
			result = append(result, p.buildToolMessage(msg))
		}
	}
	return result
}

// buildUserMessage 构建用户消息 Content。
//
// 当 ContentBlocks 存在时优先构建多 Part Content（支持 image/document），
// 否则使用纯文本 Content。
func (p *Provider) buildUserMessage(msg provider.Message) *genai.Content {
	if len(msg.ContentBlocks) > 0 {
		parts := make([]*genai.Part, 0, len(msg.ContentBlocks)+1)
		if msg.Content != "" {
			parts = append(parts, &genai.Part{Text: msg.Content})
		}
		for _, cb := range msg.ContentBlocks {
			switch cb.BlockType() {
			case "image":
				if img, ok := cb.(provider.ImageContentBlock); ok {
					parts = append(parts, buildImagePart(img))
				}
			case "document":
				if doc, ok := cb.(provider.DocumentContentBlock); ok {
					parts = append(parts, buildDocumentPart(doc))
				}
			}
		}
		return &genai.Content{Parts: parts, Role: "user"}
	}
	return genai.NewContentFromText(msg.Content, "user")
}

// buildAssistantMessage 构建助手消息 Content。
//
// 当存在 ToolCalls 时，构建包含文本和 FunctionCall 的多 Part Content；
// 否则构建纯文本 Content。FunctionCall 的 ID 在为空时合成。
func (p *Provider) buildAssistantMessage(msg provider.Message) *genai.Content {
	if len(msg.ToolCalls) > 0 {
		parts := make([]*genai.Part, 0, len(msg.ToolCalls)+1)
		if msg.Content != "" {
			parts = append(parts, &genai.Part{Text: msg.Content})
		}
		for i, tc := range msg.ToolCalls {
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("gemini_call_%s_%d", tc.Function.Name, i)
			}
			args := parseArgs(tc.Function.Arguments)
			parts = append(parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   id,
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}
		return &genai.Content{Parts: parts, Role: "model"}
	}
	return genai.NewContentFromText(msg.Content, "model")
}

// buildToolMessage 构建工具响应消息 Content。
//
// Gemini 要求 FunctionResponse 放在 user 角色的 Content 中。
// tool name 通过 ToolCallID 反查；若上层未提供 name，使用 fallback。
func (p *Provider) buildToolMessage(msg provider.Message) *genai.Content {
	name := msg.ToolCallID
	if name == "" {
		name = "tool_response"
	}
	response := map[string]any{"output": msg.Content}
	if msg.IsError {
		response["error"] = msg.Content
	}
	return genai.NewContentFromFunctionResponse(name, response, "user")
}

// buildImagePart 构建 image Part。
//
// 根据 Source.Type 选择 inline (base64) 或 file URI 方式。
func buildImagePart(img provider.ImageContentBlock) *genai.Part {
	if img.Source.Type == "base64" {
		data, err := base64.StdEncoding.DecodeString(img.Source.Data)
		if err != nil {
			return &genai.Part{Text: "[图片解码失败]"}
		}
		return &genai.Part{
			InlineData: &genai.Blob{
				Data:     data,
				MIMEType: img.Source.MediaType,
			},
		}
	}
	if img.Source.Type == "url" {
		return &genai.Part{
			FileData: &genai.FileData{
				FileURI:  img.Source.URL,
				MIMEType: img.Source.MediaType,
			},
		}
	}
	return &genai.Part{Text: "[未知图片来源]"}
}

// buildDocumentPart 构建 document Part。
//
// 与 buildImagePart 相同的策略，base64 → Blob，url → FileData。
func buildDocumentPart(doc provider.DocumentContentBlock) *genai.Part {
	if doc.Source.Type == "base64" {
		data, err := base64.StdEncoding.DecodeString(doc.Source.Data)
		if err != nil {
			return &genai.Part{Text: "[文档解码失败]"}
		}
		return &genai.Part{
			InlineData: &genai.Blob{
				Data:     data,
				MIMEType: doc.Source.MediaType,
			},
		}
	}
	if doc.Source.Type == "url" {
		return &genai.Part{
			FileData: &genai.FileData{
				FileURI:  doc.Source.URL,
				MIMEType: doc.Source.MediaType,
			},
		}
	}
	return &genai.Part{Text: "[未知文档来源]"}
}

// parseArgs 将 JSON 字符串参数解析为 map[string]any。
//
// 解析失败时返回空 map，保证 Gemini API 调用不报错。
func parseArgs(jsonArgs string) map[string]any {
	args := map[string]any{}
	if jsonArgs == "" {
		return args
	}
	var parsed map[string]any
	if err := jsonUnmarshal([]byte(jsonArgs), &parsed); err == nil {
		return parsed
	}
	return args
}

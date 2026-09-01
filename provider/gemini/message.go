package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

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
		if len(parts) == 0 {
			parts = append(parts, map[string]any{"text": msg.Content})
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
// Gemini Part.data 是 oneof：必须有 text / functionCall / inlineData / fileData 之一。
// thought + thoughtSignature 只是 sidecar 字段，单独发出会被上游拒绝为
// "Unsupported input part type: go/debugstr"。host-tool hop2 回灌时尤其常见——
// Gemini 把 thoughtSignature 打在 functionCall 同一 part 上，IR 拆成空 ThinkingBlock
// 后若再编成无正文的 thought part 就会 500。
func (p *Provider) buildAssistantMessage(msg provider.Message) map[string]any {
	parts := make([]map[string]any, 0, len(msg.ToolCalls)+2)

	nativeSig := ""
	if provider.NativeThinkingCredential(msg.ThinkingSignature, msg.ThinkingSignatureProvider, provider.SignatureProviderGemini) {
		nativeSig = msg.ThinkingSignature
	}

	// 无 Gemini 血统的思考不能编 thought:true。签名也只在有 data oneof 的 part 上挂。
	if msg.ThinkingContent != "" && nativeSig != "" {
		thought := map[string]any{
			"text":    msg.ThinkingContent,
			"thought": true,
		}
		if len(msg.ToolCalls) == 0 {
			thought["thoughtSignature"] = nativeSig
			nativeSig = ""
		}
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
			part := map[string]any{
				"functionCall": map[string]any{
					"id":   id,
					"name": tc.Function.Name,
					"args": geminiFunctionArgs(tc.Function.Arguments),
				},
			}
			if i == 0 && nativeSig != "" {
				part["thoughtSignature"] = nativeSig
				nativeSig = ""
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 {
			parts = append(parts, map[string]any{"text": msg.Content})
		}
		return map[string]any{"role": "model", "parts": parts}
	}

	if msg.Content != "" {
		textPart := map[string]any{"text": msg.Content}
		if nativeSig != "" {
			textPart["thoughtSignature"] = nativeSig
		}
		parts = append(parts, textPart)
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": msg.Content})
	}
	return map[string]any{"role": "model", "parts": parts}
}

// geminiFunctionArgs 把工具参数规范成 Gemini FunctionCall.args（protobuf Struct = JSON object）。
func geminiFunctionArgs(raw string) json.RawMessage {
	s := strings.TrimSpace(raw)
	if s == "" || s[0] != '{' || !json.Valid([]byte(s)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(s)
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
// Gemini generateContent 接受的图片形态：
//   - inlineData：mimeType + 裸 base64（官方粘贴/内联图路径，data URI 前缀必须剥掉）
//   - fileData：Files API / GCS / YouTube 等 fileUri，不是 data URI，也通常不是任意 HTTP 图
//
// data URI 若走 fileData.fileUri，上游会报 Unsupported input part type。
func buildImagePart(img provider.ImageContentBlock) map[string]any {
	return buildBlobPart(img.Source.Type, img.Source.MediaType, img.Source.Data, img.Source.URL, "image/png")
}

// buildDocumentPart 构建 document Part。
//
// 与 buildImagePart 相同的策略，base64 / data URI → inlineData，普通 URL → fileData。
func buildDocumentPart(doc provider.DocumentContentBlock) map[string]any {
	return buildBlobPart(doc.Source.Type, doc.Source.MediaType, doc.Source.Data, doc.Source.URL, "application/octet-stream")
}

func buildBlobPart(sourceType, mediaType, data, url, defaultMIME string) map[string]any {
	if inline := geminiInlineData(sourceType, mediaType, data, url, defaultMIME); inline != nil {
		return map[string]any{"inlineData": inline}
	}
	if sourceType == "url" && strings.TrimSpace(url) != "" && !strings.HasPrefix(strings.TrimSpace(url), "data:") {
		fileData := map[string]any{"fileUri": strings.TrimSpace(url)}
		if mime := strings.TrimSpace(mediaType); mime != "" {
			fileData["mimeType"] = mime
		}
		return map[string]any{"fileData": fileData}
	}
	return nil
}

func geminiInlineData(sourceType, mediaType, data, url, defaultMIME string) map[string]any {
	if mime, b64, ok := parseBase64DataURI(url); ok {
		if strings.TrimSpace(mediaType) == "" {
			mediaType = mime
		}
		return map[string]any{"mimeType": fallbackMIME(mediaType, defaultMIME), "data": b64}
	}
	if mime, b64, ok := parseBase64DataURI(data); ok {
		if strings.TrimSpace(mediaType) == "" {
			mediaType = mime
		}
		return map[string]any{"mimeType": fallbackMIME(mediaType, defaultMIME), "data": b64}
	}
	if sourceType == "base64" {
		b64 := strings.TrimSpace(data)
		if b64 == "" {
			return nil
		}
		return map[string]any{"mimeType": fallbackMIME(mediaType, defaultMIME), "data": b64}
	}
	return nil
}

func fallbackMIME(mime, defaultMIME string) string {
	if strings.TrimSpace(mime) == "" {
		return defaultMIME
	}
	return mime
}

// parseBase64DataURI 解析 data:<mime>;base64,<payload>。
// Gemini inlineData.data 只要裸 base64，不能带 data URI 头。
func parseBase64DataURI(s string) (mime, data string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "data:") {
		return "", "", false
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return "", "", false
	}
	header := s[len("data:"):comma]
	payload := s[comma+1:]
	if payload == "" || !strings.Contains(header, "base64") {
		return "", "", false
	}
	mime = header
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return mime, payload, true
}

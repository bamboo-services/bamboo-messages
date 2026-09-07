package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// toolResultRecord 缓存一条 RoleTool 的函数响应，供按 tool_call_id 回填。
type toolResultRecord struct {
	content string
	isError bool
}

// ==============================
// 内部方法
// ============================================

// buildMessages 将内部消息格式转换为 Gemini REST API 消息格式。
//
// Gemini 对 function call 历史是强校验：一轮 model 的 N 个 functionCall，
// 必须紧跟一条 role="user" content，内含 N 个同序同名的 functionResponse。
// 并行工具结果不能拆成多条 content，也不能让两条带 functionCall 的 model
// content 相邻（流式中断切分 assistant 时会出现）。
//
// 组装策略：
//   - RoleUser:     role="user"，parts 包含文本或图片/文档
//   - RoleAssistant: role="model"；若含 ToolCalls，立刻再追加一条闭合的
//     functionResponse user content（缺失结果时注入 dummy error）
//   - RoleTool:     不单独成条，按 tool_call_id 收集后挂到声明它的 assistant 后面
func (p *Provider) buildMessages(messages []provider.Message) []map[string]any {
	results := collectToolResults(messages)
	result := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			result = append(result, p.buildUserMessage(msg))
		case provider.RoleAssistant:
			result = append(result, p.buildAssistantMessage(msg))
			if len(msg.ToolCalls) > 0 {
				result = append(result, buildFunctionResponseContent(msg.ToolCalls, results))
			}
		case provider.RoleTool:
			// 已由 collectToolResults 收集，挂到对应 assistant 之后。
		}
	}
	return result
}

// collectToolResults 按 tool_call_id 收集 RoleTool 响应，同一 id 仅保留第一条。
func collectToolResults(messages []provider.Message) map[string]toolResultRecord {
	results := make(map[string]toolResultRecord)
	for _, msg := range messages {
		if msg.Role != provider.RoleTool {
			continue
		}
		id := msg.ToolCallID
		if id == "" {
			continue
		}
		if _, exists := results[id]; exists {
			continue
		}
		results[id] = toolResultRecord{content: msg.Content, isError: msg.IsError}
	}
	return results
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
			part := map[string]any{
				"functionCall": map[string]any{
					"id":   functionCallID(tc, i),
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

// buildFunctionResponseContent 将一轮 ToolCalls 闭合为单条 user content。
//
// parts 顺序与 functionCall 声明顺序一致；name 始终取函数名。
// 缺失的工具结果注入 dummy error，避免 Gemini 因历史不完整拒绝请求。
func buildFunctionResponseContent(calls []provider.ToolCall, results map[string]toolResultRecord) map[string]any {
	parts := make([]map[string]any, 0, len(calls))
	for i, tc := range calls {
		parts = append(parts, buildFunctionResponsePart(tc, i, results))
	}
	return map[string]any{
		"role":  "user",
		"parts": parts,
	}
}

func buildFunctionResponsePart(tc provider.ToolCall, index int, results map[string]toolResultRecord) map[string]any {
	name := tc.Function.Name
	if name == "" {
		name = "tool_response"
	}
	id := functionCallID(tc, index)

	responseMap := map[string]any{}
	if rec, ok := results[tc.ID]; ok && tc.ID != "" {
		responseMap["output"] = rec.content
		if rec.isError {
			responseMap["error"] = rec.content
		}
	} else {
		xLog.WithName("provider/gemini").SugarWarn(context.Background(),
			fmt.Sprintf("tool_call(id=%q name=%q) 缺少 tool_result，已注入 dummy functionResponse", tc.ID, name))
		responseMap["error"] = "tool result missing"
	}
	responseBytes, _ := json.Marshal(responseMap)

	return map[string]any{
		"functionResponse": map[string]any{
			"id":       id,
			"name":     name,
			"response": json.RawMessage(responseBytes),
		},
	}
}

func functionCallID(tc provider.ToolCall, index int) string {
	if tc.ID != "" {
		return tc.ID
	}
	return fmt.Sprintf("gemini_call_%s_%d", tc.Function.Name, index)
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

package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// ── Gemini 请求 JSON 结构体 ──

// geminiRequest Gemini GenerateContent 请求体结构。
//
// 参考: https://ai.google.dev/api/rest/v1/models/generateContent
type geminiRequest struct {
	Contents           []geminiContent      `json:"contents"`
	SystemInstruction  *geminiContent       `json:"systemInstruction,omitempty"`
	GenerationConfig   *geminiGenConfig     `json:"generationConfig,omitempty"`
	Tools              []geminiTool         `json:"tools,omitempty"`
	ToolConfig         *geminiToolConfig    `json:"toolConfig,omitempty"`
	SafetySettings     []geminiSafetySetting `json:"safetySettings,omitempty"`
	CachedContent      string               `json:"cachedContent,omitempty"`
}

// geminiContent Gemini Content 结构 (role + parts)。
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart Gemini Part 联合体，使用 RawMessage 灵活解析。
//
// Gemini 的 Part 可能是:
//   - text: {"text": "..."}
//   - inlineData: {"inlineData": {"mimeType": "...", "data": "base64..."}}
//   - fileData: {"fileData": {"mimeType": "...", "fileUri": "..."}}
//   - functionCall: {"functionCall": {"id": "...", "name": "...", "args": {...}}}
//   - functionResponse: {"functionResponse": {"id": "...", "name": "...", "response": {...}}}
//   - executableCode / codeExecutionResult 等扩展类型
type geminiPart struct {
	Text             string                `json:"text,omitempty"`
	InlineData       *geminiInlineData     `json:"inlineData,omitempty"`
	FileData         *geminiFileData       `json:"fileData,omitempty"`
	FunctionCall     *geminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFuncResponse   `json:"functionResponse,omitempty"`
}

// geminiInlineData 内联二进制数据 (base64 编码)。
type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// geminiFileData 文件引用 (fileUri)。
type geminiFileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

// geminiFunctionCall 函数调用声明。
type geminiFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// geminiFuncResponse 函数调用结果。
type geminiFuncResponse struct {
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
}

// geminiGenConfig 生成配置。
type geminiGenConfig struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	TopK             *float64 `json:"topK,omitempty"`
	MaxOutputTokens  *int64   `json:"maxOutputTokens,omitempty"`
	StopSequences    []string `json:"stopSequences,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
}

// geminiTool 工具定义（包含 functionDeclarations 数组）。
type geminiTool struct {
	FunctionDeclarations []geminiFuncDecl `json:"functionDeclarations,omitempty"`
}

// geminiFuncDecl 函数声明。
type geminiFuncDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// geminiToolConfig 工具调用配置。
type geminiToolConfig struct {
	FunctionCallingConfig *geminiFuncCallingConfig `json:"functionCallingConfig,omitempty"`
}

// geminiFuncCallingConfig 函数调用配置。
type geminiFuncCallingConfig struct {
	Mode string `json:"mode,omitempty"`
}

// geminiSafetySetting 安全设置（透传）。
type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// parseRequest 将 Gemini GenerateContent 请求体解析为 RelayRequest。
func parseRequest(body []byte) (*codec.RelayRequest, error) {
	var req geminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, codec.NewErrorWithCause(codec.ErrInvalidRequest, "failed to parse gemini request body", err)
	}

	// ── 1. 解析 systemInstruction ──
	var system string
	if req.SystemInstruction != nil {
		system = extractSystemText(req.SystemInstruction)
	}

	// ── 2. 解析 contents ──
	messages, err := parseContents(req.Contents)
	if err != nil {
		return nil, err
	}

	// ── 3. 构建配置 ──
	config := &bamboo.RequestConfig{}
	applyGenerationConfig(config, req.GenerationConfig)

	// ── 4. 工具定义 ──
	if len(req.Tools) > 0 {
		config.Tools = parseTools(req.Tools)
	}

	// ── 5. toolConfig ──
	if req.ToolConfig != nil && req.ToolConfig.FunctionCallingConfig != nil {
		config.ToolChoice = mapToolChoiceMode(req.ToolConfig.FunctionCallingConfig.Mode)
	}

	// ── 6. safetySettings 透传到 ProviderExtra ──
	if len(req.SafetySettings) > 0 {
		if config.ProviderExtra == nil {
			config.ProviderExtra = make(map[string]any)
		}
		config.ProviderExtra["safety_settings"] = req.SafetySettings
	}

	// ── 7. cachedContent 透传到 ProviderExtra ──
	if req.CachedContent != "" {
		if config.ProviderExtra == nil {
			config.ProviderExtra = make(map[string]any)
		}
		config.ProviderExtra["cached_content"] = req.CachedContent
	}

	return &codec.RelayRequest{
		Messages: messages,
		System:   system,
		Config:   config,
		IsStream: false, // Gemini stream 由 URL 参数 `?alt=sse` 决定，body 中不包含
	}, nil
}

// extractSystemText 从 systemInstruction 中拼接所有 text parts。
func extractSystemText(content *geminiContent) string {
	if content == nil || len(content.Parts) == 0 {
		return ""
	}
	var texts []string
	for _, part := range content.Parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// parseContents 将 Gemini contents 数组转换为 Bamboo 消息列表。
//
// 角色映射:
//   - "user" → user
//   - "model" → assistant
//   - "function" → user + ToolResultBlock
func parseContents(contents []geminiContent) ([]bamboo.BambooMessage, error) {
	// 全局 functionCall 序号计数器，用于合成 ID
	callIndex := 0
	result := make([]bamboo.BambooMessage, 0, len(contents))

	for _, content := range contents {
		switch content.Role {
		case "user":
			blocks, err := parseParts(content.Parts, &callIndex)
			if err != nil {
				return nil, err
			}
			if len(blocks) == 0 {
				result = append(result, bamboo.BambooMessage{Role: bamboo.RoleUser})
			} else {
				result = append(result, bamboo.NewUserMessageBlocks(blocks...))
			}

		case "model":
			blocks, err := parseParts(content.Parts, &callIndex)
			if err != nil {
				return nil, err
			}
			if len(blocks) == 0 {
				result = append(result, bamboo.BambooMessage{Role: bamboo.RoleAssistant})
			} else {
				result = append(result, bamboo.NewAssistantMessageBlocks(blocks...))
			}

		case "function":
			// Gemini "function" 角色 → user + ToolResultBlock
			blocks := make([]bamboo.ContentBlock, 0, len(content.Parts))
			for _, part := range content.Parts {
				if part.FunctionResponse != nil {
					toolUseID := part.FunctionResponse.ID
					if toolUseID == "" {
						toolUseID = part.FunctionResponse.Name
					}
					contentStr := serializeFuncResponse(part.FunctionResponse.Response)
					blocks = append(blocks, bamboo.NewToolResultBlock(toolUseID, contentStr, false))
				}
			}
			if len(blocks) == 0 {
				result = append(result, bamboo.BambooMessage{Role: bamboo.RoleUser})
			} else {
				result = append(result, bamboo.NewUserMessageBlocks(blocks...))
			}

		default:
			// 未知角色按 user 处理
			blocks, _ := parseParts(content.Parts, &callIndex)
			if len(blocks) == 0 {
				result = append(result, bamboo.BambooMessage{Role: bamboo.RoleUser})
			} else {
				result = append(result, bamboo.NewUserMessageBlocks(blocks...))
			}
		}
	}

	return result, nil
}

// parseParts 解析 Gemini Parts 为 Bamboo ContentBlock 列表。
//
// callIndex 用于为无 ID 的 functionCall 合成全局唯一 ID。
func parseParts(parts []geminiPart, callIndex *int) ([]bamboo.ContentBlock, error) {
	blocks := make([]bamboo.ContentBlock, 0, len(parts))
	for _, part := range parts {
		// text → TextBlock
		if part.Text != "" {
			blocks = append(blocks, bamboo.NewTextBlock(part.Text))
			continue
		}

		// inlineData → ImageBlock (base64)
		if part.InlineData != nil {
			blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
				Type:      "base64",
				MediaType: part.InlineData.MimeType,
				Data:      part.InlineData.Data,
			}))
			continue
		}

		// fileData → ImageBlock (url)
		if part.FileData != nil {
			blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
				Type:      "url",
				URL:       part.FileData.FileURI,
				MediaType: part.FileData.MimeType,
			}))
			continue
		}

		// functionCall → ToolUseBlock
		if part.FunctionCall != nil {
			id := part.FunctionCall.ID
			if id == "" {
				id = fmt.Sprintf("gemini_call_%s_%d", part.FunctionCall.Name, *callIndex)
			}
			*callIndex++

			input := part.FunctionCall.Args
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			blocks = append(blocks, &bamboo.ToolUseBlock{
				Type:  bamboo.ContentBlockToolUse,
				ID:    id,
				Name:  part.FunctionCall.Name,
				Input: input,
			})
			continue
		}

		// functionResponse → ToolResultBlock
		if part.FunctionResponse != nil {
			toolUseID := part.FunctionResponse.ID
			if toolUseID == "" {
				toolUseID = part.FunctionResponse.Name
			}
			contentStr := serializeFuncResponse(part.FunctionResponse.Response)
			blocks = append(blocks, bamboo.NewToolResultBlock(toolUseID, contentStr, false))
			continue
		}
	}
	return blocks, nil
}

// serializeFuncResponse 将 functionResponse.response 序列化为字符串。
//
// Gemini 的 response 是任意 JSON，统一序列化为字符串以适配 ToolResultBlock.Content。
func serializeFuncResponse(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试提取 {"output": "..."} 的 output 字段（Gemini convention）
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if outputRaw, ok := wrapper["output"]; ok {
			// output 可能是字符串或 JSON 值
			var s string
			if json.Unmarshal(outputRaw, &s) == nil {
				return s
			}
		}
	}
	// fallback: 使用原始 JSON 字符串
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// applyGenerationConfig 将 Gemini generationConfig 映射到 RequestConfig。
func applyGenerationConfig(config *bamboo.RequestConfig, gc *geminiGenConfig) {
	if gc == nil {
		return
	}

	if gc.Temperature != nil {
		config.Temperature = gc.Temperature
	}
	if gc.TopP != nil {
		config.TopP = gc.TopP
	}
	if gc.MaxOutputTokens != nil {
		config.MaxTokens = *gc.MaxOutputTokens
	}
	if len(gc.StopSequences) > 0 {
		config.StopSequences = gc.StopSequences
	}
	if gc.ResponseMimeType != "" {
		config.ResponseFormat = mapResponseMimeType(gc.ResponseMimeType)
	}

	// topK → ProviderExtra
	if gc.TopK != nil {
		if config.ProviderExtra == nil {
			config.ProviderExtra = make(map[string]any)
		}
		config.ProviderExtra["top_k"] = *gc.TopK
	}
}

// mapResponseMimeType 将 Gemini responseMimeType 映射为 bamboo ResponseFormat。
//
// "application/json" → "json_object"
// "text/plain"       → "text"
// "text/x.enum"      → "text"（降级）
// 其他                → 原值透传
func mapResponseMimeType(mime string) string {
	switch mime {
	case "application/json":
		return "json_object"
	case "text/plain", "text/x.enum":
		return "text"
	default:
		return mime
	}
}

// mapToolChoiceMode 将 Gemini toolConfig mode 映射为 bamboo ToolChoice。
//
// AUTO → "auto"
// NONE → "none"
// ANY  → "required"
func mapToolChoiceMode(mode string) string {
	switch mode {
	case "AUTO":
		return "auto"
	case "NONE":
		return "none"
	case "ANY":
		return "required"
	default:
		return mode
	}
}

// parseTools 将 Gemini tools 转为 bamboo.Tool 列表。
func parseTools(tools []geminiTool) []bamboo.Tool {
	result := make([]bamboo.Tool, 0)
	for _, tool := range tools {
		for _, fd := range tool.FunctionDeclarations {
			t := bamboo.Tool{
				Name:        fd.Name,
				Description: fd.Description,
			}
			// parameters 原样保留为 json.RawMessage，确保完整透传所有 JSON Schema 字段
			if len(fd.Parameters) > 0 {
				t.InputSchema = fd.Parameters
			} else {
				// 无参数工具的默认 schema
				t.InputSchema = json.RawMessage(`{"type":"object"}`)
			}
			result = append(result, t)
		}
	}
	return result
}

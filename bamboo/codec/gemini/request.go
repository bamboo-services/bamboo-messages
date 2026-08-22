package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

// ── Gemini 请求 JSON 结构体 ──

// geminiRequest Gemini GenerateContent 请求体结构。
//
// 参考: https://ai.google.dev/api/rest/v1/models/generateContent
type geminiRequest struct {
	Contents          []geminiContent       `json:"contents"`
	SystemInstruction *geminiContent        `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenConfig      `json:"generationConfig,omitempty"`
	Tools             []geminiTool          `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig     `json:"toolConfig,omitempty"`
	SafetySettings    []geminiSafetySetting `json:"safetySettings,omitempty"`
	CachedContent     string                `json:"cachedContent,omitempty"`
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
	Text             string              `json:"text,omitempty"`
	Thought          bool                `json:"thought,omitempty"`
	ThoughtSignature string              `json:"thoughtSignature,omitempty"`
	InlineData       *geminiInlineData   `json:"inlineData,omitempty"`
	FileData         *geminiFileData     `json:"fileData,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFuncResponse `json:"functionResponse,omitempty"`
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
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"topP,omitempty"`
	TopK             *float64           `json:"topK,omitempty"`
	MaxOutputTokens  *int64             `json:"maxOutputTokens,omitempty"`
	StopSequences    []string           `json:"stopSequences,omitempty"`
	ResponseMimeType string             `json:"responseMimeType,omitempty"`
	ThinkingConfig   *geminiThinkingCfg `json:"thinkingConfig,omitempty"`
}

// geminiThinkingCfg Gemini thinking 配置。
type geminiThinkingCfg struct {
	ThinkingBudget  *int64 `json:"thinkingBudget,omitempty"`
	IncludeThoughts *bool  `json:"includeThoughts,omitempty"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
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
		return nil, pkgErrors.NewBambooError("下游", "failed to parse gemini request body", 0)
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
	// 注意: Gemini 的 model 名称在 URL 路径中（如 /v1beta/models/gemini-2.5-pro:generateContent），
	// 不在请求 body 中，因此 config.Model 为空。relay 层需从 URL 路径提取 model。
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

	// ── 6. safetySettings 转换为 []map[string]string 后透传到 ProviderExtra ──
	//
	// codec 层使用通用 map 类型，避免引入 genai SDK 依赖。
	// provider/gemini 适配器通过 GetExtraAny 提取后原样序列化到请求体。
	if len(req.SafetySettings) > 0 {
		if config.ProviderExtra == nil {
			config.ProviderExtra = make(map[string]any)
		}
		config.ProviderExtra["safety_settings"] = convertSafetySettings(req.SafetySettings)
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
		IsStream: false, // Gemini 的流式标识不在 body 中，由 URL 参数 `?alt=sse` 决定。
		// relay 层应根据实际 URL 路径中的 `?alt=sse` 覆盖此值。
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
					trb := bamboo.NewToolResultBlock(toolUseID, contentStr, false).(*bamboo.ToolResultBlock)
					trb.ToolName = part.FunctionResponse.Name
					blocks = append(blocks, trb)
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
		// functionCall 与 thoughtSignature 可同 part。对齐 Anthropic：
		// ThinkingBlock（签名）与 ToolUseBlock 分开进 IR，禁止用 signature 把工具调用吃掉。
		if part.FunctionCall != nil {
			if part.Thought && part.Text != "" {
				blocks = append(blocks, bamboo.NewThinkingBlock(part.Text, part.ThoughtSignature))
			} else if part.ThoughtSignature != "" {
				blocks = append(blocks, bamboo.NewThinkingBlock("", part.ThoughtSignature))
			}
			blocks = append(blocks, newGeminiToolUseBlock(part.FunctionCall, callIndex))
			continue
		}

		if part.Thought || part.ThoughtSignature != "" {
			blocks = append(blocks, bamboo.NewThinkingBlock(part.Text, part.ThoughtSignature))
			continue
		}

		if part.Text != "" {
			blocks = append(blocks, bamboo.NewTextBlock(part.Text))
			continue
		}

		if part.InlineData != nil {
			blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
				Type:      "base64",
				MediaType: part.InlineData.MimeType,
				Data:      part.InlineData.Data,
			}))
			continue
		}

		if part.FileData != nil {
			blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
				Type:      "url",
				URL:       part.FileData.FileURI,
				MediaType: part.FileData.MimeType,
			}))
			continue
		}

		if part.FunctionResponse != nil {
			toolUseID := part.FunctionResponse.ID
			if toolUseID == "" {
				toolUseID = part.FunctionResponse.Name
			}
			contentStr := serializeFuncResponse(part.FunctionResponse.Response)
			trb := bamboo.NewToolResultBlock(toolUseID, contentStr, false).(*bamboo.ToolResultBlock)
			trb.ToolName = part.FunctionResponse.Name
			blocks = append(blocks, trb)
			continue
		}
	}
	return blocks, nil
}

func newGeminiToolUseBlock(call *geminiFunctionCall, callIndex *int) *bamboo.ToolUseBlock {
	id := call.ID
	if id == "" {
		id = fmt.Sprintf("gemini_call_%s_%d", call.Name, *callIndex)
	}
	*callIndex++
	input := call.Args
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	return &bamboo.ToolUseBlock{
		Type:  bamboo.ContentBlockToolUse,
		ID:    id,
		Name:  call.Name,
		Input: input,
	}
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

	// thinkingConfig → ThinkingConfig.Effort（Bamboo 规范化槽）。
	// includeThoughts 另存 ProviderExtra，供 Gemini provider 还原 includeThoughts。
	if gc.ThinkingConfig != nil {
		config.ThinkingConfig = mapGeminiThinkingCfg(gc.ThinkingConfig)
		if gc.ThinkingConfig.IncludeThoughts != nil {
			if config.ProviderExtra == nil {
				config.ProviderExtra = make(map[string]any)
			}
			config.ProviderExtra["include_thoughts"] = *gc.ThinkingConfig.IncludeThoughts
		}
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

// convertSafetySettings 将 geminiSafetySetting 列表转换为 []map[string]string。
//
// codec 层使用通用 map 类型输出，避免引入 genai SDK 依赖。
// provider/gemini 适配器通过 GetExtraAny 提取后原样序列化到请求体。
func convertSafetySettings(settings []geminiSafetySetting) []map[string]string {
	result := make([]map[string]string, 0, len(settings))
	for _, s := range settings {
		result = append(result, map[string]string{
			"category":  s.Category,
			"threshold": s.Threshold,
		})
	}
	return result
}

func mapGeminiThinkingCfg(cfg *geminiThinkingCfg) *bamboo.ThinkingConfig {
	if cfg == nil {
		return nil
	}
	if cfg.ThinkingLevel != "" {
		return &bamboo.ThinkingConfig{Effort: strings.ToLower(cfg.ThinkingLevel)}
	}
	if cfg.ThinkingBudget != nil {
		return mapThinkingBudgetToEffort(cfg.ThinkingBudget)
	}
	if cfg.IncludeThoughts != nil && *cfg.IncludeThoughts {
		return &bamboo.ThinkingConfig{Effort: "medium"}
	}
	return nil
}

// mapThinkingBudgetToEffort 将 Gemini thinkingBudget 映射为 ThinkingConfig。
//
// budget 为 nil 时返回 nil（不启用思考）；
// budget > 0 时根据大小推断 effort 级别。
func mapThinkingBudgetToEffort(budget *int64) *bamboo.ThinkingConfig {
	if budget == nil {
		return nil
	}
	b := *budget
	if b <= 0 {
		return &bamboo.ThinkingConfig{Effort: "none"}
	}
	if b <= 2048 {
		return &bamboo.ThinkingConfig{Effort: "low"}
	}
	if b <= 8192 {
		return &bamboo.ThinkingConfig{Effort: "medium"}
	}
	return &bamboo.ThinkingConfig{Effort: "high"}
}

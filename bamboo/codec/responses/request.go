package responses

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// ── OpenAI Responses 请求 JSON 结构体 ──

// responsesRequest OpenAI Responses API 请求体结构。
//
// 与 Chat Completions 完全不同，Responses 使用 input 数组而非 messages，
// 工具定义直接展开（无 function 外层包装）。
type responsesRequest struct {
	Model              string          `json:"model"`
	Instructions       string          `json:"instructions,omitempty"`
	Input              json.RawMessage `json:"input,omitempty"`
	MaxOutputTokens    *int64          `json:"max_output_tokens,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	TopP               *float64        `json:"top_p,omitempty"`
	Stream             bool            `json:"stream,omitempty"`
	Tools              []responsesTool `json:"tools,omitempty"`
	ToolChoice         json.RawMessage `json:"tool_choice,omitempty"`
	Reasoning          *reasoningCfg   `json:"reasoning,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Store              *bool           `json:"store,omitempty"`
	Truncation         string          `json:"truncation,omitempty"`
	User               string          `json:"user,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	Text               *textConfig     `json:"text,omitempty"`
}

// reasoningCfg Reasoning 配置。
type reasoningCfg struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// textConfig 文本输出配置（含 format）。
type textConfig struct {
	Format *textFormat `json:"format,omitempty"`
}

// textFormat 文本格式配置。
type textFormat struct {
	Type string `json:"type,omitempty"`
}

// responsesTool Responses 格式的工具定义（扁平结构，无 function 外层包装）。
type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// inputItem input 数组元素，通过 type 字段区分不同类型。
type inputItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	// reasoning 专用
	Summary []outputReasoningSummary `json:"summary,omitempty"`
	// function_call 专用
	CallID    string `json:"call_id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// function_call_output 专用
	Output string `json:"output,omitempty"`
}

// inputContent input message 的 content 元素。
type inputContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"` // input_image 专用
	FileID   string `json:"file_id,omitempty"`   // input_file 专用（file ID 引用）
	FileData string `json:"file_data,omitempty"` // input_file 专用（base64 数据）
}

// parseRequest 将 OpenAI Responses 请求体解析为 RelayRequest。
func parseRequest(body []byte) (*codec.RelayRequest, error) {
	var req responsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, codec.NewErrorWithCause(codec.ErrInvalidRequest, "failed to parse request body", err)
	}

	var systemParts []string
	if req.Instructions != "" {
		systemParts = append(systemParts, req.Instructions)
	}

	var messages []bamboo.BambooMessage

	// 解析 input 字段（string 或 array）
	if len(req.Input) > 0 {
		bm, sys, err := parseInput(req.Input)
		if err != nil {
			return nil, err
		}
		messages = bm
		if sys != "" {
			systemParts = append(systemParts, sys)
		}
	}

	// 构建配置
	config := &bamboo.RequestConfig{
		Model: req.Model,
	}

	if req.MaxOutputTokens != nil {
		config.MaxTokens = *req.MaxOutputTokens
	}
	if req.Temperature != nil {
		config.Temperature = req.Temperature
	}
	if req.TopP != nil {
		config.TopP = req.TopP
	}

	// 工具定义
	if len(req.Tools) > 0 {
		config.Tools = parseResponsesTools(req.Tools)
	}

	// tool_choice
	if choice, err := parseToolChoice(req.ToolChoice); err == nil && choice != "" {
		config.ToolChoice = choice
	}

	// text.format.type → ResponseFormat
	if req.Text != nil && req.Text.Format != nil && req.Text.Format.Type != "" {
		config.ResponseFormat = req.Text.Format.Type
	}

	// reasoning.effort → ThinkingConfig
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		config.ThinkingConfig = &bamboo.ThinkingConfig{
			Effort: req.Reasoning.Effort,
		}
	}

	if req.User != "" {
		config.UserID = req.User
	}

	// ProviderExtra：previous_response_id / store / truncation
	extra := make(map[string]any)
	if req.PreviousResponseID != "" {
		extra["previous_response_id"] = req.PreviousResponseID
	}
	if req.Store != nil {
		extra["store"] = *req.Store
	}
	if req.Truncation != "" {
		extra["truncation"] = req.Truncation
	}
	if len(req.Metadata) > 0 {
		var metaMap map[string]any
		if err := json.Unmarshal(req.Metadata, &metaMap); err == nil {
			// 检查是否所有值都是 string，如果是则存入 config.Metadata
			allStrings := true
			stringMeta := make(map[string]string, len(metaMap))
			for k, v := range metaMap {
				if s, ok := v.(string); ok {
					stringMeta[k] = s
				} else {
					allStrings = false
					break
				}
			}
			if allStrings && len(stringMeta) > 0 {
				config.Metadata = stringMeta
			} else {
				// 混合类型回退到 ProviderExtra
				extra["metadata"] = metaMap
			}
		}
	}
	if len(extra) > 0 {
		config.ProviderExtra = extra
	}

	return &codec.RelayRequest{
		Messages: messages,
		System:   strings.Join(systemParts, "\n\n"),
		Config:   config,
		IsStream: req.Stream,
	}, nil
}

// parseInput 解析 input 字段（string 或 array）。
//
// 返回消息列表、从 message 中提取的 system 内容、以及可能的错误。
func parseInput(raw json.RawMessage) ([]bamboo.BambooMessage, string, error) {
	if len(raw) == 0 {
		return nil, "", nil
	}

	// 尝试 string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []bamboo.BambooMessage{bamboo.NewUserMessage(s)}, "", nil
	}

	// 尝试 array
	var items []inputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, "", codec.NewErrorWithCause(codec.ErrInvalidRequest, "failed to parse input field", err)
	}

	var messages []bamboo.BambooMessage
	var systemParts []string

	for _, item := range items {
		switch item.Type {
		case "message":
			msg, sys := parseInputMessage(item)
			if sys != "" {
				systemParts = append(systemParts, sys)
			} else if len(msg.Content) > 0 {
				messages = append(messages, msg)
			}
		case "function_call":
			// assistant 工具调用
			var input json.RawMessage
			if item.Arguments != "" {
				input = json.RawMessage(item.Arguments)
			} else {
				input = json.RawMessage(`{}`)
			}
			messages = append(messages, bamboo.NewAssistantMessageBlocks(&bamboo.ToolUseBlock{
				Type:  bamboo.ContentBlockToolUse,
				ID:    item.CallID,
				Name:  item.Name,
				Input: input,
			}))
		case "function_call_output":
			// 用户侧工具结果
			messages = append(messages, bamboo.NewUserMessageBlocks(
				bamboo.NewToolResultBlock(item.CallID, item.Output, false),
			))
		case "reasoning":
			text := extractSummaryText(item.Summary)
			if text == "" {
				text = extractReasoningText(item.Content)
			}
			if text != "" {
				messages = append(messages, bamboo.NewAssistantMessageBlocks(
					bamboo.NewThinkingBlock(text, ""),
				))
			}
		default:
			log.Printf("[codec/responses] unknown input item type %q, skipped", item.Type)
		}
	}

	return messages, strings.Join(systemParts, "\n\n"), nil
}

// parseInputMessage 解析 type=message 的 input 元素。
//
// 返回消息和可选的 system 文本（当 role 为 system 时）。
func parseInputMessage(item inputItem) (bamboo.BambooMessage, string) {
	if len(item.Content) == 0 {
		return bamboo.BambooMessage{}, ""
	}

	var parts []inputContent
	if err := json.Unmarshal(item.Content, &parts); err != nil {
		// content 可能是纯字符串
		var s string
		if err2 := json.Unmarshal(item.Content, &s); err2 == nil {
			parts = []inputContent{{Type: "input_text", Text: s}}
		} else {
			return bamboo.BambooMessage{}, ""
		}
	}

	switch item.Role {
	case "system":
		var texts []string
		for _, p := range parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return bamboo.BambooMessage{}, strings.Join(texts, "\n")

	case "assistant":
		var blocks []bamboo.ContentBlock
		for _, p := range parts {
			switch p.Type {
			case "input_image":
				if p.ImageURL != "" {
					blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
						Type: "url",
						URL:  p.ImageURL,
					}))
				}
			case "input_file":
				if p.FileID != "" {
					blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
						Type: "url",
						URL:  p.FileID,
					}))
				} else if p.FileData != "" {
					blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
						Type: "base64",
						Data: p.FileData,
					}))
				}
			default:
				if p.Text != "" {
					blocks = append(blocks, bamboo.NewTextBlock(p.Text))
				}
			}
		}
		if len(blocks) == 0 {
			return bamboo.BambooMessage{Role: bamboo.RoleAssistant}, ""
		}
		return bamboo.NewAssistantMessageBlocks(blocks...), ""

	default: // user 或未知角色都按 user 处理
		var blocks []bamboo.ContentBlock
		for _, p := range parts {
			switch p.Type {
			case "input_image":
				if p.ImageURL != "" {
					blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
						Type: "url",
						URL:  p.ImageURL,
					}))
				}
			case "input_file":
				if p.FileID != "" {
					blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
						Type: "url",
						URL:  p.FileID,
					}))
				} else if p.FileData != "" {
					blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
						Type: "base64",
						Data: p.FileData,
					}))
				}
			default:
				if p.Text != "" {
					blocks = append(blocks, bamboo.NewTextBlock(p.Text))
				}
			}
		}
		if len(blocks) == 0 {
			return bamboo.BambooMessage{Role: bamboo.RoleUser}, ""
		}
		return bamboo.NewUserMessageBlocks(blocks...), ""
	}
}

// extractReasoningText 从 reasoning item 的 content 中提取推理文本。
func extractReasoningText(contentRaw json.RawMessage) string {
	if len(contentRaw) == 0 {
		return ""
	}
	var parts []inputContent
	if err := json.Unmarshal(contentRaw, &parts); err != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// extractSummaryText 从 reasoning item 的 summary 数组中提取摘要文本。
func extractSummaryText(summary []outputReasoningSummary) string {
	var texts []string
	for _, s := range summary {
		if s.Text != "" {
			texts = append(texts, s.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// parseResponsesTools 将 Responses 格式的 tools 转为 bamboo.Tool。
//
// Responses 的工具定义是扁平结构：{type:"function", name, description, parameters}
func parseResponsesTools(tools []responsesTool) []bamboo.Tool {
	result := make([]bamboo.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		tool := bamboo.Tool{
			Name:        t.Name,
			Description: t.Description,
		}
		// parameters 原样保留为 json.RawMessage，确保完整透传所有 JSON Schema 字段
		tool.InputSchema = t.Parameters
		result = append(result, tool)
	}
	return result
}

// parseToolChoice 解析 tool_choice 字段（string 或 object）。
func parseToolChoice(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	// 尝试 string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto", "none", "required":
			return s, nil
		default:
			return "", nil
		}
	}
	// 尝试 object: {type:"function", name:"xxx"}
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Type == "function" {
		return "forced", nil
	}
	return "", nil
}

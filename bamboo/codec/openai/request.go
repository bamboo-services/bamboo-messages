package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// ── OpenAI 请求 JSON 结构体 ──

type openaiRequest struct {
	Model             string          `json:"model"`
	Messages          []openaiMessage `json:"messages"`
	Stream            bool            `json:"stream,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	MaxTokens         *int64          `json:"max_tokens,omitempty"`
	MaxCompTokens     *int64          `json:"max_completion_tokens,omitempty"`
	Stop              json.RawMessage `json:"stop,omitempty"`
	Tools             []openaiTool    `json:"tools,omitempty"`
	ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`
	ResponseFormat    *openaiRespFmt  `json:"response_format,omitempty"`
	ParallelToolCalls bool            `json:"parallel_tool_calls,omitempty"`
	ReasoningEffort   string          `json:"reasoning_effort,omitempty"`
	User              string          `json:"user,omitempty"`
	FrequencyPenalty  *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64        `json:"presence_penalty,omitempty"`
	Seed              *int64          `json:"seed,omitempty"`
	PromptCacheKey    string          `json:"prompt_cache_key,omitempty"`
}

type openaiMessage struct {
	Role             string           `json:"role"`
	Content          json.RawMessage  `json:"content,omitempty"`
	ReasoningContent json.RawMessage  `json:"reasoning_content,omitempty"`
	Reasoning        json.RawMessage  `json:"reasoning,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type     string           `json:"type"`
	Function openaiToolSchema `json:"function"`
}

type openaiToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openaiRespFmt struct {
	Type string `json:"type"`
}

// content array 元素
type openaiContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openaiImageURL `json:"image_url,omitempty"`
}

type openaiImageURL struct {
	URL string `json:"url"`
}

// parseRequest 将 OpenAI Chat Completions 请求体解析为 RelayRequest。
func parseRequest(body []byte) (*codec.RelayRequest, error) {
	var req openaiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, codec.NewErrorWithCause(codec.ErrInvalidRequest, "failed to parse request body", err)
	}

	// 解析消息
	var systemParts []string
	var messages []bamboo.BambooMessage

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			text := extractPlainText(msg.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}
		case "user":
			bm, err := parseUserMessage(msg)
			if err != nil {
				return nil, err
			}
			messages = append(messages, bm)
		case "assistant":
			bm := parseAssistantMessage(msg)
			messages = append(messages, bm)
		case "tool":
			messages = append(messages, parseToolMessage(msg))
		default:
			// 未知角色按 user 处理
			bm, _ := parseUserMessage(msg)
			messages = append(messages, bm)
		}
	}

	// 构建配置
	config := &bamboo.RequestConfig{
		Model: req.Model,
	}

	// max_tokens 优先使用 max_completion_tokens
	if req.MaxCompTokens != nil {
		config.MaxTokens = *req.MaxCompTokens
	} else if req.MaxTokens != nil {
		config.MaxTokens = *req.MaxTokens
	}

	if req.Temperature != nil {
		config.Temperature = req.Temperature
	}
	if req.TopP != nil {
		config.TopP = req.TopP
	}

	// stop 序列
	if stops, err := parseStop(req.Stop); err == nil && len(stops) > 0 {
		config.StopSequences = stops
	}

	// 工具定义
	if len(req.Tools) > 0 {
		config.Tools = parseTools(req.Tools)
	}

	// tool_choice
	if choice, err := parseToolChoice(req.ToolChoice); err == nil && choice != "" {
		config.ToolChoice = choice
	}

	// response_format
	if req.ResponseFormat != nil && req.ResponseFormat.Type != "" {
		config.ResponseFormat = req.ResponseFormat.Type
	}

	config.ParallelToolCalls = req.ParallelToolCalls

	// reasoning_effort → ThinkingConfig
	if req.ReasoningEffort != "" {
		config.ThinkingConfig = &bamboo.ThinkingConfig{
			Effort: req.ReasoningEffort,
		}
	}

	if req.User != "" {
		config.UserID = req.User
	}

	if req.PromptCacheKey != "" {
		config.PromptCacheKey = req.PromptCacheKey
	}

	// ProviderExtra
	if req.FrequencyPenalty != nil || req.PresencePenalty != nil || req.Seed != nil {
		extra := make(map[string]any)
		if req.FrequencyPenalty != nil {
			extra["frequency_penalty"] = *req.FrequencyPenalty
		}
		if req.PresencePenalty != nil {
			extra["presence_penalty"] = *req.PresencePenalty
		}
		if req.Seed != nil {
			extra["seed"] = *req.Seed
		}
		config.ProviderExtra = extra
	}

	return &codec.RelayRequest{
		Messages: messages,
		System:   strings.Join(systemParts, "\n\n"),
		Config:   config,
		IsStream: req.Stream,
	}, nil
}

// extractPlainText 从 content（string 或 array）中提取纯文本。
func extractPlainText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// 尝试 string
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	// 尝试 array
	var parts []openaiContentPart
	if err := json.Unmarshal(content, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// parseUserMessage 解析 user 角色消息。
func parseUserMessage(msg openaiMessage) (bamboo.BambooMessage, error) {
	if len(msg.Content) == 0 {
		return bamboo.BambooMessage{Role: bamboo.RoleUser}, nil
	}
	// 尝试 string
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return bamboo.NewUserMessage(s), nil
	}
	// 尝试 array
	var parts []openaiContentPart
	if err := json.Unmarshal(msg.Content, &parts); err != nil {
		return bamboo.BambooMessage{}, codec.NewErrorWithCause(codec.ErrInvalidRequest, "failed to parse user message content", err)
	}

	blocks := make([]bamboo.ContentBlock, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			blocks = append(blocks, bamboo.NewTextBlock(p.Text))
		case "image_url":
			if p.ImageURL != nil {
				blocks = append(blocks, parseImageURL(p.ImageURL.URL))
			}
		}
	}
	return bamboo.NewUserMessageBlocks(blocks...), nil
}

// parseImageURL 将 OpenAI image_url 转为 bamboo.ImageBlock。
func parseImageURL(url string) bamboo.ContentBlock {
	// data URI: data:image/png;base64,xxxx
	if strings.HasPrefix(url, "data:") {
		commaIdx := strings.Index(url, ",")
		if commaIdx > 0 {
			header := url[:commaIdx]
			data := url[commaIdx+1:]
			mediaType := parseDataURIMediaType(header)
			return bamboo.NewImageBlock(bamboo.ContentSource{
				Type:      "base64",
				MediaType: mediaType,
				Data:      data,
			})
		}
	}
	// 普通 URL
	return bamboo.NewImageBlock(bamboo.ContentSource{
		Type: "url",
		URL:  url,
	})
}

// parseDataURIMediaType 从 data URI header 提取 MIME 类型。
// header 格式: "data:image/png;base64"
func parseDataURIMediaType(header string) string {
	// 去掉 "data:" 前缀
	s := strings.TrimPrefix(header, "data:")
	// 去掉 ";base64" 后缀
	if idx := strings.Index(s, ";"); idx > 0 {
		return s[:idx]
	}
	return s
}

// parseAssistantMessage 解析 assistant 角色消息。
func parseAssistantMessage(msg openaiMessage) bamboo.BambooMessage {
	var blocks []bamboo.ContentBlock

	if reasoning := extractReasoningContent(msg.ReasoningContent, msg.Reasoning); reasoning != "" {
		blocks = append(blocks, bamboo.NewThinkingBlock(reasoning, ""))
	}

	if text := extractPlainText(msg.Content); text != "" {
		blocks = append(blocks, bamboo.NewTextBlock(text))
	}

	for _, tc := range msg.ToolCalls {
		var input json.RawMessage
		if tc.Function.Arguments != "" {
			input = json.RawMessage(tc.Function.Arguments)
		} else {
			input = json.RawMessage(`{}`)
		}
		blocks = append(blocks, &bamboo.ToolUseBlock{
			Type:  bamboo.ContentBlockToolUse,
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	if len(blocks) == 0 {
		return bamboo.BambooMessage{Role: bamboo.RoleAssistant}
	}
	return bamboo.NewAssistantMessageBlocks(blocks...)
}

func extractReasoningContent(sources ...json.RawMessage) string {
	for _, raw := range sources {
		if len(raw) == 0 {
			continue
		}
		s := strings.TrimSpace(string(raw))
		if s == "" || s == "null" {
			continue
		}
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			return str
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err == nil {
			if text, ok := obj["text"].(string); ok && text != "" {
				return text
			}
			if content, ok := obj["content"].(string); ok && content != "" {
				return content
			}
		}
		return s
	}
	return ""
}

// parseToolMessage 解析 tool 角色消息 → user + ToolResultBlock。
func parseToolMessage(msg openaiMessage) bamboo.BambooMessage {
	content := extractPlainText(msg.Content)
	block := bamboo.NewToolResultBlock(msg.ToolCallID, content, false)
	return bamboo.NewUserMessageBlocks(block)
}

// parseStop 解析 stop 字段（string 或 []string）。
func parseStop(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	// 尝试 string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []string{s}, nil
	}
	// 尝试 []string
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("invalid stop format")
}

// parseToolChoice 解析 tool_choice 字段。
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
	// 尝试 object: {type:"function", function:{name:"xxx"}}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Type == "function" {
		return "forced", nil
	}
	return "", nil
}

// parseTools 将 OpenAI tools 转为 bamboo.Tool。
func parseTools(tools []openaiTool) []bamboo.Tool {
	result := make([]bamboo.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		tool := bamboo.Tool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
		}
		// parameters 原样保留为 json.RawMessage，确保完整透传所有 JSON Schema 字段
		tool.InputSchema = t.Function.Parameters
		result = append(result, tool)
	}
	return result
}

// 编译时确保 base64 包被使用（data URI 场景中解码验证用）
var _ = base64.StdEncoding

package anthropic

import (
	"encoding/json"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ── Anthropic Messages 请求 JSON 结构体 ──

// anthropicRequest Anthropic Messages API 请求体结构。
type anthropicRequest struct {
	Model         string             `json:"model"`
	Messages      []anthropicMessage `json:"messages"`
	System        json.RawMessage    `json:"system,omitempty"`     // string 或 [{type:"text",text}]
	MaxTokens     *int64             `json:"max_tokens,omitempty"` // 缺省给 4096
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	TopK          *int64             `json:"top_k,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Tools         []anthropicTool    `json:"tools,omitempty"`
	ToolChoice    json.RawMessage    `json:"tool_choice,omitempty"`
	Metadata      *anthropicMetadata `json:"metadata,omitempty"`
	Thinking      json.RawMessage    `json:"thinking,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content json.RawMessage    `json:"content,omitempty"` // string 或 []ContentBlock
}

type anthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type anthropicTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

// ── content block 原始 JSON 结构 ──

type rawContentBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	Source       *rawSource      `json:"source,omitempty"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      json.RawMessage `json:"content,omitempty"` // string 或 []{type,text}
	IsError      bool            `json:"is_error,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`
	Signature    string          `json:"signature,omitempty"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

type rawSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// parseRequest 将 Anthropic Messages 请求体解析为 RelayRequest。
func parseRequest(body []byte) (*codec.RelayRequest, error) {
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, codec.NewErrorWithCause(codec.ErrInvalidRequest, "failed to parse request body", err)
	}

	// 解析 system 提示词
	system := parseSystem(req.System)

	// 解析 system 上的 cache_control（放在 system 数组最后一个 block 上）
	systemCacheControl := parseSystemCacheControl(req.System)

	// 解析消息列表
	messages := make([]bamboo.BambooMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		bm, err := parseMessage(msg)
		if err != nil {
			return nil, err
		}
		messages = append(messages, bm)
	}

	// 构建配置
	config := &bamboo.RequestConfig{
		Model: req.Model,
	}

	if systemCacheControl != nil {
		config.SystemCacheControl = systemCacheControl
	}

	// max_tokens 缺省给 4096
	if req.MaxTokens != nil {
		config.MaxTokens = *req.MaxTokens
	} else {
		config.MaxTokens = 4096
	}

	if req.Temperature != nil {
		config.Temperature = req.Temperature
	}
	if req.TopP != nil {
		config.TopP = req.TopP
	}

	if len(req.StopSequences) > 0 {
		config.StopSequences = req.StopSequences
	}

	// 工具定义
	if len(req.Tools) > 0 {
		config.Tools = parseTools(req.Tools)
	}

	// tool_choice
	if choice := parseToolChoice(req.ToolChoice); choice != "" {
		config.ToolChoice = choice
	}

	// thinking
	if tc := parseThinking(req.Thinking); tc != nil {
		config.ThinkingConfig = tc
	}

	// metadata.user_id
	if req.Metadata != nil && req.Metadata.UserID != "" {
		config.UserID = req.Metadata.UserID
	}

	// top_k → ProviderExtra
	if req.TopK != nil {
		if config.ProviderExtra == nil {
			config.ProviderExtra = make(map[string]any)
		}
		config.ProviderExtra["top_k"] = *req.TopK
	}

	return &codec.RelayRequest{
		Messages: messages,
		System:   system,
		Config:   config,
		IsStream: req.Stream,
	}, nil
}

// parseSystem 解析 system 字段（string 或 [{type:"text",text}]）。
func parseSystem(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试 string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// 尝试 array
	var parts []rawContentBlock
	if err := json.Unmarshal(raw, &parts); err == nil {
		texts := make([]string, 0, len(parts))
		for _, p := range parts {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n\n")
	}
	return ""
}

// parseSystemCacheControl 从 system 数组中提取最后一个 text block 的 cache_control。
func parseSystemCacheControl(raw json.RawMessage) *provider.CacheControl {
	if len(raw) == 0 {
		return nil
	}
	var parts []rawContentBlock
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i].Type == "text" && len(parts[i].CacheControl) > 0 {
			return parseCacheControlRaw(parts[i].CacheControl)
		}
	}
	return nil
}

// parseCacheControlRaw 将 Anthropic cache_control JSON 解析为 provider.CacheControl。
func parseCacheControlRaw(raw json.RawMessage) *provider.CacheControl {
	if len(raw) == 0 {
		return nil
	}
	var obj struct {
		Type string `json:"type"`
		TTL  string `json:"ttl,omitempty"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	if obj.Type == "" {
		return nil
	}
	cc := &provider.CacheControl{Type: obj.Type}
	if obj.TTL != "" {
		cc.TTL = provider.CacheControlEphemeralTTL(obj.TTL)
	}
	return cc
}

// parseMessage 解析单条消息，将 Anthropic content 格式转换为 bamboo.BambooMessage。
func parseMessage(msg anthropicMessage) (bamboo.BambooMessage, error) {
	role := bamboo.RoleUser
	if msg.Role == "assistant" {
		role = bamboo.RoleAssistant
	}

	// content 缺失或空 → 返回空消息
	if len(msg.Content) == 0 {
		return bamboo.BambooMessage{Role: role}, nil
	}

	// 尝试 string
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		if role == bamboo.RoleUser {
			return bamboo.NewUserMessage(s), nil
		}
		return bamboo.NewAssistantMessage(s), nil
	}

	// 解析为 content block 数组
	var rawBlocks []rawContentBlock
	if err := json.Unmarshal(msg.Content, &rawBlocks); err != nil {
		return bamboo.BambooMessage{}, codec.NewErrorWithCause(codec.ErrInvalidRequest, "failed to parse message content", err)
	}

	blocks := make([]bamboo.ContentBlock, 0, len(rawBlocks))
	for _, rb := range rawBlocks {
		block := convertContentBlock(rb)
		if block != nil {
			blocks = append(blocks, block)
		}
	}

	if role == bamboo.RoleUser {
		return bamboo.NewUserMessageBlocks(blocks...), nil
	}
	return bamboo.NewAssistantMessageBlocks(blocks...), nil
}

// convertContentBlock 将 Anthropic content block JSON 转为 bamboo.ContentBlock。
func convertContentBlock(rb rawContentBlock) bamboo.ContentBlock {
	cc := parseCacheControlRaw(rb.CacheControl)
	switch rb.Type {
	case "text":
		return &bamboo.TextBlock{
			Type:         bamboo.ContentBlockText,
			Text:         rb.Text,
			CacheControl: cc,
		}

	case "image":
		if rb.Source == nil {
			return nil
		}
		source := bamboo.ContentSource{
			Type:      rb.Source.Type,
			MediaType: rb.Source.MediaType,
			Data:      rb.Source.Data,
			URL:       rb.Source.URL,
		}
		return &bamboo.ImageBlock{
			Type:         bamboo.ContentBlockImage,
			Source:       &source,
			CacheControl: cc,
		}

	case "tool_use":
		input := rb.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		return &bamboo.ToolUseBlock{
			Type:         bamboo.ContentBlockToolUse,
			ID:           rb.ID,
			Name:         rb.Name,
			Input:        input,
			CacheControl: cc,
		}

	case "tool_result":
		content := extractToolResultContent(rb.Content)
		return &bamboo.ToolResultBlock{
			Type:         bamboo.ContentBlockToolResult,
			ToolUseID:    rb.ToolUseID,
			Content:      content,
			IsError:      rb.IsError,
			CacheControl: cc,
		}

	case "thinking":
		return &bamboo.ThinkingBlock{
			Type:         bamboo.ContentBlockThinking,
			Thinking:     rb.Thinking,
			Signature:    rb.Signature,
			CacheControl: cc,
		}
	}
	return nil
}

// extractToolResultContent 从 tool_result 的 content 字段提取文本。
// content 可为 string 或 [{type:"text",text}]。
func extractToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试 string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// 尝试 array
	var parts []rawContentBlock
	if err := json.Unmarshal(raw, &parts); err == nil {
		texts := make([]string, 0, len(parts))
		for _, p := range parts {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// parseTools 将 Anthropic tools 转为 bamboo.Tool 列表。
func parseTools(tools []anthropicTool) []bamboo.Tool {
	result := make([]bamboo.Tool, 0, len(tools))
	for _, t := range tools {
		tool := bamboo.Tool{
			Name:         t.Name,
			Description:  t.Description,
			CacheControl: parseCacheControlRaw(t.CacheControl),
		}
		tool.InputSchema = t.InputSchema
		result = append(result, tool)
	}
	return result
}

// parseToolChoice 解析 tool_choice 字段。
//
// 映射规则:
//   - {type:"auto"}  → "auto"
//   - {type:"any"}   → "required"
//   - {type:"none"}  → "none"
//   - {type:"tool", name:"xxx"} → "forced"
func parseToolChoice(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	switch obj.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		return "forced"
	}
	return ""
}

// parseThinking 解析 thinking 字段。
//
// 映射规则:
//   - {type:"adaptive"}             → ThinkingConfig{Effort:"high"}
//   - {type:"enabled", budget_tokens:N} → ThinkingConfig{Effort:"medium"}
func parseThinking(raw json.RawMessage) *bamboo.ThinkingConfig {
	if len(raw) == 0 {
		return nil
	}
	var obj struct {
		Type         string `json:"type"`
		BudgetTokens *int64 `json:"budget_tokens,omitempty"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	switch obj.Type {
	case "adaptive":
		return &bamboo.ThinkingConfig{Effort: "high"}
	case "enabled":
		return &bamboo.ThinkingConfig{Effort: "medium"}
	}
	return nil
}

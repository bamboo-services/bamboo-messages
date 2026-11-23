package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ── OpenAI 请求 JSON 结构体 ──

type openaiRequest struct {
	Model                string                    `json:"model"`
	Messages             []openaiMessage           `json:"messages"`
	Stream               bool                      `json:"stream,omitempty"`
	Temperature          *float64                  `json:"temperature,omitempty"`
	TopP                 *float64                  `json:"top_p,omitempty"`
	MaxTokens            *int64                    `json:"max_tokens,omitempty"`
	MaxCompTokens        *int64                    `json:"max_completion_tokens,omitempty"`
	Stop                 json.RawMessage           `json:"stop,omitempty"`
	Tools                []openaiTool              `json:"tools,omitempty"`
	ToolChoice           json.RawMessage           `json:"tool_choice,omitempty"`
	ResponseFormat       *openaiRespFmt            `json:"response_format,omitempty"`
	ParallelToolCalls    bool                      `json:"parallel_tool_calls,omitempty"`
	ReasoningEffort      string                    `json:"reasoning_effort,omitempty"`
	User                 string                    `json:"user,omitempty"`
	FrequencyPenalty     *float64                  `json:"frequency_penalty,omitempty"`
	PresencePenalty      *float64                  `json:"presence_penalty,omitempty"`
	Seed                 *int64                    `json:"seed,omitempty"`
	PromptCacheKey       string                    `json:"prompt_cache_key,omitempty"`
	PromptCacheOptions   *openaiPromptCacheOptions `json:"prompt_cache_options,omitempty"`
	PromptCacheRetention string                    `json:"prompt_cache_retention,omitempty"`
	CachedContent        string                    `json:"cached_content,omitempty"`
	CachedContentCamel   string                    `json:"cachedContent,omitempty"`
	CacheControl         json.RawMessage           `json:"cache_control,omitempty"`
	ExtraBody            *openaiExtraBody          `json:"extra_body,omitempty"`
	ServiceTier          string                    `json:"service_tier,omitempty"`
}

type openaiExtraBody struct {
	CachedContent      string `json:"cached_content,omitempty"`
	CachedContentCamel string `json:"cachedContent,omitempty"`
	Google             *struct {
		CachedContent      string `json:"cached_content,omitempty"`
		CachedContentCamel string `json:"cachedContent,omitempty"`
	} `json:"google,omitempty"`
}

type openaiMessage struct {
	Role              string           `json:"role"`
	Content           json.RawMessage  `json:"content,omitempty"`
	CacheControl      json.RawMessage  `json:"cache_control,omitempty"`
	ReasoningContent  json.RawMessage  `json:"reasoning_content,omitempty"`
	Reasoning         json.RawMessage  `json:"reasoning,omitempty"`
	ThinkingSignature string           `json:"thinking_signature,omitempty"`
	ThinkingProvider  string           `json:"thinking_provider,omitempty"`
	ReasoningID       string           `json:"reasoning_id,omitempty"`
	ToolCalls         []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID        string           `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Function     openaiToolFunction `json:"function"`
	ExtraContent *openaiToolExtra   `json:"extra_content,omitempty"`
}

type openaiToolExtra struct {
	Google *struct {
		ThoughtSignature string `json:"thought_signature,omitempty"`
	} `json:"google,omitempty"`
}

type openaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type         string           `json:"type"`
	Function     openaiToolSchema `json:"function"`
	CacheControl json.RawMessage  `json:"cache_control,omitempty"`
}

type openaiToolSchema struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Parameters   json.RawMessage `json:"parameters,omitempty"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

type openaiRespFmt struct {
	Type string `json:"type"`
}

// content array 元素
type openaiContentPart struct {
	Type                  string                 `json:"type"`
	Text                  string                 `json:"text,omitempty"`
	ImageURL              *openaiImageURL        `json:"image_url,omitempty"`
	PromptCacheBreakpoint *openaiCacheBreakpoint `json:"prompt_cache_breakpoint,omitempty"`
	CacheControl          json.RawMessage        `json:"cache_control,omitempty"`
}

type openaiImageURL struct {
	URL string `json:"url"`
}

// openaiPromptCacheOptions OpenAI 请求级缓存选项（gpt-5.6+）。
type openaiPromptCacheOptions struct {
	Mode string `json:"mode,omitempty"` // implicit / explicit
	TTL  string `json:"ttl,omitempty"`  // 30m / 12h
}

// openaiCacheBreakpoint OpenAI 块级缓存断点。
type openaiCacheBreakpoint struct {
	Mode string `json:"mode,omitempty"` // 固定 explicit
	TTL  string `json:"ttl,omitempty"`  // 30m / 12h
}

// parseCacheBreakpoint 将 OpenAI 块级缓存断点转换为统一的 provider.CacheControl。
//
// OpenAI 的 prompt_cache_breakpoint 与 Anthropic 的块级 cache_control 语义一致：
// 都标记「可复用 prompt 前缀的精确末尾」。type 统一为 "ephemeral"
// （OpenAI 侧对应 prompt_cache_breakpoint.mode = "explicit"）。
//
// TTL 只保留 OpenAI 支持的取值（30m / 12h）；其他取值丢弃并退化为请求级 TTL。
func parseCacheBreakpoint(bp *openaiCacheBreakpoint) *provider.CacheControl {
	if bp == nil {
		return nil
	}
	cc := &provider.CacheControl{Type: "ephemeral"}
	switch bp.TTL {
	case string(provider.CacheTTL30m):
		cc.TTL = provider.CacheTTL30m
	case string(provider.CacheTTL12h):
		cc.TTL = provider.CacheTTL12h
	}
	return cc
}

// parseCacheControlRaw 将 Anthropic / 兼容客户端的 cache_control JSON 解析为统一的 provider.CacheControl。
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
		cc.TTL = provider.CacheTTL(obj.TTL)
	}
	return cc
}

// resolvePartCacheControl 统一解析内容块上的缓存标记（优先 OpenAI prompt_cache_breakpoint，其次兼容 cache_control）。
func resolvePartCacheControl(bp *openaiCacheBreakpoint, rawCC json.RawMessage) *provider.CacheControl {
	if bp != nil {
		return parseCacheBreakpoint(bp)
	}
	return parseCacheControlRaw(rawCC)
}

// extractPartCacheControl 从消息内容数组末位提取缓存标记，未命中时回退到消息级 cache_control。
func extractPartCacheControl(raw, msgCC json.RawMessage, textOnly bool) *provider.CacheControl {
	if len(raw) > 0 {
		var parts []openaiContentPart
		if err := json.Unmarshal(raw, &parts); err == nil {
			for i := len(parts) - 1; i >= 0; i-- {
				if textOnly && parts[i].Type != "text" {
					continue
				}
				if cc := resolvePartCacheControl(parts[i].PromptCacheBreakpoint, parts[i].CacheControl); cc != nil {
					return cc
				}
			}
		}
	}
	return parseCacheControlRaw(msgCC)
}

func setExtra(config *bamboo.RequestConfig, key string, value any) {
	if config.ProviderExtra == nil {
		config.ProviderExtra = make(map[string]any)
	}
	config.ProviderExtra[key] = value
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveCachedContent 提取 OpenAI 请求中携带的 Google 显式缓存资源引用（cached_content / cachedContent）。
func resolveCachedContent(req openaiRequest) string {
	var extraCC, extraCamel, googleCC, googleCamel string
	if req.ExtraBody != nil {
		extraCC = req.ExtraBody.CachedContent
		extraCamel = req.ExtraBody.CachedContentCamel
		if req.ExtraBody.Google != nil {
			googleCC = req.ExtraBody.Google.CachedContent
			googleCamel = req.ExtraBody.Google.CachedContentCamel
		}
	}
	return firstNonEmpty(
		req.CachedContent,
		req.CachedContentCamel,
		extraCC,
		extraCamel,
		googleCC,
		googleCamel,
	)
}

// parsePromptCacheOptions 将 OpenAI 请求级缓存选项转换为统一的 provider.PromptCacheOptions。
//
// prompt_cache_options 与 prompt_cache_retention 任意一个存在即视为调用方已
// 显式启用 OpenAI 的 gpt-5.6+ 缓存 API，因此同时开启块级断点透传，
// 保证入站请求里的 prompt_cache_breakpoint 能在出站协议中保留。
//
// 返回 nil 表示请求未携带任何请求级缓存选项。
func parsePromptCacheOptions(opts *openaiPromptCacheOptions, retention string) *provider.PromptCacheOptions {
	if opts == nil && retention == "" {
		return nil
	}
	result := &provider.PromptCacheOptions{EnableBlockBreakpoints: true}
	if opts != nil {
		switch opts.Mode {
		case string(provider.PromptCacheModeImplicit):
			result.Mode = provider.PromptCacheModeImplicit
		case string(provider.PromptCacheModeExplicit):
			result.Mode = provider.PromptCacheModeExplicit
		}
		switch opts.TTL {
		case string(provider.CacheTTL30m):
			result.TTL = provider.CacheTTL30m
		case string(provider.CacheTTL12h):
			result.TTL = provider.CacheTTL12h
		}
	}
	switch retention {
	case string(provider.PromptCacheRetentionInMemory):
		result.Retention = provider.PromptCacheRetentionInMemory
	case string(provider.PromptCacheRetention24h):
		result.Retention = provider.PromptCacheRetention24h
	}
	return result
}

// parseRequest 将 OpenAI Chat Completions 请求体解析为 RelayRequest。
func parseRequest(body []byte) (*codec.RelayRequest, error) {
	var req openaiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, pkgErrors.NewBambooError("下游", "failed to parse request body", 0)
	}

	// 解析消息
	var systemParts []string
	var systemCacheControl *provider.CacheControl
	var messages []bamboo.BambooMessage

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system", "developer":
			text := extractPlainText(msg.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}
			// system 消息内容数组末位 text 块或消息级 cache_control → system 级缓存标记
			if cc := extractPartCacheControl(msg.Content, msg.CacheControl, true); cc != nil {
				systemCacheControl = cc
			}
		case "user":
			bm, err := parseUserMessage(msg)
			if err != nil {
				return nil, err
			}
			messages = append(messages, bm)
		case "assistant", "model":
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
	if choice, toolName, err := parseToolChoice(req.ToolChoice); err == nil && choice != "" {
		config.ToolChoice = choice
		if toolName != "" {
			setExtra(config, "forced_tool_name", toolName)
		}
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
	// prompt_cache_options / prompt_cache_retention → PromptCacheOptions
	if pco := parsePromptCacheOptions(req.PromptCacheOptions, req.PromptCacheRetention); pco != nil {
		config.PromptCacheOptions = pco
	}
	// cached_content / cachedContent → Gemini 显式缓存资源引用（同时同步至 ProviderExtra 兜底槽）
	if cachedContent := resolveCachedContent(req); cachedContent != "" {
		config.CachedContent = cachedContent
		setExtra(config, "cached_content", cachedContent)
	}
	// system 消息上的块级缓存断点 → system 级缓存标记；缺失时回退到顶层 cache_control
	if systemCacheControl == nil {
		systemCacheControl = parseCacheControlRaw(req.CacheControl)
	}
	if systemCacheControl != nil {
		config.SystemCacheControl = systemCacheControl
	}

	if req.ServiceTier != "" {
		config.ServiceTier = req.ServiceTier
	}

	// ProviderExtra（增量写入，禁止覆盖前面已写入的 forced_tool_name / cached_content 等扩展键）
	if req.FrequencyPenalty != nil {
		setExtra(config, "frequency_penalty", *req.FrequencyPenalty)
	}
	if req.PresencePenalty != nil {
		setExtra(config, "presence_penalty", *req.PresencePenalty)
	}
	if req.Seed != nil {
		setExtra(config, "seed", *req.Seed)
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
	msgCC := parseCacheControlRaw(msg.CacheControl)
	// 尝试 string
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		if msgCC != nil {
			return bamboo.NewUserMessageBlocks(bamboo.NewTextBlockWithCache(s, msgCC)), nil
		}
		return bamboo.NewUserMessage(s), nil
	}
	// 尝试 array
	var parts []openaiContentPart
	if err := json.Unmarshal(msg.Content, &parts); err != nil {
		return bamboo.BambooMessage{}, pkgErrors.NewBambooError("下游", "failed to parse user message content", 0)
	}

	blocks := make([]bamboo.ContentBlock, 0, len(parts))
	hasPartCC := false
	for _, p := range parts {
		cc := resolvePartCacheControl(p.PromptCacheBreakpoint, p.CacheControl)
		if cc != nil {
			hasPartCC = true
		}
		switch p.Type {
		case "text":
			blocks = append(blocks, bamboo.NewTextBlockWithCache(p.Text, cc))
		case "image_url":
			if p.ImageURL != nil {
				blocks = append(blocks, parseImageURL(p.ImageURL.URL, cc))
			}
		}
	}
	if !hasPartCC && msgCC != nil && len(blocks) > 0 {
		attachCacheControl(blocks[len(blocks)-1], msgCC)
	}
	return bamboo.NewUserMessageBlocks(blocks...), nil
}

// attachCacheControl 在消息级兜底场景下为内容块附加 CacheControl。
func attachCacheControl(block bamboo.ContentBlock, cc *provider.CacheControl) {
	if block == nil || cc == nil {
		return
	}
	switch b := block.(type) {
	case *bamboo.TextBlock:
		b.CacheControl = cc
	case *bamboo.ThinkingBlock:
		b.CacheControl = cc
	case *bamboo.ToolUseBlock:
		b.CacheControl = cc
	case *bamboo.ToolResultBlock:
		b.CacheControl = cc
	case *bamboo.ImageBlock:
		b.CacheControl = cc
	case *bamboo.DocumentBlock:
		b.CacheControl = cc
	}
}

// parseImageURL 将 OpenAI image_url 转为 bamboo.ImageBlock。
func parseImageURL(url string, cc *provider.CacheControl) bamboo.ContentBlock {
	// data URI: data:image/png;base64,xxxx
	if strings.HasPrefix(url, "data:") {
		commaIdx := strings.Index(url, ",")
		if commaIdx > 0 {
			header := url[:commaIdx]
			data := url[commaIdx+1:]
			mediaType := parseDataURIMediaType(header)
			return bamboo.NewImageBlockWithCache(bamboo.ContentSource{
				Type:      "base64",
				MediaType: mediaType,
				Data:      data,
			}, cc)
		}
	}
	// 普通 URL
	return bamboo.NewImageBlockWithCache(bamboo.ContentSource{
		Type: "url",
		URL:  url,
	}, cc)
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

	reasoning := extractReasoningContent(msg.ReasoningContent, msg.Reasoning)
	sig := msg.ThinkingSignature
	sigProv := msg.ThinkingProvider
	if sig == "" {
		for _, tc := range msg.ToolCalls {
			if tc.ExtraContent != nil && tc.ExtraContent.Google != nil && tc.ExtraContent.Google.ThoughtSignature != "" {
				sig = tc.ExtraContent.Google.ThoughtSignature
				sigProv = bamboo.SignatureProviderGemini
				break
			}
		}
	}
	if reasoning != "" || sig != "" {
		blocks = append(blocks, bamboo.NewThinkingBlockWithProvider(reasoning, sig, sigProv))
	}

	contentCC := extractPartCacheControl(msg.Content, msg.CacheControl, false)
	textAttached := false
	if text := extractPlainText(msg.Content); text != "" {
		blocks = append(blocks, bamboo.NewTextBlockWithCache(text, contentCC))
		textAttached = true
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
	if !textAttached && contentCC != nil && len(blocks) > 0 {
		attachCacheControl(blocks[len(blocks)-1], contentCC)
	}
	if len(blocks) == 0 {
		out := bamboo.BambooMessage{Role: bamboo.RoleAssistant}
		out.ReasoningID = msg.ReasoningID
		return out
	}
	out := bamboo.NewAssistantMessageBlocks(blocks...)
	out.ReasoningID = msg.ReasoningID
	return out
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
	cc := extractPartCacheControl(msg.Content, msg.CacheControl, false)
	block := bamboo.NewToolResultBlockWithCache(msg.ToolCallID, content, false, cc)
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
func parseToolChoice(raw json.RawMessage) (string, string, error) {
	if len(raw) == 0 {
		return "", "", nil
	}
	// 尝试 string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto", "none", "required":
			return s, "", nil
		default:
			return "", "", nil
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
		return "forced", obj.Function.Name, nil
	}
	return "", "", nil
}

// parseTools 将 OpenAI tools 转为 bamboo.Tool。
func parseTools(tools []openaiTool) []bamboo.Tool {
	result := make([]bamboo.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		cc := parseCacheControlRaw(t.CacheControl)
		if cc == nil {
			cc = parseCacheControlRaw(t.Function.CacheControl)
		}
		tool := bamboo.Tool{
			Name:         t.Function.Name,
			Description:  t.Function.Description,
			CacheControl: cc,
		}
		// parameters 原样保留为 json.RawMessage，确保完整透传所有 JSON Schema 字段
		tool.InputSchema = t.Function.Parameters
		result = append(result, tool)
	}
	return result
}

// 编译时确保 base64 包被使用（data URI 场景中解码验证用）
var _ = base64.StdEncoding

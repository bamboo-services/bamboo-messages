package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
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
	PromptCacheKey     string          `json:"prompt_cache_key,omitempty"`
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
//
// Arguments / Output / Summary 使用 json.RawMessage 而非具体类型，
// 兼容客户端将 arguments 序列化为 JSON object（而非标准 JSON string）、
// output 序列化为 object、summary 序列化为 string 等非标准格式。
type inputItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	// reasoning 专用（标准: []outputReasoningSummary，容错: string）
	Summary json.RawMessage `json:"summary,omitempty"`
	// EncryptedContent 服务端加密的推理内容（客户端多轮回传时原样携带）。
	// 解析为 ThinkingBlock.Signature，使 relay 转发给上游 Responses Provider
	// 时能保留加密推理链；relay 自身不解密、不伪造该值。
	EncryptedContent string `json:"encrypted_content,omitempty"`
	// function_call 专用（标准: JSON string，容错: raw object）
	CallID    string          `json:"call_id,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	// function_call_output 专用（标准: string，容错: object）
	Output json.RawMessage `json:"output,omitempty"`
}

// inputContent input message 的 content 元素。
type inputContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL json.RawMessage `json:"image_url,omitempty"` // input_image 专用（string 或 {"url": "..."}）
	FileID   string          `json:"file_id,omitempty"`   // input_file 专用（file ID 引用）
	FileData string          `json:"file_data,omitempty"` // input_file 专用（base64 数据）
	MimeType string          `json:"mime_type,omitempty"` // input_file 专用（如 "image/png"）
}

// parseRequest 将 OpenAI Responses 请求体解析为 RelayRequest。
func parseRequest(body []byte) (*codec.RelayRequest, error) {
	var req responsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, pkgErrors.NewBambooError("下游", "failed to parse request body", 0)
	}

	var systemParts []string
	extra := make(map[string]any)
	if req.Instructions != "" {
		systemParts = append(systemParts, req.Instructions)
		extra["instructions"] = req.Instructions
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

	if req.PromptCacheKey != "" {
		config.PromptCacheKey = req.PromptCacheKey
	}

	// ProviderExtra：instructions / previous_response_id / store / truncation
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
		// 容错：部分客户端将 input 发送为单个 object 而非 array
		var single inputItem
		if err2 := json.Unmarshal(raw, &single); err2 == nil {
			items = []inputItem{single}
		} else {
			return nil, "", pkgErrors.NewBambooError("下游",
				fmt.Sprintf("failed to parse input field: %v", err), 0)
		}
	}

	var messages []bamboo.BambooMessage
	var systemParts []string

	// Responses 协议中连续的 assistant 侧条目（reasoning / message[assistant] /
	// function_call）属于同一对话轮次，必须合并为单条 assistant 消息——
	// Chat Completions 语义下单轮 assistant 消息同时携带 reasoning_content +
	// content + tool_calls。拆分为多条 assistant 消息后，仅 reasoning 条目对应的
	// 消息携带 reasoning_content，DeepSeek 等思考模式强校验上游会以
	// "reasoning_content must be passed back" 拒绝请求；并行工具调用也会被
	// 错误拆分为多轮。
	var assistantBlocks []bamboo.ContentBlock
	var assistantReasoningID string
	flushAssistant := func() {
		if len(assistantBlocks) == 0 {
			return
		}
		msg := bamboo.NewAssistantMessageBlocks(assistantBlocks...)
		msg.ReasoningID = assistantReasoningID
		messages = append(messages, msg)
		assistantBlocks = nil
		assistantReasoningID = ""
	}

	// 工具结果中提取的截图图片缓冲：Chat Completions 语义下 tool 消息必须
	// 连续紧跟 assistant(tool_calls) 消息，图片作为独立 user 消息只能在
	// 所有 tool 消息之后补发，否则并行工具调用会被 user 消息截断。
	var pendingImages []bamboo.ContentBlock
	flushPendingImages := func() {
		if len(pendingImages) == 0 {
			return
		}
		for _, img := range pendingImages {
			messages = append(messages, bamboo.NewUserMessageBlocks(img))
		}
		pendingImages = nil
	}

	for _, item := range items {
		switch item.Type {
		case "message":
			if item.Role == "assistant" {
				// assistant 消息条目并入当前轮次
				msg, _ := parseInputMessage(item)
				assistantBlocks = append(assistantBlocks, msg.Content...)
				continue
			}
			msg, sys := parseInputMessage(item)
			flushAssistant()
			flushPendingImages()
			if sys != "" {
				systemParts = append(systemParts, sys)
			} else if len(msg.Content) > 0 {
				messages = append(messages, msg)
			}
		case "function_call":
			// assistant 工具调用并入当前轮次（并行工具调用同属一条消息）
			input := normalizeArguments(item.Arguments)
			assistantBlocks = append(assistantBlocks, &bamboo.ToolUseBlock{
				Type:  bamboo.ContentBlockToolUse,
				ID:    item.CallID,
				Name:  item.Name,
				Input: input,
			})
		case "function_call_output":
			// 用户侧工具结果：结束当前 assistant 轮次
			flushAssistant()
			// 提取 output 中内嵌的截图图片（Codex 截图工具链将 base64 写入
			// image_data 字段或 data URI），避免超大 base64 以文本形式计入
			// Chat Completions 上游的输入长度限制；图片缓冲为独立 user 消息，
			// 使上游视觉模型能正常识别截图。
			text, imgs := splitOutputTextAndImage(item.Output)
			messages = append(messages, bamboo.NewUserMessageBlocks(
				bamboo.NewToolResultBlock(item.CallID, text, false),
			))
			pendingImages = append(pendingImages, imgs...)
		case "reasoning":
			// 优先取 content 的 reasoning_text 原始思考全文，缺失时回退到
			// summary（摘要为有损内容，仅作兜底）。encrypted_content 原样
			// 透传为 Signature，保证多轮对话的加密推理链不断裂。
			text := extractReasoningText(item.Content)
			if text == "" {
				text = extractSummaryText(normalizeSummary(item.Summary))
			}
			if text != "" || item.EncryptedContent != "" {
				assistantBlocks = append(assistantBlocks,
					bamboo.NewThinkingBlock(text, item.EncryptedContent))
				if assistantReasoningID == "" {
					assistantReasoningID = item.ID
				}
			}
		default:
			flushAssistant()
			xLog.WithName("codec/responses").SugarWarn(context.Background(),
				fmt.Sprintf("unknown input item type %q, skipped", item.Type))
		}
	}
	flushAssistant()
	flushPendingImages()

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
				if url, ok := normalizeImageURL(p.ImageURL); ok {
					blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
						Type: "url",
						URL:  url,
					}))
				}
			case "input_file":
				if p.FileID != "" {
					blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
						Type: "url",
						URL:  p.FileID,
					}))
				} else if p.FileData != "" {
					// 图片类型的内联文件（file_data + mime_type: image/*）
					// 转为 ImageBlock，Chat Completions 上游才能以 image_url
					// 识别；文档类型保持 DocumentBlock。
					if strings.HasPrefix(p.MimeType, "image/") {
						blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
							Type:      "base64",
							MediaType: p.MimeType,
							Data:      p.FileData,
						}))
					} else {
						blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
							Type: "base64",
							Data: p.FileData,
						}))
					}
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
				if url, ok := normalizeImageURL(p.ImageURL); ok {
					blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
						Type: "url",
						URL:  url,
					}))
				}
			case "input_file":
				if p.FileID != "" {
					blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
						Type: "url",
						URL:  p.FileID,
					}))
				} else if p.FileData != "" {
					// 图片类型的内联文件（file_data + mime_type: image/*）
					// 转为 ImageBlock，Chat Completions 上游才能以 image_url
					// 识别；文档类型保持 DocumentBlock。
					if strings.HasPrefix(p.MimeType, "image/") {
						blocks = append(blocks, bamboo.NewImageBlock(bamboo.ContentSource{
							Type:      "base64",
							MediaType: p.MimeType,
							Data:      p.FileData,
						}))
					} else {
						blocks = append(blocks, bamboo.NewDocumentBlock(bamboo.ContentSource{
							Type: "base64",
							Data: p.FileData,
						}))
					}
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

// ── 字段规范化 helpers ──

// normalizeArguments 将 function_call 的 arguments 规范化为 json.RawMessage。
//
// 兼容两种客户端序列化格式：
//   - 标准 JSON string: "{"city":"SF"}" → 提取内部 JSON
//   - 非标准 raw object: {"city":"SF"} → 直接使用
func normalizeArguments(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	// 尝试解析为 JSON string（标准 Responses 格式）
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return json.RawMessage(`{}`)
		}
		return json.RawMessage(s)
	}
	// 已经是 raw object/array（非标准但合法的 JSON），直接使用
	return raw
}

// normalizeOutputString 将 function_call_output 的 output 规范化为 string。
//
// 兼容两种客户端序列化格式：
//   - 标准 JSON string: "Sunny, 72F" → 提取内部字符串
//   - 非标准 raw object: {"result":"ok"} → 序列化为 JSON 字符串
func normalizeOutputString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试解析为 JSON string（标准格式）
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// 非 string 类型（object/array/number），原样序列化为字符串
	return string(raw)
}

// splitOutputTextAndImage 将 function_call_output 的 output 拆分为
// 工具结果文本与内嵌的截图图片。
//
// Codex 截图工具链等客户端将截图 base64 直接写入 output：
//   - 数组格式: [{"detail":"high","image_url":"data:image/png;base64,..."}]
//   - object 格式: {"image_data": "<base64>", "width": 1920, "height": 1080}
//   - string 格式: "data:image/png;base64,..."
//
// 若整段以文本形式转发给 Chat Completions 上游，超大 base64 会被计入输入
// 长度限制（如阿里云百炼的 "Range of input length should be [1, 983616]"）
// 导致请求被拒，且截图无法被上游视觉模型识别。此处将图片提取为
// ImageBlock，其余字段（尺寸等元数据）保留为工具结果文本。
func splitOutputTextAndImage(raw json.RawMessage) (string, []bamboo.ContentBlock) {
	if len(raw) == 0 {
		return "", nil
	}

	// 标准 string 格式：data URI 识别为图片，其余保留为文本
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if img := imageFromDataURI(s); img != nil {
			return "", []bamboo.ContentBlock{img}
		}
		return s, nil
	}

	// 数组格式（Codex 截图工具链）：遍历元素提取 image_url 图片，
	// 其余元素原样保留为文本；无图片时保持原样序列化。
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) == nil {
		var texts []string
		var images []bamboo.ContentBlock
		for _, itemRaw := range items {
			var itemMap map[string]json.RawMessage
			if json.Unmarshal(itemRaw, &itemMap) != nil {
				// 元素不是 object（如纯字符串），保留原文
				texts = append(texts, string(itemRaw))
				continue
			}
			urlRaw, hasURL := itemMap["image_url"]
			if !hasURL {
				texts = append(texts, string(itemRaw))
				continue
			}
			url, ok := normalizeImageURL(urlRaw)
			if !ok {
				texts = append(texts, string(itemRaw))
				continue
			}
			delete(itemMap, "image_url")
			if img := imageFromDataURI(url); img != nil {
				images = append(images, img)
			} else {
				images = append(images, &bamboo.ImageBlock{
					Type: bamboo.ContentBlockImage,
					Source: &bamboo.ContentSource{
						Type: "url",
						URL:  url,
					},
				})
			}
			if len(itemMap) > 0 {
				texts = append(texts, compactObjectText(itemMap))
			}
		}
		if len(images) > 0 {
			return strings.Join(texts, "\n"), images
		}
		return string(raw), nil
	}

	// object 格式：优先提取 image_data（base64 截图），其次 image_url
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		if dataRaw, ok := obj["image_data"]; ok {
			var data string
			if json.Unmarshal(dataRaw, &data) == nil && data != "" {
				delete(obj, "image_data")
				return compactObjectText(obj), []bamboo.ContentBlock{&bamboo.ImageBlock{
					Type: bamboo.ContentBlockImage,
					Source: &bamboo.ContentSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      data,
					},
				}}
			}
		}
		if urlRaw, ok := obj["image_url"]; ok {
			if url, ok2 := normalizeImageURL(urlRaw); ok2 {
				delete(obj, "image_url")
				if img := imageFromDataURI(url); img != nil {
					return compactObjectText(obj), []bamboo.ContentBlock{img}
				}
				return compactObjectText(obj), []bamboo.ContentBlock{&bamboo.ImageBlock{
					Type: bamboo.ContentBlockImage,
					Source: &bamboo.ContentSource{
						Type: "url",
						URL:  url,
					},
				}}
			}
		}
	}

	// 未识别到图片：保持现有行为，原样序列化为文本
	return string(raw), nil
}

// compactObjectText 将 map 序列化为紧凑 JSON 字符串（工具结果文本）。
func compactObjectText(obj map[string]json.RawMessage) string {
	if len(obj) == 0 {
		return "{}"
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// imageFromDataURI 解析 data URI 形式的图片地址。
//
// 支持格式: data:image/png;base64,iVBOR... → 提取 MIME 类型与 base64 数据。
// 非图片 data URI 或普通 URL 返回 nil。
func imageFromDataURI(s string) *bamboo.ImageBlock {
	const dataImagePrefix = "data:image/"
	const base64Marker = ";base64,"
	if !strings.HasPrefix(s, dataImagePrefix) {
		return nil
	}
	idx := strings.Index(s, base64Marker)
	if idx < 0 {
		return nil
	}
	data := s[idx+len(base64Marker):]
	if data == "" {
		return nil
	}
	return &bamboo.ImageBlock{
		Type: bamboo.ContentBlockImage,
		Source: &bamboo.ContentSource{
			Type:      "base64",
			MediaType: strings.TrimPrefix(s[:idx], "data:"),
			Data:      data,
		},
	}
}

// normalizeImageURL 兼容 input_image.image_url 的两种序列化格式：
//   - 标准 string: "https://..." 或 "data:image/png;base64,..."
//   - 非标准 object: {"url": "..."}（Chat Completions 风格）
func normalizeImageURL(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	var obj struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.URL != "" {
		return obj.URL, true
	}
	return "", false
}

// normalizeSummary 将 reasoning item 的 summary 规范化为 []outputReasoningSummary。
//
// 兼容两种客户端序列化格式：
//   - 标准 array: [{"type":"summary_text","text":"..."}]
//   - 非标准 string: "some summary text" → 包装为单条 summary_text
func normalizeSummary(raw json.RawMessage) []outputReasoningSummary {
	if len(raw) == 0 {
		return nil
	}
	// 尝试标准 array 格式
	var arr []outputReasoningSummary
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	// 容错：string 格式视为单条摘要
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return []outputReasoningSummary{{Type: "summary_text", Text: s}}
	}
	return nil
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

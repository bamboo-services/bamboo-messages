package bamboo

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// finishReasonMap provider.FinishReason → bamboo.FinishReason 映射表。
//
// 用于将底层 provider 的完成原因映射为上层 bamboo 的标准完成原因。
var finishReasonMap = map[provider.FinishReason]FinishReason{
	provider.FinishReasonStop:      FinishReasonEndTurn,
	provider.FinishReasonLength:    FinishReasonMaxTokens,
	provider.FinishReasonToolCalls: FinishReasonToolUse,
}

// messagesToProvider 将 bamboo.BambooMessage 列表转换为 provider.Message 列表。
//
// tool_result 消息会被拆分为独立的 Role=tool 消息；
// image/document 类型会被静默丢弃并记录日志。
// thinking block 的内容会被保留到 provider.Message 的 ThinkingContent/ThinkingSignature 字段。
func messagesToProvider(msgs []BambooMessage) ([]provider.Message, error) {
	var result []provider.Message
	for _, msg := range msgs {
		var textBuilder strings.Builder
		var toolCalls []provider.ToolCall
		var toolResults []provider.Message
		var contentBlocks []provider.ContentBlock
		var msgCacheControl *provider.CacheControl
		var ccCount int
		var thinkingContent string
		var thinkingSignature string

		// Content 为空数组时允许透传，生成 Content="" 的空消息，
		// 由下游适配器内部处理（补空字符串或跳过），外部允许传递空 content。
		for _, block := range msg.Content {
			switch b := block.(type) {
			case *TextBlock:
				textBuilder.WriteString(b.Text)
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
				}
			case *ThinkingBlock:
				// 保留思考过程到 provider.Message 的 ThinkingContent/ThinkingSignature 字段，
				// 用于多轮对话中向 provider 回传 thinking block 内容。
				// 多个 ThinkingBlock 时拼接内容，保留最后一个签名。
				if b.Thinking != "" {
					thinkingContent += b.Thinking
					thinkingSignature = b.Signature
				}
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
				}
			case *ToolUseBlock:
				toolCalls = append(toolCalls, provider.ToolCall{
					ID:   b.ID,
					Type: "function",
					Function: provider.FunctionCall{
						Name:      b.Name,
						Arguments: string(b.Input),
					},
				})
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
				}
			case *ToolResultBlock:
				toolResults = append(toolResults, provider.Message{
					Role:         provider.RoleTool,
					Content:      b.Content,
					ToolCallID:   b.ToolUseID,
					ToolName:     b.ToolName,
					CacheControl: b.CacheControl,
				})
			case *ImageBlock:
				// 仅用户消息支持图片内容块
				if msg.Role == RoleUser && b.Source != nil {
					contentBlocks = append(contentBlocks, provider.ImageContentBlock{
						Source: provider.ImageSource{
							Type:      b.Source.Type,
							MediaType: b.Source.MediaType,
							Data:      b.Source.Data,
							URL:       b.Source.URL,
						},
					})
				}
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
				}
			case *DocumentBlock:
				// 仅用户消息支持文档内容块
				if msg.Role == RoleUser && b.Source != nil {
					contentBlocks = append(contentBlocks, provider.DocumentContentBlock{
						Source: provider.DocumentSource{
							Type:      b.Source.Type,
							MediaType: b.Source.MediaType,
							Data:      b.Source.Data,
							URL:       b.Source.URL,
						},
					})
				}
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
				}
			default:
				// 未知的 ContentBlock 类型，记录详细信息以便排查
				log.Printf("[bamboo] warning: dropped unsupported content block type %q (implement %T in messagesToProvider to support it)", b.BlockType(), b)
			}
		}

		if ccCount > 1 {
			log.Printf("[bamboo] warning: message has %d cache_control breakpoints, only the last one is kept", ccCount)
		}

		content := textBuilder.String()
		hasContent := content != "" || len(toolCalls) > 0 || len(contentBlocks) > 0 || thinkingContent != ""
		if hasContent || len(msg.Content) == 0 {
			if msg.Role == RoleAssistant && content == "" && len(toolCalls) == 0 && len(contentBlocks) == 0 && thinkingContent == "" {
				content = "-"
			}
			result = append(result, provider.Message{
				Role:              providerRole(msg.Role),
				Content:           content,
				ContentBlocks:     contentBlocks,
				ThinkingContent:   thinkingContent,
				ThinkingSignature: thinkingSignature,
				ToolCalls:         toolCalls,
				CacheControl:      msgCacheControl,
			})
		}
		result = append(result, toolResults...)
	}
	return result, nil
}

// providerRole 将 bamboo.MessageRole 转换为 provider.MessageRole。
//
// 已知角色直接映射；"system" 角色记录警告并降级为 user（应通过 system 参数传递）；
// 其他未知角色记录警告并降级为 user。
func providerRole(role MessageRole) provider.MessageRole {
	switch role {
	case RoleUser:
		return provider.RoleUser
	case RoleAssistant:
		return provider.RoleAssistant
	case "system":
		// system 角色应通过 Chat/Complete 的 system 参数传递，而非消息角色。
		// 此处记录警告并降级为 user 角色，避免请求被拒绝。
		log.Printf(`[bamboo] warning: message role "system" should use the system parameter instead, falling back to "user"`)
		return provider.RoleUser
	default:
		log.Printf("[bamboo] warning: unknown message role %q, falling back to \"user\"", role)
		return provider.RoleUser
	}
}

// configToProvider 将 bamboo.RequestConfig 转换为 provider.ChatConfig。
//
// 复制所有公共字段（Model, MaxTokens, Temperature, TopP, Stop, Tools, Metadata），
// 并透传 ThinkingConfig 和 ProviderExtra。
func configToProvider(cfg *RequestConfig) *provider.ChatConfig {
	if cfg == nil {
		return nil
	}
	return &provider.ChatConfig{
		Model:              cfg.Model,
		MaxTokens:          cfg.MaxTokens,
		Temperature:        cfg.Temperature,
		TopP:               cfg.TopP,
		Stop:               cfg.StopSequences,
		Tools:              toolsToProvider(cfg.Tools),
		Metadata:           cfg.Metadata,
		UserID:             cfg.UserID,
		ToolChoice:         cfg.ToolChoice,
		ResponseFormat:     cfg.ResponseFormat,
		ParallelToolCalls:  cfg.ParallelToolCalls,
		ThinkingConfig:     cfg.ThinkingConfig,
		SystemCacheControl: cfg.SystemCacheControl,
		PromptCacheKey:     cfg.PromptCacheKey,
		ProviderExtra:      cfg.ProviderExtra,
	}
}

// toolsToProvider 将 bamboo.Tool 列表转换为 provider.Tool 列表。
//
// 将每个 bamboo.Tool 转换为 provider.Tool，类型固定为 "function"，
// 函数定义包含名称、描述和 JSON Schema 参数。
// InputSchema 作为 json.RawMessage 原样解析为 map，确保完整保留所有 JSON Schema 字段。
func toolsToProvider(tools []Tool) []provider.Tool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]provider.Tool, 0, len(tools))
	for _, t := range tools {
		result = append(result, provider.Tool{
			Type: "function",
			Function: provider.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  buildParameters(t.InputSchema),
			},
			CacheControl: t.CacheControl,
		})
	}
	return result
}

// buildParameters 将 InputSchema（json.RawMessage）解析为 map[string]any。
//
// 作为完整 JSON Schema 的无损中间表示，直接反序列化原始 JSON，
// 避免结构体字段穷举导致的 schema 字段丢失（如 additionalProperties、嵌套 properties 等）。
// 若 schema 为空则返回 nil（表示工具无参数）。
func buildParameters(schema json.RawMessage) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	var params map[string]any
	if err := json.Unmarshal(schema, &params); err != nil {
		// 解析失败时回退为最小合法 schema，保证请求不因工具定义格式问题中断
		return map[string]any{"type": "object"}
	}
	return params
}

// resultToResponse 将 provider.CompletionResult 转换为 bamboo.Response。
//
// 生成唯一的 ID 和 RequestID，填充 Type / Role / StopReason / Usage /
// ProviderType / CreatedAt 等字段，将工具调用转换为 ToolUse 类型的 ContentBlock。
// 若 result 包含 Thinking 内容，会生成 ThinkingBlock 放在 Content 最前面。
func resultToResponse(result *provider.CompletionResult, providerType string) *Response {
	if result == nil {
		return nil
	}
	var content []ContentBlock
	if result.Thinking != "" {
		content = append(content, NewThinkingBlock(result.Thinking, result.ThinkingSignature))
	}
	if result.Content != "" {
		content = append(content, NewTextBlock(result.Content))
	}
	for _, tc := range result.ToolCalls {
		content = append(content, NewToolUseBlockWithRawInput(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}
	if len(content) == 0 {
		content = []ContentBlock{}
	}
	return &Response{
		ID:         fmt.Sprintf("bamboo_msg_%d", time.Now().UnixNano()),
		Type:       "message",
		Role:       RoleAssistant,
		Content:    content,
		StopReason: mapFinishReason(result.FinishReason),
		Usage: Usage{
			InputTokens:              result.Usage.InputTokens,
			OutputTokens:             result.Usage.OutputTokens,
			CacheCreationInputTokens: result.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     result.Usage.CacheReadInputTokens,
		},
		ProviderType: providerType,
		RequestID:    fmt.Sprintf("req_%d", time.Now().UnixNano()),
		ResponseID:   result.ResponseID,
		CreatedAt:    time.Now().Unix(),
	}
}

func mapFinishReason(reason provider.FinishReason) FinishReason {
	if r, ok := finishReasonMap[reason]; ok {
		return r
	}
	return FinishReasonEndTurn
}

// StreamConverter 维护流事件转换状态，将 provider.StreamEvent 序列映射为 Anthropic 风格事件序列。
type StreamConverter struct {
	blockIndex               int
	nextBlockIndex           int
	usage                    *Usage
	started                  bool
	textBlockStarted         bool
	textBlockIndex           int
	thinkingBlockStarted     bool
	thinkingBlockIndex       int
	currentToolBlockIndex    int
	toolBlockStarted         bool
	openToolBlockIndexes     []int
	toolBlockByProviderIndex map[int]int
	toolBlockByID            map[string]int
	finishReason             FinishReason
	stopHandled              bool
}

func NewStreamConverter() *StreamConverter { return &StreamConverter{} }

// Convert 将单个 provider.StreamEvent 转换为 bamboo.StreamEvent 列表。
//
// 根据事件类型路由到不同的处理方法，自动管理内容块索引
// 和 BlockStart 状态追踪。
//
// StreamTypeStop 不会立即输出终止事件，而是缓存 finishReason 并等待
// StreamTypeDone 触发 handleStop()。这彻底消除了双 stop 导致的
// finish_reason 覆盖问题——无论上游发多少个 StreamTypeStop，
// 客户端只收到一次终止事件序列。
func (sc *StreamConverter) Convert(event provider.StreamEvent) []StreamEvent {
	switch event.Type {
	case provider.StreamTypeStart:
		return sc.handleStart()
	case provider.StreamTypeDelta:
		return sc.handleDelta(event.Delta)
	case provider.StreamTypeStop:
		sc.recordFinishReason(event.FinishReason)
		return nil
	case provider.StreamTypeDone:
		if !sc.started || sc.stopHandled {
			return nil
		}
		return sc.handleStop()
	case provider.StreamTypeError:
		return sc.handleError(event.Err)
	default:
		return nil
	}
}

// recordFinishReason 以优先级策略记录完成原因。
//
// 优先级：tool_use > max_tokens > end_turn > stop_sequence。
// 当已记录的原因为 tool_use 时不允许被后续的 stop 覆盖——
// 这是 agent loop 不中断的关键保障：即使上游先发 stop 后发 tool_calls，
// 最终输出给客户端的 finish_reason 始终是 tool_calls。
func (sc *StreamConverter) recordFinishReason(reason provider.FinishReason) {
	incoming := mapFinishReason(reason)
	if finishReasonPriority(incoming) > finishReasonPriority(sc.finishReason) {
		sc.finishReason = incoming
	}
}

// finishReasonPriority 返回完成原因的优先级权重。
// tool_use 最高（2），确保有工具调用时不会被其他原因覆盖。
// max_tokens 次之（1），因为 token 限制是上游的硬性截断。
// 其他原因最低（0）。
func finishReasonPriority(r FinishReason) int {
	switch r {
	case FinishReasonToolUse:
		return 2
	case FinishReasonMaxTokens:
		return 1
	default:
		return 0
	}
}

func (sc *StreamConverter) handleStart() []StreamEvent {
	sc.started = true
	sc.blockIndex = 0
	sc.nextBlockIndex = 0
	sc.textBlockStarted = false
	sc.textBlockIndex = 0
	sc.thinkingBlockStarted = false
	sc.thinkingBlockIndex = 0
	sc.currentToolBlockIndex = 0
	sc.toolBlockStarted = false
	sc.openToolBlockIndexes = nil
	sc.toolBlockByProviderIndex = make(map[int]int)
	sc.toolBlockByID = make(map[string]int)
	sc.finishReason = ""
	sc.stopHandled = false
	return []StreamEvent{
		{
			Type:    EventMessageStart,
			Message: &BambooMessage{Role: RoleAssistant, Content: []ContentBlock{}},
			Usage:   &Usage{},
		},
	}
}

func (sc *StreamConverter) nextIndex() int {
	idx := sc.nextBlockIndex
	sc.nextBlockIndex++
	sc.blockIndex = idx
	return idx
}

func (sc *StreamConverter) stopTextOrThinkingBlock() []StreamEvent {
	var events []StreamEvent
	if sc.thinkingBlockStarted {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.thinkingBlockIndex,
		})
	}
	if sc.textBlockStarted {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.textBlockIndex,
		})
	}
	return events
}

func (sc *StreamConverter) startTextBlock() StreamEvent {
	idx := sc.nextIndex()
	sc.textBlockStarted = true
	sc.textBlockIndex = idx
	return StreamEvent{
		Type:         EventContentBlockStart,
		Index:        idx,
		ContentBlock: NewTextBlock(""),
	}
}

func (sc *StreamConverter) startThinkingBlock() StreamEvent {
	idx := sc.nextIndex()
	sc.thinkingBlockStarted = true
	sc.thinkingBlockIndex = idx
	return StreamEvent{
		Type:         EventContentBlockStart,
		Index:        idx,
		ContentBlock: NewThinkingBlock("", ""),
	}
}

func (sc *StreamConverter) startToolBlock(data provider.ToolCallData) []StreamEvent {
	var events []StreamEvent
	events = append(events, sc.stopTextOrThinkingBlock()...)

	if data.HasIndex {
		if idx, ok := sc.toolBlockByProviderIndex[data.Index]; ok {
			sc.currentToolBlockIndex = idx
			sc.toolBlockStarted = true
			sc.blockIndex = idx
			return events
		}
	}
	if data.ID != "" {
		if idx, ok := sc.toolBlockByID[data.ID]; ok {
			sc.currentToolBlockIndex = idx
			sc.toolBlockStarted = true
			sc.blockIndex = idx
			return events
		}
	}

	idx := sc.nextIndex()
	sc.currentToolBlockIndex = idx
	sc.toolBlockStarted = true
	sc.openToolBlockIndexes = append(sc.openToolBlockIndexes, idx)
	if data.HasIndex {
		sc.toolBlockByProviderIndex[data.Index] = idx
	}
	if data.ID != "" {
		sc.toolBlockByID[data.ID] = idx
	}

	events = append(events, StreamEvent{
		Type:         EventContentBlockStart,
		Index:        idx,
		ContentBlock: NewToolUseBlock(data.ID, data.Name, nil),
	})
	return events
}

func (sc *StreamConverter) toolDeltaIndex(data any) (int, bool) {
	switch d := data.(type) {
	case provider.ToolCallDeltaData:
		if sc.toolBlockStarted {
			return sc.currentToolBlockIndex, true
		}
		return sc.blockIndex, true
	case provider.IndexedToolCallDeltaData:
		if d.HasIndex {
			if idx, ok := sc.toolBlockByProviderIndex[d.Index]; ok {
				return idx, true
			}
		}
		if sc.toolBlockStarted {
			return sc.currentToolBlockIndex, true
		}
		return sc.blockIndex, true
	default:
		return 0, false
	}
}

func toolDeltaPartialJSON(data any) (string, bool) {
	switch d := data.(type) {
	case provider.ToolCallDeltaData:
		return string(d), true
	case provider.IndexedToolCallDeltaData:
		return d.PartialJSON, true
	default:
		return "", false
	}
}

func (sc *StreamConverter) handleDelta(delta provider.StreamDelta[any]) []StreamEvent {
	switch delta.Type {
	case provider.StreamDeltaTypeBlockStart:
		data, ok := delta.Data.(provider.BlockStartData)
		if !ok {
			return nil
		}

		var events []StreamEvent

		switch mapBlockType(data.BlockType) {
		case ContentBlockText:
			events = append(events, sc.stopTextOrThinkingBlock()...)
			events = append(events, sc.startTextBlock())
		case ContentBlockThinking:
			events = append(events, sc.stopTextOrThinkingBlock()...)
			events = append(events, sc.startThinkingBlock())
		case ContentBlockToolUse:
			events = append(events, sc.startToolBlock(provider.ToolCallData{ID: data.ID, Name: data.Name})...)
		default:
			events = append(events, sc.stopTextOrThinkingBlock()...)
			events = append(events, sc.startTextBlock())
		}
		return events
	case provider.StreamDeltaTypeTextOutput:
		var events []StreamEvent
		if !sc.textBlockStarted {
			events = append(events, sc.stopTextOrThinkingBlock()...)
			events = append(events, sc.startTextBlock())
		}
		textData, ok := delta.Data.(provider.TextData)
		if !ok {
			return events
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.textBlockIndex,
			Delta: &StreamDelta{Type: DeltaTextDelta, Text: string(textData)},
		})
		return events
	case provider.StreamDeltaTypeThinking:
		var events []StreamEvent
		if !sc.thinkingBlockStarted {
			events = append(events, sc.stopTextOrThinkingBlock()...)
			events = append(events, sc.startThinkingBlock())
		}
		thinkingData, ok := delta.Data.(provider.ThinkingData)
		if !ok {
			return events
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.thinkingBlockIndex,
			Delta: &StreamDelta{Type: DeltaThinkingDelta, Thinking: string(thinkingData)},
		})
		return events
	case provider.StreamDeltaTypeToolCall:
		data, ok := delta.Data.(provider.ToolCallData)
		if !ok {
			return nil
		}
		return sc.startToolBlock(data)
	case provider.StreamDeltaTypeToolCallDelta:
		partialJSON, ok := toolDeltaPartialJSON(delta.Data)
		if !ok {
			return nil
		}
		idx, ok := sc.toolDeltaIndex(delta.Data)
		if !ok {
			return nil
		}
		return []StreamEvent{{
			Type:  EventContentBlockDelta,
			Index: idx,
			Delta: &StreamDelta{Type: DeltaInputJSON, PartialJSON: partialJSON},
		}}
	case provider.StreamDeltaTypeUsage:
		data, ok := delta.Data.(provider.UsageData)
		if !ok {
			return nil
		}
		sc.usage = &Usage{
			InputTokens:              data.InputTokens,
			OutputTokens:             data.OutputTokens,
			CacheCreationInputTokens: data.CacheCreationInputTokens,
			CacheReadInputTokens:     data.CacheReadInputTokens,
		}
		// 立即返回 usage 事件，确保流中断时 usage 不丢失
		return []StreamEvent{{
			Type:  EventMessageDelta,
			Delta: &MessageDelta{},
			Usage: &Usage{
				InputTokens:              data.InputTokens,
				OutputTokens:             data.OutputTokens,
				CacheCreationInputTokens: data.CacheCreationInputTokens,
				CacheReadInputTokens:     data.CacheReadInputTokens,
			},
		}}
	default:
		return nil
	}
}

func (sc *StreamConverter) handleStop() []StreamEvent {
	sc.stopHandled = true
	usage := sc.usage
	if usage == nil {
		usage = &Usage{}
	}
	// 使用适配器提供的完成原因，若未提供则默认为 FinishReasonEndTurn
	stopReason := sc.finishReason
	if stopReason == "" {
		stopReason = FinishReasonEndTurn
	}

	var events []StreamEvent
	events = append(events, sc.stopTextOrThinkingBlock()...)
	for _, idx := range sc.openToolBlockIndexes {
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: idx})
	}
	sc.openToolBlockIndexes = nil
	sc.toolBlockStarted = false
	sc.toolBlockByProviderIndex = nil
	sc.toolBlockByID = nil

	events = append(events,
		StreamEvent{Type: EventMessageDelta, Delta: &MessageDelta{StopReason: stopReason}, Usage: usage},
		StreamEvent{Type: EventMessageStop},
	)
	return events
}

func mapBlockType(blockType string) ContentBlockType {
	switch blockType {
	case "text":
		return ContentBlockText
	case "thinking":
		return ContentBlockThinking
	case "tool_use":
		return ContentBlockToolUse
	default:
		return ContentBlockText
	}
}

func (sc *StreamConverter) handleError(err *xError.Error) []StreamEvent {
	msg := "unknown error"
	if err != nil {
		msg = err.Error()
	}
	return []StreamEvent{{
		Type:  EventError,
		Error: NewBambooError(ErrorTypeProvider, msg),
	}}
}

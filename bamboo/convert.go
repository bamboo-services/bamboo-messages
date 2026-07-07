package bamboo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
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
		var cacheControlBlockType string
		var ccCount int
		var thinkingContent string
		var thinkingSignature string
		var redactedThinkingData string

		// Content 为空数组时允许透传，生成 Content="" 的空消息，
		// 由下游适配器内部处理（补空字符串或跳过），外部允许传递空 content。
		for _, block := range msg.Content {
			switch b := block.(type) {
			case *TextBlock:
				textBuilder.WriteString(b.Text)
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
					cacheControlBlockType = "text"
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
					cacheControlBlockType = "thinking"
				}
			case *RedactedThinkingBlock:
				redactedThinkingData = b.Data
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
					cacheControlBlockType = "tool_use"
				}
			case *ToolResultBlock:
				toolResults = append(toolResults, provider.Message{
					Role:                  provider.RoleTool,
					Content:               b.Content,
					ToolCallID:            b.ToolUseID,
					ToolName:              b.ToolName,
					CacheControl:          b.CacheControl,
					CacheControlBlockType: "tool_result",
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
					if b.CacheControl != nil {
						ccCount++
						msgCacheControl = b.CacheControl
						cacheControlBlockType = "image"
					}
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
					if b.CacheControl != nil {
						ccCount++
						msgCacheControl = b.CacheControl
						cacheControlBlockType = "document"
					}
				}
			default:
				// 未知的 ContentBlock 类型，记录详细信息以便排查
				xLog.WithName("bamboo").SugarWarn(context.Background(),
					fmt.Sprintf("warning: dropped unsupported content block type %q (implement %T in messagesToProvider to support it)", b.BlockType(), b))
			}
		}

		if ccCount > 1 {
			xLog.WithName("bamboo").SugarWarn(context.Background(),
				fmt.Sprintf("warning: message has %d cache_control breakpoints, only the last one is kept", ccCount))
		}

		content := textBuilder.String()
		hasContent := content != "" || len(toolCalls) > 0 || len(contentBlocks) > 0 || thinkingContent != "" || redactedThinkingData != ""
		if hasContent || len(msg.Content) == 0 {
			if msg.Role == RoleAssistant && content == "" && len(toolCalls) == 0 && len(contentBlocks) == 0 && thinkingContent == "" && redactedThinkingData == "" {
				content = "-"
			}
			if msgCacheControl != nil && !hasContent && len(msg.Content) > 0 {
				xLog.WithName("bamboo").SugarWarn(context.Background(),
					"warning: message has CacheControl but no content blocks were produced, cache_control will be lost")
			}
			result = append(result, provider.Message{
				Role:                  providerRole(msg.Role),
				Content:               content,
				ContentBlocks:         contentBlocks,
				ThinkingContent:       thinkingContent,
				ThinkingSignature:     thinkingSignature,
				RedactedThinkingData:  redactedThinkingData,
				ReasoningID:           msg.ReasoningID,
				ToolCalls:             toolCalls,
				CacheControl:          msgCacheControl,
				CacheControlBlockType: cacheControlBlockType,
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
		xLog.WithName("bamboo").SugarWarn(context.Background(),
			`warning: message role "system" should use the system parameter instead, falling back to "user"`)
		return provider.RoleUser
	default:
		xLog.WithName("bamboo").SugarWarn(context.Background(),
			fmt.Sprintf("warning: unknown message role %q, falling back to \"user\"", role))
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
	for _, rt := range result.RedactedThinking {
		content = append(content, NewRedactedThinkingBlock(rt))
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
	stoppedBlockIndexes      map[int]bool  // 已发送 content_block_stop 的 block index 集合（防重复）
	metadata                 *MessageDelta // 由 MetadataDelta 收集的元信息，在 handleStop 时输出
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
	sc.stoppedBlockIndexes = make(map[int]bool)
	sc.metadata = nil
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

// stopForNewBlock 在开始新内容块前关闭异类已活跃的块。
//
// 仅在类型切换时关闭对方类型（如开始 text 时关闭 thinking，反之亦然），
// 允许同一类型的增量在已有 block 上继续追加。
// startToolBlock 和 handleStop 仍通过 stopAllTextOrThinking 关闭全部。
func (sc *StreamConverter) stopForNewBlock(newType ContentBlockType) []StreamEvent {
	var events []StreamEvent
	if newType == ContentBlockText && sc.thinkingBlockStarted {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.thinkingBlockIndex,
		})
	}
	if newType == ContentBlockThinking && sc.textBlockStarted {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.textBlockIndex,
		})
	}
	return events
}

// stopAllTextOrThinking 关闭所有活跃的 text/thinking 块，用于 tool block 开始和流结束。
func (sc *StreamConverter) stopAllTextOrThinking() []StreamEvent {
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
	events = append(events, sc.stopAllTextOrThinking()...)

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
			if !sc.textBlockStarted {
				events = append(events, sc.stopForNewBlock(ContentBlockText)...)
				events = append(events, sc.startTextBlock())
			}
		case ContentBlockThinking:
			if !sc.thinkingBlockStarted {
				events = append(events, sc.stopForNewBlock(ContentBlockThinking)...)
				events = append(events, sc.startThinkingBlock())
			}
		case ContentBlockRedactedThinking:
			// redacted_thinking 的完整生命周期由 StreamDeltaTypeRedactedThinking 独立处理，
			// block_start 阶段不创建任何内容块，避免被错误映射为文本块。
			return nil
		case ContentBlockToolUse:
			events = append(events, sc.startToolBlock(provider.ToolCallData{ID: data.ID, Name: data.Name})...)
		default:
			if !sc.textBlockStarted {
				events = append(events, sc.stopForNewBlock(ContentBlockText)...)
				events = append(events, sc.startTextBlock())
			}
		}
		return events
	case provider.StreamDeltaTypeTextOutput:
		var events []StreamEvent
		if !sc.textBlockStarted {
			events = append(events, sc.stopForNewBlock(ContentBlockText)...)
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
			events = append(events, sc.stopForNewBlock(ContentBlockThinking)...)
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
	case provider.StreamDeltaTypeSignature:
		// 防御性处理: omitted 模式下 Provider 只发 signature_delta 不发 thinking_delta，
		// 此时 thinkingBlockStarted 为 false。自动开启 thinking block 以保留 signature。
		var events []StreamEvent
		if !sc.thinkingBlockStarted {
			events = append(events, sc.stopForNewBlock(ContentBlockThinking)...)
			events = append(events, sc.startThinkingBlock())
		}
		sigData, ok := delta.Data.(provider.SignatureData)
		if !ok {
			return events
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.thinkingBlockIndex,
			Delta: &StreamDelta{Type: DeltaSignature, Signature: string(sigData)},
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
	case provider.StreamDeltaTypeBlockStop:
		data, ok := delta.Data.(provider.BlockStopData)
		if !ok {
			return nil
		}
		return sc.handleBlockStop(data)
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
		// 通过 Ping 事件携带 usage，确保流中断时 relay 层仍可提取 usage。
		// 不使用 EventMessageDelta 以避免在 finish_reason 之前产生携带 usage 的终止语义 chunk，
		// 导致部分客户端（如 Vercel AI SDK）误判流结束。
		// 最终 usage 统一由 handleStop() 在 message_delta 中输出。
		return []StreamEvent{{
			Type: EventPing,
			Usage: &Usage{
				InputTokens:              data.InputTokens,
				OutputTokens:             data.OutputTokens,
				CacheCreationInputTokens: data.CacheCreationInputTokens,
				CacheReadInputTokens:     data.CacheReadInputTokens,
			},
		}}
	case provider.StreamDeltaTypeMetadata:
		data, ok := delta.Data.(provider.MetadataData)
		if !ok {
			return nil
		}
		// 元数据不产生即时事件，累积到 sc.metadata，由 handleStop 在 message_delta 中统一输出。
		// 多次 MetadataDelta 时采用"后值覆盖"策略（与 provider 层语义一致）。
		if sc.metadata == nil {
			sc.metadata = &MessageDelta{}
		}
		if data.ResponseID != "" {
			sc.metadata.ResponseID = data.ResponseID
		}
		if data.ReasoningID != "" {
			sc.metadata.ReasoningID = data.ReasoningID
		}
		if data.EncryptedContent != "" {
			sc.metadata.EncryptedContent = data.EncryptedContent
		}
		return nil
	case provider.StreamDeltaTypeRedactedThinking:
		// redacted_thinking 作为独立内容块输出: 先关闭异类活跃块，再发出 block_start + block_stop。
		// redacted_thinking 是原子块（无增量），直接 start + stop 完成生命周期。
		rtData, ok := delta.Data.(provider.RedactedThinkingData)
		if !ok {
			return nil
		}
		var events []StreamEvent
		events = append(events, sc.stopAllTextOrThinking()...)
		idx := sc.nextIndex()
		events = append(events, StreamEvent{
			Type:         EventContentBlockStart,
			Index:        idx,
			ContentBlock: NewRedactedThinkingBlock(string(rtData)),
		})
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: idx,
		})
		sc.stoppedBlockIndexes[idx] = true
		return events
	default:
		return nil
	}
}

// handleBlockStop 处理 BlockStop delta，立即输出 content_block_stop 事件。
//
// 当 Provider 发出 BlockStop delta（如 Anthropic 的 content_block_stop）时，
// 立即关闭对应的活跃 block，无需等待 handleStop 统一关闭。
// 通过 stoppedBlockIndexes 集合防止与 handleStop 产生重复的 content_block_stop。
func (sc *StreamConverter) handleBlockStop(data provider.BlockStopData) []StreamEvent {
	if !data.HasIndex {
		return nil
	}
	index := data.Index
	if sc.stoppedBlockIndexes[index] {
		return nil
	}

	var events []StreamEvent

	// 关闭该 index 对应的活跃 block
	// 1. tool block
	for i, idx := range sc.openToolBlockIndexes {
		if idx == index {
			events = append(events, StreamEvent{Type: EventContentBlockStop, Index: idx})
			sc.openToolBlockIndexes = append(sc.openToolBlockIndexes[:i], sc.openToolBlockIndexes[i+1:]...)
			break
		}
	}
	// 2. text block
	if sc.textBlockStarted && sc.textBlockIndex == index {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: index})
	}
	// 3. thinking block
	if sc.thinkingBlockStarted && sc.thinkingBlockIndex == index {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: index})
	}

	sc.stoppedBlockIndexes[index] = true
	return events
}

func (sc *StreamConverter) handleStop() []StreamEvent {
	sc.stopHandled = true
	usage := sc.usage
	if usage == nil {
		usage = &Usage{}
	}
	// 使用适配器提供的完成原因，若未提供则根据流状态推断。
	// 关键兜底：当流被中断（stream.Err）但已收到 tool_call 增量时，
	// openToolBlockIndexes 非空 → 推断为 tool_use，避免 agent loop 误判为正常结束。
	stopReason := sc.finishReason
	if stopReason == "" {
		if len(sc.openToolBlockIndexes) > 0 {
			stopReason = FinishReasonToolUse
		} else {
			stopReason = FinishReasonEndTurn
		}
	}

	var events []StreamEvent
	if sc.thinkingBlockStarted && !sc.stoppedBlockIndexes[sc.thinkingBlockIndex] {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: sc.thinkingBlockIndex})
	}
	if sc.textBlockStarted && !sc.stoppedBlockIndexes[sc.textBlockIndex] {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: sc.textBlockIndex})
	}
	for _, idx := range sc.openToolBlockIndexes {
		if !sc.stoppedBlockIndexes[idx] {
			events = append(events, StreamEvent{Type: EventContentBlockStop, Index: idx})
		}
	}
	sc.openToolBlockIndexes = nil
	sc.toolBlockStarted = false
	sc.toolBlockByProviderIndex = nil
	sc.toolBlockByID = nil

	events = append(events,
		StreamEvent{Type: EventMessageDelta, Delta: sc.buildMessageDelta(stopReason), Usage: usage},
		StreamEvent{Type: EventMessageStop},
	)
	return events
}

// buildMessageDelta 构造 message_delta 事件携带的 MessageDelta，
// 合并 stopReason 和流式过程中累积的 metadata（ResponseID/ReasoningID/EncryptedContent）。
func (sc *StreamConverter) buildMessageDelta(stopReason FinishReason) *MessageDelta {
	md := &MessageDelta{StopReason: stopReason}
	if sc.metadata != nil {
		md.ResponseID = sc.metadata.ResponseID
		md.ReasoningID = sc.metadata.ReasoningID
		md.EncryptedContent = sc.metadata.EncryptedContent
	}
	return md
}

func mapBlockType(blockType string) ContentBlockType {
	switch blockType {
	case "text":
		return ContentBlockText
	case "thinking":
		return ContentBlockThinking
	case "redacted_thinking":
		return ContentBlockRedactedThinking
	case "tool_use":
		return ContentBlockToolUse
	default:
		return ContentBlockText
	}
}

func (sc *StreamConverter) handleError(err *pkgErrors.BambooError) []StreamEvent {
	events := []StreamEvent{{
		Type:  EventError,
		Error: err,
	}}

	// 流中断兜底：若尚未发出终止序列，自动补发。
	// 参考 Vercel AI SDK flush 机制 — 无论流是否出错，客户端都必须收到完整终止信号。
	if sc.started && !sc.stopHandled {
		events = append(events, sc.handleStop()...)
	}
	return events
}

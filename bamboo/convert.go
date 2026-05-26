package bamboo

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bamboo-services/bamboo-messages/internal/provider"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
)

// finishReasonMap provider.FinishReason → bamboo.FinishReason 映射表。
var finishReasonMap = map[provider.FinishReason]FinishReason{
	provider.FinishReasonStop:      FinishReasonEndTurn,
	provider.FinishReasonLength:    FinishReasonMaxTokens,
	provider.FinishReasonToolCalls: FinishReasonToolUse,
}

// messagesToProvider 将 bamboo.BambooMessage 列表转换为 provider.Message 列表。
// tool_result 消息会被拆分为独立的 Role=tool 消息；image/document 当前不支持。
func messagesToProvider(msgs []BambooMessage) ([]provider.Message, error) {
	var result []provider.Message
	for _, msg := range msgs {
		if len(msg.Content) == 0 {
			return nil, NewBambooError(ErrorTypeInvalidRequest, "content cannot be empty")
		}

		var textBuilder strings.Builder
		var toolCalls []provider.ToolCall
		var toolResults []provider.Message

		for _, block := range msg.Content {
			switch block.Type {
			case ContentBlockText:
				textBuilder.WriteString(block.Text)
			case ContentBlockThinking:
				// 思考过程不发送给 provider
			case ContentBlockToolUse:
				toolCalls = append(toolCalls, provider.ToolCall{
					ID:   block.ID,
					Type: "function",
					Function: provider.FunctionCall{
						Name:      block.Name,
						Arguments: string(block.Input),
					},
				})
			case ContentBlockToolResult:
				toolResults = append(toolResults, provider.Message{
					Role:       provider.RoleTool,
					Content:    block.ResultContent,
					ToolCallID: block.ToolUseID,
				})
			case ContentBlockImage:
				return nil, NewBambooError(ErrorTypeProvider, "image content not supported by this provider")
			case ContentBlockDocument:
				return nil, NewBambooError(ErrorTypeProvider, "document content not supported by this provider")
			}
		}

		content := textBuilder.String()
		if content != "" || len(toolCalls) > 0 {
			result = append(result, provider.Message{
				Role:      providerRole(msg.Role),
				Content:   content,
				ToolCalls: toolCalls,
			})
		}
		result = append(result, toolResults...)
	}
	return result, nil
}

func providerRole(role MessageRole) provider.MessageRole {
	switch role {
	case RoleUser:
		return provider.RoleUser
	case RoleAssistant:
		return provider.RoleAssistant
	default:
		return provider.RoleUser
	}
}

// configToProvider 将 bamboo.RequestConfig 转换为 provider.ChatConfig。
func configToProvider(cfg *RequestConfig) *provider.ChatConfig {
	if cfg == nil {
		return nil
	}
	return &provider.ChatConfig{
		Model:         cfg.Model,
		MaxTokens:     cfg.MaxTokens,
		Temperature:   cfg.Temperature,
		TopP:          cfg.TopP,
		Stop:          cfg.StopSequences,
		Tools:         toolsToProvider(cfg.Tools),
		Metadata:      cfg.Metadata,
		ThinkingConfig: cfg.ThinkingConfig,
		ProviderExtra: cfg.ProviderExtra,
	}
}

// toolsToProvider 将 bamboo.Tool 列表转换为 provider.Tool 列表。
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
		})
	}
	return result
}

func buildParameters(schema InputSchema) map[string]any {
	params := map[string]any{"type": schema.Type}
	if len(schema.Properties) > 0 {
		props := make(map[string]any, len(schema.Properties))
		for name, def := range schema.Properties {
			prop := map[string]any{"type": def.Type}
			if def.Description != "" {
				prop["description"] = def.Description
			}
			if len(def.Enum) > 0 {
				prop["enum"] = def.Enum
			}
			if def.Items != nil {
				prop["items"] = def.Items
			}
			props[name] = prop
		}
		params["properties"] = props
	}
	if len(schema.Required) > 0 {
		params["required"] = schema.Required
	}
	return params
}

// resultToResponse 将 provider.CompletionResult 转换为 bamboo.Response。
func resultToResponse(result *provider.CompletionResult, providerType string) *Response {
	if result == nil {
		return nil
	}
	var content []ContentBlock
	if result.Content != "" {
		content = append(content, NewTextBlock(result.Content))
	}
	for _, tc := range result.ToolCalls {
		content = append(content, ContentBlock{
			Type:  ContentBlockToolUse,
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	if len(content) == 0 {
		content = []ContentBlock{}
	}
	return &Response{
		ID:           fmt.Sprintf("bamboo_msg_%d", time.Now().UnixNano()),
		Type:         "message",
		Role:         RoleAssistant,
		Content:      content,
		StopReason:   mapFinishReason(result.FinishReason),
		Usage:        Usage{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens},
		ProviderType: providerType,
		RequestID:    fmt.Sprintf("req_%d", time.Now().UnixNano()),
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
	blockIndex       int
	usage            *Usage
	started          bool
	textBlockStarted bool
	thinkingBlockStarted bool
}

func NewStreamConverter() *StreamConverter { return &StreamConverter{} }

// Convert 将单个 provider.StreamEvent 转换为 bamboo.StreamEvent 列表。
func (sc *StreamConverter) Convert(event provider.StreamEvent) []StreamEvent {
	switch event.Type {
	case provider.StreamTypeStart:
		return sc.handleStart()
	case provider.StreamTypeDelta:
		return sc.handleDelta(event.Delta)
	case provider.StreamTypeStop:
		return sc.handleStop()
	case provider.StreamTypeDone:
		return nil
	case provider.StreamTypeError:
		return sc.handleError(event.Err)
	default:
		return nil
	}
}

func (sc *StreamConverter) handleStart() []StreamEvent {
	sc.started = true
	sc.blockIndex = 0
	return []StreamEvent{
		{
			Type:    EventMessageStart,
			Message: &BambooMessage{Role: RoleAssistant, Content: []ContentBlock{}},
			Usage:   &Usage{},
		},
	}
}

func (sc *StreamConverter) handleDelta(delta provider.StreamDelta[any]) []StreamEvent {
	switch delta.Type {
	case provider.StreamDeltaTypeBlockStart:
		data := delta.Data.(provider.BlockStartData)
		if data.BlockType == "text" {
			sc.textBlockStarted = true
		}
		if data.BlockType == "thinking" {
			sc.thinkingBlockStarted = true
		}
		return []StreamEvent{
			{
				Type:         EventContentBlockStart,
				Index:        sc.blockIndex,
				ContentBlock: &ContentBlock{Type: mapBlockType(data.BlockType), ID: data.ID, Name: data.Name},
			},
		}
	case provider.StreamDeltaTypeTextOutput:
		// 防御性：若 provider 未发送 block_start，自动补发
		var events []StreamEvent
		if !sc.textBlockStarted {
			sc.textBlockStarted = true
			events = append(events, StreamEvent{
				Type:         EventContentBlockStart,
				Index:        sc.blockIndex,
				ContentBlock: &ContentBlock{Type: ContentBlockText},
			})
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.blockIndex,
			Delta: &StreamDelta{Type: DeltaTextDelta, Text: string(delta.Data.(provider.TextData))},
		})
		return events
	case provider.StreamDeltaTypeThinking:
		var events []StreamEvent
		if !sc.thinkingBlockStarted {
			sc.thinkingBlockStarted = true
			events = append(events, StreamEvent{
				Type:         EventContentBlockStart,
				Index:        sc.blockIndex,
				ContentBlock: &ContentBlock{Type: ContentBlockThinking},
			})
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.blockIndex,
			Delta: &StreamDelta{Type: DeltaThinkingDelta, Thinking: string(delta.Data.(provider.ThinkingData))},
		})
		return events
	case provider.StreamDeltaTypeToolCall:
		data := delta.Data.(provider.ToolCallData)
		stopIdx := sc.blockIndex
		sc.blockIndex++
		return []StreamEvent{
			{Type: EventContentBlockStop, Index: stopIdx},
			{
				Type:         EventContentBlockStart,
				Index:        sc.blockIndex,
				ContentBlock: &ContentBlock{Type: ContentBlockToolUse, ID: data.ID, Name: data.Name},
			},
		}
	case provider.StreamDeltaTypeToolCallDelta:
		return []StreamEvent{{
			Type:  EventContentBlockDelta,
			Index: sc.blockIndex,
			Delta: &StreamDelta{Type: DeltaInputJSON, PartialJSON: string(delta.Data.(provider.ToolCallDeltaData))},
		}}
	case provider.StreamDeltaTypeUsage:
		data := delta.Data.(provider.UsageData)
		sc.usage = &Usage{InputTokens: data.InputTokens, OutputTokens: data.OutputTokens}
		return nil
	default:
		return nil
	}
}

func (sc *StreamConverter) handleStop() []StreamEvent {
	usage := sc.usage
	if usage == nil {
		usage = &Usage{}
	}
	return []StreamEvent{
		{Type: EventContentBlockStop, Index: sc.blockIndex},
		{Type: EventMessageDelta, Delta: &MessageDelta{StopReason: FinishReasonEndTurn}, Usage: usage},
		{Type: EventMessageStop},
	}
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

package openai

import (
	"context"

	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ResponsesProvider OpenAI (GPT) AI 服务提供商实现，基于 Responses API
type ResponsesProvider provider.BaseProvider[openai.Client]

// NewResponsesProvider 创建 OpenAI Responses Provider 实例
func NewResponsesProvider(apiKey string) *ResponsesProvider {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &ResponsesProvider{
		Client: client,
	}
}

// Chat 流式对话
func (p *ResponsesProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话
func (p *ResponsesProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 发送流开始事件
		eventCh <- provider.StreamEvent{
			Type: provider.StreamTypeStart,
		}

		params := responses.ResponseNewParams{
			Model: config.Model,
			Input: p.buildInput(systemPrompt, messages),
		}

		if config.MaxTokens > 0 {
			params.MaxOutputTokens = openai.Int(config.MaxTokens)
		}

		stream := p.Client.Responses.NewStreaming(ctx, params)

		for stream.Next() {
			event := stream.Current()
			events := p.handleStreamEvent(event)
			for _, e := range events {
				eventCh <- e
			}
		}

		if err := stream.Err(); err != nil {
			eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  xError.NewError(ctx, xError.OperationFailed, "OpenAI 流式对话失败", false, err),
			}
			return
		}

		// 发送完成事件
		eventCh <- provider.StreamEvent{
			Type: provider.StreamTypeDone,
		}
	}()

	return eventCh
}

// GetProviderType 获取提供商类型
func (p *ResponsesProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAIResponses
}

// GetAvailableModels 获取可用模型列表
func (p *ResponsesProvider) GetAvailableModels() []string {
	return []string{
		openai.ChatModelGPT4o,
		openai.ChatModelGPT4oMini,
		openai.ChatModelGPT4_1,
		openai.ChatModelGPT4_1Mini,
		openai.ChatModelGPT4_1Nano,
		openai.ChatModelO3,
		openai.ChatModelO3Mini,
		openai.ChatModelO4Mini,
	}
}

// ==============================
// 内部方法
// ==============================

// buildInput 将内部消息格式转换为 OpenAI Responses API 输入格式
func (p *ResponsesProvider) buildInput(systemPrompt string, messages []provider.Message) responses.ResponseNewParamsInputUnion {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(messages)+1)

	if systemPrompt != "" {
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleSystem,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(systemPrompt),
				},
			},
		})
	}

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			items = append(items, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(msg.Content),
					},
				},
			})
		case provider.RoleAssistant:
			items = append(items, p.buildAssistantItem(msg))
		case provider.RoleTool:
			items = append(items, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: msg.ToolCallID,
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: openai.String(msg.Content),
					},
				},
			})
		}
	}

	return responses.ResponseNewParamsInputUnion{
		OfInputItemList: items,
	}
}

// buildAssistantItem 构建助手消息项（支持文本和工具调用）
func (p *ResponsesProvider) buildAssistantItem(msg provider.Message) responses.ResponseInputItemUnionParam {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(msg.ToolCalls)+1)

	if msg.Content != "" {
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleAssistant,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(msg.Content),
				},
			},
		})
	}

	for _, tc := range msg.ToolCalls {
		items = append(items, responses.ResponseInputItemUnionParam{
			OfFunctionCall: &responses.ResponseFunctionToolCallParam{
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	if len(items) == 1 {
		return items[0]
	}
	return items[0]
}

// ==============================
// 流式事件处理
// ==============================

// handleStreamEvent 根据事件类型分发到对应的处理方法
func (p *ResponsesProvider) handleStreamEvent(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	switch event.Type {
	case "response.created":
		return p.contentResponseCreated(event)
	case "response.output_item.added":
		return p.contentOutputItemAdded(event)
	case "response.output_text.delta":
		return p.contentOutputTextDelta(event)
	case "response.reasoning_text.delta":
		return p.contentReasoningTextDelta(event)
	case "response.function_call_arguments.delta":
		return p.contentFunctionCallDelta(event)
	case "response.function_call_arguments.done":
		return p.contentFunctionCallDone(event)
	case "response.completed":
		return p.contentResponseCompleted(event)
	case "response.failed":
		return p.contentResponseFailed(event)
	case "response.incomplete":
		return p.contentResponseIncomplete(event)
	default:
		return nil
	}
}

// contentResponseCreated 处理响应创建事件
func (p *ResponsesProvider) contentResponseCreated(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 响应创建，已在 ChatWithSystem 中发送 StreamTypeStart
	return nil
}

// contentOutputItemAdded 处理输出项添加事件
func (p *ResponsesProvider) contentOutputItemAdded(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseOutputItemAdded()
	switch e.Item.Type {
	case "function_call":
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(e.Item.ID, e.Item.Name),
		}}
	default:
		return nil
	}
}

// contentOutputTextDelta 处理文本输出增量事件
func (p *ResponsesProvider) contentOutputTextDelta(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseOutputTextDelta()
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta(e.Delta),
	}}
}

// contentReasoningTextDelta 处理推理文本增量事件
func (p *ResponsesProvider) contentReasoningTextDelta(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseReasoningTextDelta()
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewThinkingDelta(e.Delta),
	}}
}

// contentFunctionCallDelta 处理函数调用参数增量事件
func (p *ResponsesProvider) contentFunctionCallDelta(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseFunctionCallArgumentsDelta()
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaData(e.Delta),
	}}
}

// contentFunctionCallDone 处理函数调用完成事件
func (p *ResponsesProvider) contentFunctionCallDone(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 函数调用完成，无需特殊处理
	return nil
}

// contentResponseCompleted 处理响应完成事件（包含 usage）
func (p *ResponsesProvider) contentResponseCompleted(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseCompleted()
	usage := e.Response.Usage
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewUsageDelta(usage.InputTokens, usage.OutputTokens),
		}}
	}
	return nil
}

// contentResponseFailed 处理响应失败事件
func (p *ResponsesProvider) contentResponseFailed(event responses.ResponseStreamEventUnion) []provider.StreamEvent {
	e := event.AsResponseFailed()
	errMsg := "OpenAI 响应失败"
	if e.Response.Error.Message != "" {
		errMsg += ": " + e.Response.Error.Message
	}
	return []provider.StreamEvent{{
		Type: provider.StreamTypeError,
		Err:  xError.NewError(context.TODO(), xError.OperationFailed, xError.ErrMessage(errMsg), false, nil),
	}}
}

// contentResponseIncomplete 处理响应未完成事件
func (p *ResponsesProvider) contentResponseIncomplete(_ responses.ResponseStreamEventUnion) []provider.StreamEvent {
	// 响应未完成，发送停止事件
	return []provider.StreamEvent{{
		Type: provider.StreamTypeStop,
	}}
}

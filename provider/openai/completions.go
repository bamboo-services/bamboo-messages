package openai

import (
	"context"

	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// CompletionsProvider OpenAI (GPT) AI 服务提供商实现，基于 Chat Completions API
type CompletionsProvider provider.BaseProvider[openai.Client]

// NewCompletionsProvider 创建 OpenAI Completions Provider 实例
func NewCompletionsProvider(apiKey string) *CompletionsProvider {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &CompletionsProvider{
		Client: client,
	}
}

// Chat 流式对话
func (p *CompletionsProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话
func (p *CompletionsProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 发送流开始事件
		eventCh <- provider.StreamEvent{
			Type: provider.StreamTypeStart,
		}

		params := openai.ChatCompletionNewParams{
			Model:    config.Model,
			Messages: p.buildMessages(systemPrompt, messages),
		}

		if config.MaxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(config.MaxTokens)
		}

		if config.Temperature != nil {
			params.Temperature = openai.Float(*config.Temperature)
		}

		if config.TopP != nil {
			params.TopP = openai.Float(*config.TopP)
		}

		// 启用 usage 流式返回
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}

		stream := p.Client.Chat.Completions.NewStreaming(ctx, params)

		for stream.Next() {
			chunk := stream.Current()
			events := p.handleChunk(chunk)
			for _, e := range events {
				eventCh <- e
			}
		}

		if err := stream.Err(); err != nil {
			eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  xError.NewError(ctx, xError.OperationFailed, "OpenAI Completions 流式对话失败", false, err),
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
func (p *CompletionsProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAICompletions
}

// GetAvailableModels 获取可用模型列表
func (p *CompletionsProvider) GetAvailableModels() []string {
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

// buildMessages 将内部消息格式转换为 OpenAI Chat Completions API 消息格式
func (p *CompletionsProvider) buildMessages(systemPrompt string, messages []provider.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)

	if systemPrompt != "" {
		result = append(result, openai.SystemMessage(systemPrompt))
	}

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			result = append(result, openai.UserMessage(msg.Content))
		case provider.RoleAssistant:
			result = append(result, p.buildAssistantMessage(msg))
		case provider.RoleTool:
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	return result
}

// buildAssistantMessage 构建助手消息（支持文本和工具调用）
func (p *CompletionsProvider) buildAssistantMessage(msg provider.Message) openai.ChatCompletionMessageParamUnion {
	assistantMsg := openai.ChatCompletionAssistantMessageParam{
		ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{},
	}

	if msg.Content != "" {
		assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: param.NewOpt(msg.Content),
		}
	}

	for _, tc := range msg.ToolCalls {
		assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: tc.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			},
		})
	}

	return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg}
}

// ==============================
// 流式事件处理
// ==============================

// handleChunk 处理单个 ChatCompletionChunk，提取 delta 数据转换为统一事件
func (p *CompletionsProvider) handleChunk(chunk openai.ChatCompletionChunk) []provider.StreamEvent {
	var events []provider.StreamEvent

	// 处理 usage（最后一个 chunk 可能没有 choices）
	if chunk.Usage.TotalTokens > 0 {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewUsageDelta(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens),
		})
	}

	// 处理 choices
	for _, choice := range chunk.Choices {
		events = append(events, p.handleChoice(choice)...)
	}

	return events
}

// handleChoice 处理单个 choice 的 delta 数据
func (p *CompletionsProvider) handleChoice(choice openai.ChatCompletionChunkChoice) []provider.StreamEvent {
	delta := choice.Delta
	var events []provider.StreamEvent

	// 文本内容增量
	if delta.Content != "" {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta(delta.Content),
		})
	}

	// 工具调用增量
	for _, tc := range delta.ToolCalls {
		events = append(events, p.handleToolCallDelta(tc)...)
	}

	// 处理完成原因
	if choice.FinishReason == "stop" {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeStop,
		})
	}

	return events
}

// handleToolCallDelta 处理工具调用增量数据
func (p *CompletionsProvider) handleToolCallDelta(tc openai.ChatCompletionChunkChoiceDeltaToolCall) []provider.StreamEvent {
	var events []provider.StreamEvent

	// 当 ID 存在时表示新的工具调用开始
	if tc.ID != "" {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(tc.ID, tc.Function.Name),
		})
	}

	// 参数增量
	if tc.Function.Arguments != "" {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDeltaData(tc.Function.Arguments),
		})
	}

	return events
}

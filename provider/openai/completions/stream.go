package completions

import (
	"github.com/openai/openai-go/v3"
	"github.com/bamboo-services/bamboo-messages/provider"
)

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

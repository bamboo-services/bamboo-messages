package completions

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
)

// handleChunk 处理单个 ChatCompletionChunk，提取 delta 数据转换为统一事件。
//
// 解析 usage 和 choices 数据，调用 handleChoice 处理每个 choice。
func (p *CompletionsProvider) handleChunk(chunk openai.ChatCompletionChunk, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	var events []provider.StreamEvent

	// 处理 usage（最后一个 chunk 可能没有 choices）
	if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				chunk.Usage.PromptTokens,
				chunk.Usage.CompletionTokens,
				0,
				chunk.Usage.PromptTokensDetails.CachedTokens,
			),
		})
	}

	// 处理 choices
	for _, choice := range chunk.Choices {
		events = append(events, p.handleChoice(choice, textBlockStarted, thinkingBlockStarted)...)
	}

	return events
}

// handleChoice 处理单个 choice 的 delta 数据。
//
// 提取 reasoning_content、文本内容和工具调用增量，合成 BlockStart 事件。
func (p *CompletionsProvider) handleChoice(choice openai.ChatCompletionChunkChoice, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	delta := choice.Delta
	var events []provider.StreamEvent

	// 推理内容增量 — 从 ExtraFields 提取 reasoning_content
	if field, ok := delta.JSON.ExtraFields["reasoning_content"]; ok && field.Raw() != "" {
		var reasoning string
		if err := json.Unmarshal([]byte(field.Raw()), &reasoning); err == nil && reasoning != "" {
			if !*thinkingBlockStarted {
				events = append(events, provider.StreamEvent{
					Type:  provider.StreamTypeDelta,
					Delta: provider.NewBlockStartDelta("thinking"),
				})
				*thinkingBlockStarted = true
			}
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewThinkingDelta(reasoning),
			})
		}
	}

	// 文本内容增量
	if delta.Content != "" {
		// OpenAI Completions 没有显式的 content_block_start 事件，我们在第一次文本增量前合成它
		if !*textBlockStarted {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("text"),
			})
			*textBlockStarted = true
		}
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
	if choice.FinishReason != "" {
		events = append(events, provider.StreamEvent{
			Type:         provider.StreamTypeStop,
			FinishReason: mapFinishReason(choice.FinishReason),
		})
	}

	return events
}

// handleToolCallDelta 处理工具调用增量数据。
//
// 提取工具 ID、函数名称和参数增量，转换为统一的 ToolCallDelta 事件。
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

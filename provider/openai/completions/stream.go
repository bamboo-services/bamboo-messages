package completions

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
)

// handleChunk 处理单个 ChatCompletionChunk，提取 delta 数据转换为统一事件。
func (p *CompletionsProvider) handleChunk(chunk openai.ChatCompletionChunk, textBlockStarted *bool, thinkingBlockStarted *bool, stopSent *bool) []provider.StreamEvent {
	var events []provider.StreamEvent

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

	for _, choice := range chunk.Choices {
		events = append(events, p.handleChoice(choice, textBlockStarted, thinkingBlockStarted, stopSent)...)
	}

	return events
}

func (p *CompletionsProvider) handleChoice(choice openai.ChatCompletionChunkChoice, textBlockStarted *bool, thinkingBlockStarted *bool, stopSent *bool) []provider.StreamEvent {
	delta := choice.Delta
	var events []provider.StreamEvent

	// 推理内容提取：兼容 reasoning_content（DeepSeek/智谱等）和 reasoning（xAI/Grok 等）两种字段名。
	// 参考 Vercel AI SDK: delta.reasoning_content ?? delta.reasoning
	reasoningRaw := ""
	if field, ok := delta.JSON.ExtraFields["reasoning_content"]; ok && field.Raw() != "" {
		reasoningRaw = field.Raw()
	} else if field, ok := delta.JSON.ExtraFields["reasoning"]; ok && field.Raw() != "" {
		reasoningRaw = field.Raw()
	}
	if reasoningRaw != "" {
		var reasoning string
		if err := json.Unmarshal([]byte(reasoningRaw), &reasoning); err == nil && reasoning != "" {
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

	if delta.Content != "" {
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

	for _, tc := range delta.ToolCalls {
		events = append(events, p.handleToolCallDelta(tc)...)
	}

	if choice.FinishReason != "" {
		*stopSent = true
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
			Delta: provider.NewToolCallDeltaWithIndex(tc.ID, tc.Function.Name, int(tc.Index)),
		})
	}

	// 参数增量
	if tc.Function.Arguments != "" {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDeltaDataWithIndex(tc.Function.Arguments, int(tc.Index)),
		})
	}

	return events
}

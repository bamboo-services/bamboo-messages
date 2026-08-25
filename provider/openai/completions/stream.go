package completions

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// handleChunk 处理单个 chatCompletionChunk，提取 delta 数据转换为统一事件。
//
// 使用 types.go 中定义的 DTO 结构体，不依赖 openai-go SDK。
// stripper 非 nil 时，content 增量先经内联 think 标签剥离（见 WithStripThinkTags）。
func (p *CompletionsProvider) handleChunk(chunk chatCompletionChunk, textBlockStarted *bool, thinkingBlockStarted *bool, stopSent *bool, stripper *thinkTagStripper) []provider.StreamEvent {
	var events []provider.StreamEvent

	// Usage 提取：任一非零字段即触发（兼容 TotalTokens=0 但 PromptTokens>0 的场景）
	if chunk.Usage != nil &&
		(chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0) {
		var cached int
		if chunk.Usage.PromptTokensDetails != nil {
			cached = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				int64(chunk.Usage.PromptTokens),
				int64(chunk.Usage.CompletionTokens),
				0,
				int64(cached),
			),
		})
	}

	for _, choice := range chunk.Choices {
		events = append(events, p.handleChoice(choice, textBlockStarted, thinkingBlockStarted, stopSent, stripper)...)
	}

	return events
}

// handleChoice 处理单个 choice 的 delta + FinishReason。
//
// 提取顺序：reasoning_content → content → tool_calls → finish_reason。
// textBlockStarted / thinkingBlockStarted 独立追踪，互不干扰。
func (p *CompletionsProvider) handleChoice(choice chatCompletionChunkChoice, textBlockStarted *bool, thinkingBlockStarted *bool, stopSent *bool, stripper *thinkTagStripper) []provider.StreamEvent {
	delta := choice.Delta
	var events []provider.StreamEvent

	// 推理内容提取：兼容 reasoning_content（DeepSeek/智谱等）和 reasoning（xAI/Grok 等）两种字段名。
	// 参考 Vercel AI SDK: delta.reasoning_content ?? delta.reasoning
	reasoningStr := parseReasoningRaw(delta.ReasoningContent)
	if reasoningStr == "" {
		reasoningStr = parseReasoningRaw(delta.Reasoning)
	}
	if reasoningStr != "" {
		if !*thinkingBlockStarted {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("thinking"),
			})
			*thinkingBlockStarted = true
		}
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(reasoningStr),
		})
	}
	if delta.ThinkingSignature != "" {
		if !*thinkingBlockStarted {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("thinking"),
			})
			*thinkingBlockStarted = true
		}
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewSignatureDeltaWithProvider(delta.ThinkingSignature, delta.ThinkingProvider),
		})
	}

	if delta.Content != "" {
		if stripper != nil {
			// 内联 think 标签剥离模式：content 经状态机扫描后可能产生
			// thinking / text 混合事件序列，BlockStart 契约由 syncBlockState 维护
			stripped := stripper.process(delta.Content)
			events = append(events, syncBlockState(stripped, textBlockStarted, thinkingBlockStarted)...)
		} else {
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
	}

	for _, tc := range delta.ToolCalls {
		events = append(events, p.handleToolCallDelta(tc)...)
	}

	if choice.FinishReason != nil && *choice.FinishReason != "" {
		*stopSent = true
		events = append(events, provider.StreamEvent{
			Type:         provider.StreamTypeStop,
			FinishReason: mapFinishReason(*choice.FinishReason),
		})
	}

	return events
}

// syncBlockState 将剥离器产生的事件序列与适配器 BlockStart 状态同步。
//
// 剥离器自身仅发出 BlockStart(thinking)，不发出 BlockStart(text)。
// 此函数负责：
//   - 剥离器发出 BlockStart(thinking) 时同步 thinkingBlockStarted 标志，
//     避免后续 reasoning_content 增量重复合成块开始事件
//   - 首个 text_output 增量前合成 BlockStart(text)，保持与常规路径一致的
//     BlockStart 契约（所有适配器在首个文本增量前必须发出 BlockStart）
func syncBlockState(stripped []provider.StreamEvent, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	var events []provider.StreamEvent
	for _, e := range stripped {
		switch e.Delta.Type {
		case provider.StreamDeltaTypeBlockStart:
			*thinkingBlockStarted = true
		case provider.StreamDeltaTypeTextOutput:
			if !*textBlockStarted {
				events = append(events, provider.StreamEvent{
					Type:  provider.StreamTypeDelta,
					Delta: provider.NewBlockStartDelta("text"),
				})
				*textBlockStarted = true
			}
		}
		events = append(events, e)
	}
	return events
}

// handleToolCallDelta 处理工具调用增量数据。
//
// 提取工具 ID、函数名称和参数增量，转换为统一的 ToolCallDelta 事件。
// 当 Index 字段存在时使用带索引版本（支持 OpenAI 并行工具调用）。
func (p *CompletionsProvider) handleToolCallDelta(tc chunkDeltaToolCall) []provider.StreamEvent {
	var events []provider.StreamEvent

	// 当 ID 存在时表示新的工具调用开始
	if tc.ID != "" {
		if tc.Index != nil {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewToolCallDeltaWithIndex(tc.ID, tc.Function.Name, *tc.Index),
			})
		} else {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewToolCallDelta(tc.ID, tc.Function.Name),
			})
		}
	}

	// 参数增量
	if tc.Function.Arguments != "" {
		if tc.Index != nil {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewToolCallDeltaDataWithIndex(tc.Function.Arguments, *tc.Index),
			})
		} else {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewToolCallDeltaData(tc.Function.Arguments),
			})
		}
	}

	return events
}

// parseReasoningRaw 将 json.RawMessage 格式的推理内容解析为字符串。
//
// 兼容两种格式：
//   - 字符串格式：`"thinking text"` → 直接提取
//   - JSON 对象格式：`{"text": "..."}` 或 `{"content": "..."}` → 尝试提取 text/content 字段
//   - 解析失败时回退为原始 JSON 字符串
func parseReasoningRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// null 值直接跳过
	r := string(raw)
	if r == "null" {
		return ""
	}

	// 尝试作为字符串解析
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// 尝试作为 JSON 对象解析，提取 text 或 content 字段
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if text, ok := obj["text"].(string); ok && text != "" {
			return text
		}
		if content, ok := obj["content"].(string); ok && content != "" {
			return content
		}
	}

	// 回退：将原始 JSON 作为字符串返回
	return r
}

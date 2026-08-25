package gemini

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// handleStreamEvent 处理单个 Gemini generateContentResponse，提取增量数据转换为统一事件。
//
// 遍历 Candidates 的 Content.Parts，根据 Part 类型分发到对应处理函数。
// 通过 textBlockStarted / thinkingBlockStarted 两个独立标志合成 BlockStart 事件，
// 与 OpenAI Completions 适配器保持一致的模式。
func (p *Provider) handleStreamEvent(resp *generateContentResponse, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	if resp == nil {
		return nil
	}
	var events []provider.StreamEvent

	// 处理 Usage（最后一个 chunk 可能只携带 UsageMetadata）
	if resp.UsageMetadata != nil {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.StreamDelta[any]{
				Type: provider.StreamDeltaTypeUsage,
				Data: provider.UsageData{
					InputTokens:          int64(resp.UsageMetadata.PromptTokenCount),
					OutputTokens:         int64(resp.UsageMetadata.CandidatesTokenCount),
					CacheReadInputTokens: int64(resp.UsageMetadata.CachedContentTokenCount),
					ReasoningTokens:      int64(resp.UsageMetadata.ThoughtsTokenCount),
				},
			},
		})
	}

	// 处理 Candidates
	for i := range resp.Candidates {
		events = append(events, p.handleCandidate(&resp.Candidates[i], textBlockStarted, thinkingBlockStarted)...)
	}

	return events
}

// handleCandidate 处理单个 geminiCandidate 的 Content 数据。
//
// 遍历 Content.Parts 提取文本、推理、工具调用，合成 BlockStart 事件。
// 若 FinishReason 非空且不为 STOP，映射为统一的 FinishReason 并发送 Stop 事件。
func (p *Provider) handleCandidate(candidate *geminiCandidate, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	var events []provider.StreamEvent

	if candidate.Content != nil {
		for i := range candidate.Content.Parts {
			events = append(events, p.handlePart(&candidate.Content.Parts[i], textBlockStarted, thinkingBlockStarted)...)
		}
	}

	// 处理 FinishReason — 非空且不为 STOP/FINISH_REASON_STOP 表示生成结束
	if candidate.FinishReason != "" && candidate.FinishReason != "STOP" && candidate.FinishReason != "FINISH_REASON_STOP" {
		events = append(events, provider.StreamEvent{
			Type:         provider.StreamTypeStop,
			FinishReason: mapFinishReason(candidate.FinishReason),
		})
	}

	return events
}

// handlePart 处理单个 geminiPart。
//
// 根据 part.Thought、part.ThoughtSignature 和 part.FunctionCall 字段分发：
//   - Thought==true && Text!=""  → 推理增量（thinking block）
//   - ThoughtSignature!=""       → 思考签名增量（signature delta）
//   - Thought==false && Text!="" → 文本增量（text block）
//   - FunctionCall != nil        → 工具调用增量（tool_call + tool_call_delta）
//
// 注意：Gemini 在调用工具时，FunctionCall part 经常同时附带 ThoughtSignature。
// 不能使用互斥的 early return，必须确保 FunctionCall 与 ThoughtSignature 均被正常处理。
func (p *Provider) handlePart(part *geminiPart, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	if part == nil {
		return nil
	}
	var events []provider.StreamEvent

	// 1. 推理内容增量（Thought == true 且 Text != ""）
	if part.Thought && part.Text != "" {
		if !*thinkingBlockStarted {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("thinking"),
			})
			*thinkingBlockStarted = true
		}
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewThinkingDelta(part.Text),
		})
	}

	// 2. 思考签名增量（ThoughtSignature != ""）
	if part.ThoughtSignature != "" {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewSignatureDelta(part.ThoughtSignature),
		})
	}

	// 3. 文本内容增量（!Thought 且 Text != ""）
	if !part.Thought && part.Text != "" {
		if !*textBlockStarted {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("text"),
			})
			*textBlockStarted = true
		}
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewTextDelta(part.Text),
		})
	}

	// 4. 工具调用（FunctionCall != nil）
	// 不发送 BlockStartDelta，由 StreamConverter 的 ToolCall 处理自动管理 block 生命周期。
	// 与 Anthropic/OpenAI 适配器保持一致：仅发送 ToolCallDelta + ToolCallDeltaData。
	if part.FunctionCall != nil {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(part.FunctionCall.ID, part.FunctionCall.Name),
		})
		// Args 为 json.RawMessage，直接转为字符串传递
		argsStr := string(part.FunctionCall.Args)
		if argsStr == "" {
			argsStr = "{}"
		}
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDeltaData(argsStr),
		})
	}

	return events
}

// mapFinishReason 将 Gemini FinishReason 字符串映射为统一的 FinishReason。
//
// Gemini FinishReason 取值（REST API 字符串形式）：
//   - STOP / FINISH_REASON_STOP         → Stop（正常结束）
//   - MAX_TOKENS / FINISH_REASON_MAX_TOKENS → Length（达到最大长度）
//   - SAFETY / FINISH_REASON_SAFETY     → Stop（安全过滤，降级为正常结束）
//   - RECITATION / FINISH_REASON_RECITATION → Stop（引用过滤，降级为正常结束）
//   - 其他                              → Stop（默认降级）
func mapFinishReason(reason string) provider.FinishReason {
	switch reason {
	case "STOP", "FINISH_REASON_STOP":
		return provider.FinishReasonStop
	case "MAX_TOKENS", "FINISH_REASON_MAX_TOKENS":
		return provider.FinishReasonLength
	case "SAFETY", "FINISH_REASON_SAFETY", "RECITATION", "FINISH_REASON_RECITATION":
		return provider.FinishReasonStop
	default:
		return provider.FinishReasonStop
	}
}

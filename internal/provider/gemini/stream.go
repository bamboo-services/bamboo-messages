package gemini

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/internal/provider"
	"google.golang.org/genai"
)

// handleStreamEvent 处理单个 Gemini GenerateContentResponse，提取增量数据转换为统一事件。
//
// 遍历 Candidates[0].Content.Parts，根据 Part 类型分发到对应处理函数。
// 通过 textBlockStarted / thinkingBlockStarted 两个独立标志合成 BlockStart 事件，
// 与 OpenAI Completions 适配器保持一致的模式。
func (p *Provider) handleStreamEvent(resp *genai.GenerateContentResponse, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	if resp == nil {
		return nil
	}
	var events []provider.StreamEvent

	// 处理 Usage（最后一个 chunk 可能只携带 UsageMetadata）
	if resp.UsageMetadata != nil {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDelta(
				int64(resp.UsageMetadata.PromptTokenCount),
				int64(resp.UsageMetadata.CandidatesTokenCount),
			),
		})
	}

	// 处理 Candidates
	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		events = append(events, p.handleCandidate(candidate, textBlockStarted, thinkingBlockStarted)...)
	}

	return events
}

// handleCandidate 处理单个 Candidate 的 Content 数据。
//
// 遍历 Content.Parts 提取文本、推理、工具调用，合成 BlockStart 事件。
func (p *Provider) handleCandidate(candidate *genai.Candidate, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	var events []provider.StreamEvent

	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			events = append(events, p.handlePart(part, textBlockStarted, thinkingBlockStarted)...)
		}
	}

	// 处理 FinishReason — 非空表示生成结束
	if candidate.FinishReason != "" && candidate.FinishReason != genai.FinishReasonUnspecified {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeStop,
		})
	}

	return events
}

// handlePart 处理单个 Part。
//
// 根据 part.Thought 和 part.FunctionCall 字段分发：
//   - Thought==true && Text!="" → 推理增量（thinking block）
//   - Thought==false && Text!="" → 文本增量（text block）
//   - FunctionCall != nil → 工具调用增量（tool_call + tool_call_delta）
func (p *Provider) handlePart(part *genai.Part, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	var events []provider.StreamEvent

	// 推理内容增量（Gemini 的 Thought 标志）
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
		return events
	}

	// 文本内容增量
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
		return events
	}

	// 工具调用（FunctionCall）
	if part.FunctionCall != nil {
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStartDeltaWithID("tool_use", part.FunctionCall.ID, part.FunctionCall.Name),
		})
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(part.FunctionCall.ID, part.FunctionCall.Name),
		})
		argsBytes, _ := json.Marshal(part.FunctionCall.Args)
		events = append(events, provider.StreamEvent{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDeltaData(string(argsBytes)),
		})
		return events
	}

	return nil
}

// mapFinishReason 将 Gemini FinishReason 映射为统一的 FinishReason。
//
// Gemini: STOP → Stop, MAX_TOKENS → Length, SAFETY → Stop（降级），其他默认为 Stop。
func mapFinishReason(reason genai.FinishReason) provider.FinishReason {
	switch reason {
	case genai.FinishReasonStop:
		return provider.FinishReasonStop
	case genai.FinishReasonMaxTokens:
		return provider.FinishReasonLength
	case genai.FinishReasonSafety, genai.FinishReasonRecitation:
		return provider.FinishReasonStop
	default:
		return provider.FinishReasonStop
	}
}

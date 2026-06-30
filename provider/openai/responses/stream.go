package responses

import (
	"context"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 流式事件处理
// ==============================

// handleStreamEvent 根据事件类型分发到对应的处理方法。
//
// 接收 OpenAI Responses SSE 事件（responseStreamEvent），根据事件类型调用对应的处理函数，
// 返回统一格式的 StreamEvent 列表。
//
// textBlockStarted / thinkingBlockStarted 用于在没有原生 content_block_start 事件时
// 合成 BlockStart delta，确保与 Anthropic 协议的一致性。
func (p *ResponsesProvider) handleStreamEvent(ctx context.Context, event responseStreamEvent, textBlockStarted *bool, thinkingBlockStarted *bool) []provider.StreamEvent {
	switch event.Type {
	case "response.created":
		return p.contentResponseCreated(event)
	case "response.output_item.added":
		return p.contentOutputItemAdded(event, thinkingBlockStarted)
	case "response.output_item.done":
		return p.contentOutputItemDone(event)
	case "response.output_text.delta":
		return p.contentOutputTextDelta(event, textBlockStarted)
	case "response.reasoning_summary_part.added":
		return p.contentReasoningSummaryPartAdded(event, thinkingBlockStarted)
	case "response.reasoning_summary_text.delta":
		return p.contentReasoningSummaryTextDelta(event, thinkingBlockStarted)
	case "response.reasoning_summary_text.done":
		return p.contentReasoningSummaryTextDone()
	case "response.reasoning_summary_part.done":
		return p.contentReasoningSummaryPartDone()
	case "response.reasoning_text.delta":
		return p.contentReasoningTextDelta(event, thinkingBlockStarted)
	case "response.function_call_arguments.delta":
		return p.contentFunctionCallDelta(event)
	case "response.function_call_arguments.done":
		return p.contentFunctionCallDone(event)
	case "response.completed":
		return p.contentResponseCompleted(event)
	case "response.failed":
		return p.contentResponseFailed(ctx, event)
	case "response.incomplete":
		return p.contentResponseIncomplete(event)
	default:
		return nil
	}
}

// contentResponseCreated 处理响应创建事件。
//
// 从 response.created 事件提取 Response.ID，通过 MetadataDelta 返回，
// 供上层关联后续请求（如 WithPreviousResponseID）。
func (p *ResponsesProvider) contentResponseCreated(event responseStreamEvent) []provider.StreamEvent {
	if event.Response == nil || event.Response.ID == "" {
		return nil
	}
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewMetadataDelta(event.Response.ID, "", ""),
	}}
}

// contentOutputItemAdded 处理输出项添加事件。
//
// 当新输出项被添加到响应时触发：
//   - function_call: 返回工具调用开始事件
//   - reasoning: 提取 encrypted_content 存入 ThinkingSignature + 合成 BlockStart
func (p *ResponsesProvider) contentOutputItemAdded(event responseStreamEvent, thinkingBlockStarted *bool) []provider.StreamEvent {
	if event.Item == nil {
		return nil
	}
	switch event.Item.Type {
	case "function_call":
		callID := event.Item.CallID
		if callID == "" {
			callID = event.Item.ID
		}
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewToolCallDelta(callID, event.Item.Name),
		}}
	case "reasoning":
		// 参考 Vercel AI SDK: reasoning output_item.added 时提取 encrypted_content
		// 存入 ThinkingSignature，用于多轮对话保留推理上下文。
		var events []provider.StreamEvent
		if !*thinkingBlockStarted {
			*thinkingBlockStarted = true
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("thinking"),
			})
		}
		// encrypted_content 是 OpenAI 服务端加密的不透明 token
		if event.Item.EncryptedContent != "" {
			events = append(events, provider.StreamEvent{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewSignatureDelta(event.Item.EncryptedContent),
			})
		}
		return events
	default:
		return nil
	}
}

// contentOutputTextDelta 处理文本输出增量事件。
//
// 处理普通文本增量，首次文本增量时合成 BlockStart 事件，
// 确保与 Anthropic 协议的一致性。
func (p *ResponsesProvider) contentOutputTextDelta(event responseStreamEvent, textBlockStarted *bool) []provider.StreamEvent {
	// OpenAI Responses 没有明确的 content_block_start 事件，在第一次文本增量前合成 BlockStart
	if !*textBlockStarted {
		*textBlockStarted = true
		return []provider.StreamEvent{
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("text"),
			},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewTextDelta(event.Text),
			},
		}
	}

	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta(event.Text),
	}}
}

// contentReasoningTextDelta 处理推理文本增量事件。
//
// 处理 Reasoning/Thinking 过程的文本增量，首次推理增量时合成 BlockStart 事件，
// 确保流式事件的一致性。
func (p *ResponsesProvider) contentReasoningTextDelta(event responseStreamEvent, thinkingBlockStarted *bool) []provider.StreamEvent {
	// 推理文本也需要在第一次增量前合成 BlockStart
	if !*thinkingBlockStarted {
		*thinkingBlockStarted = true
		return []provider.StreamEvent{
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("thinking"),
			},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewThinkingDelta(event.Text),
			},
		}
	}

	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewThinkingDelta(event.Text),
	}}
}

// contentReasoningSummaryPartAdded 处理推理摘要段落添加事件。
//
// 参考 Vercel AI SDK: reasoning_summary_part.added 是主要的推理内容生命周期事件。
// 首次添加时合成 BlockStart（若尚未开始）。
func (p *ResponsesProvider) contentReasoningSummaryPartAdded(_ responseStreamEvent, thinkingBlockStarted *bool) []provider.StreamEvent {
	if !*thinkingBlockStarted {
		*thinkingBlockStarted = true
		return []provider.StreamEvent{{
			Type:  provider.StreamTypeDelta,
			Delta: provider.NewBlockStartDelta("thinking"),
		}}
	}
	return nil
}

// contentReasoningSummaryTextDelta 处理推理摘要文本增量事件。
//
// 这是 OpenAI Responses API 推理内容的主要传输事件（而非 reasoning_text.delta）。
// 与 contentReasoningTextDelta 逻辑一致，但触发来源不同。
func (p *ResponsesProvider) contentReasoningSummaryTextDelta(event responseStreamEvent, thinkingBlockStarted *bool) []provider.StreamEvent {
	if !*thinkingBlockStarted {
		*thinkingBlockStarted = true
		return []provider.StreamEvent{
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewBlockStartDelta("thinking"),
			},
			{
				Type:  provider.StreamTypeDelta,
				Delta: provider.NewThinkingDelta(event.Text),
			},
		}
	}

	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewThinkingDelta(event.Text),
	}}
}

// contentReasoningSummaryPartDone 处理推理摘要段落完成事件。
func (p *ResponsesProvider) contentReasoningSummaryPartDone() []provider.StreamEvent {
	return nil
}

// contentReasoningSummaryTextDone 处理推理摘要文本完成事件。
//
// 摘要文本的完整内容已通过增量事件传输完毕，此处无需重复发送。
func (p *ResponsesProvider) contentReasoningSummaryTextDone() []provider.StreamEvent {
	return nil
}

// contentOutputItemDone 处理输出项完成事件。
//
// 当 item.Type == "reasoning" 时，提取 ID（如 "rs_xxx"）和 EncryptedContent，
// 通过 MetadataDelta 返回，供上层在多轮对话中保留推理上下文。
func (p *ResponsesProvider) contentOutputItemDone(event responseStreamEvent) []provider.StreamEvent {
	if event.Item == nil || event.Item.Type != "reasoning" {
		return nil
	}
	if event.Item.ID == "" && event.Item.EncryptedContent == "" {
		return nil
	}
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewMetadataDelta("", event.Item.ID, event.Item.EncryptedContent),
	}}
}

// contentFunctionCallDelta 处理函数调用参数增量事件。
//
// 处理工具调用参数的流式增量，返回包含参数片段的 StreamEvent。
func (p *ResponsesProvider) contentFunctionCallDelta(event responseStreamEvent) []provider.StreamEvent {
	return []provider.StreamEvent{{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaData(event.Arguments),
	}}
}

// contentFunctionCallDone 处理函数调用完成事件。
//
// 当工具调用参数传输完成时触发，OpenAI Responses 协议中此事件无需特殊处理。
func (p *ResponsesProvider) contentFunctionCallDone(_ responseStreamEvent) []provider.StreamEvent {
	// 函数调用完成，无需特殊处理
	return nil
}

// contentResponseCompleted 处理响应完成事件（包含 usage 和 stop 事件）。
//
// 当 OpenAI 响应完成时触发，提取 Token 用量信息并返回 UsageDelta 和 StreamTypeStop 事件。
func (p *ResponsesProvider) contentResponseCompleted(event responseStreamEvent) []provider.StreamEvent {
	if event.Response == nil {
		return []provider.StreamEvent{{
			Type:         provider.StreamTypeStop,
			FinishReason: provider.FinishReasonStop,
		}}
	}
	resp := event.Response
	var events []provider.StreamEvent

	if resp.Usage != nil && (resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0) {
		var cachedTokens int64
		if resp.Usage.InputTokensDetails != nil {
			cachedTokens = int64(resp.Usage.InputTokensDetails.CachedTokens)
		}
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				int64(resp.Usage.InputTokens),
				int64(resp.Usage.OutputTokens),
				0,
				cachedTokens,
			),
		})
	}

	// 发送 StreamTypeStop 并携带完成原因
	events = append(events, provider.StreamEvent{
		Type:         provider.StreamTypeStop,
		FinishReason: mapResponseFinishReason(resp),
	})

	return events
}

// contentResponseFailed 处理响应失败事件。
//
// 当 OpenAI 请求失败时触发，包装错误信息并返回 ErrorEvent。
func (p *ResponsesProvider) contentResponseFailed(ctx context.Context, event responseStreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent

	if event.Response != nil && event.Response.Usage != nil &&
		(event.Response.Usage.InputTokens > 0 || event.Response.Usage.OutputTokens > 0) {
		var cachedTokens int64
		if event.Response.Usage.InputTokensDetails != nil {
			cachedTokens = int64(event.Response.Usage.InputTokensDetails.CachedTokens)
		}
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				int64(event.Response.Usage.InputTokens),
				int64(event.Response.Usage.OutputTokens),
				0,
				cachedTokens,
			),
		})
	}

	errMsg := "OpenAI 响应失败"
	events = append(events, provider.StreamEvent{
		Type: provider.StreamTypeError,
		Err:  xError.NewError(ctx, nil, errMsg, false, nil),
	})

	return events
}

// contentResponseIncomplete 处理响应未完成事件。
//
// 当响应因长度限制等原因未完成时触发，发送停止事件结束流。
func (p *ResponsesProvider) contentResponseIncomplete(event responseStreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent

	if event.Response != nil && event.Response.Usage != nil &&
		(event.Response.Usage.InputTokens > 0 || event.Response.Usage.OutputTokens > 0) {
		var cachedTokens int64
		if event.Response.Usage.InputTokensDetails != nil {
			cachedTokens = int64(event.Response.Usage.InputTokensDetails.CachedTokens)
		}
		events = append(events, provider.StreamEvent{
			Type: provider.StreamTypeDelta,
			Delta: provider.NewUsageDeltaWithCache(
				int64(event.Response.Usage.InputTokens),
				int64(event.Response.Usage.OutputTokens),
				0,
				cachedTokens,
			),
		})
	}

	// 响应未完成，发送停止事件并携带完成原因
	events = append(events, provider.StreamEvent{
		Type:         provider.StreamTypeStop,
		FinishReason: mapResponseFinishReason(event.Response),
	})

	return events
}

// mapResponseFinishReason 根据 OpenAI Responses 状态和输出推断完成原因。
//
// 参考 Vercel AI SDK mapOpenAIResponseFinishReason:
// incomplete 状态使用 incomplete_details.reason 区分 max_output_tokens/content_filter；
// 其他状态的 finishReason 为 null/undefined 时，有 function_call → tool_calls，否则 → stop。
//
// 注意：responseObjectItem.Summary 在流式事件中为 string 类型（reasoning summary 文本），
// 在非流式完整响应中可能为结构化数组，此处仅依赖 Status 和 Output[].Type 做推断。
func mapResponseFinishReason(resp *responseObject) provider.FinishReason {
	if resp == nil {
		return provider.FinishReasonStop
	}

	hasToolCalls := false
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			hasToolCalls = true
			break
		}
	}

	if resp.Status == "incomplete" {
		// incomplete_details.reason 需要从原始 JSON 提取，responseObject 未直接建模
		// 此处简化处理：incomplete + function_call → ToolCalls，否则 → Length
		if hasToolCalls {
			return provider.FinishReasonToolCalls
		}
		return provider.FinishReasonLength
	}

	if hasToolCalls {
		return provider.FinishReasonToolCalls
	}
	return provider.FinishReasonStop
}

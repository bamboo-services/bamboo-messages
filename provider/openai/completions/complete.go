package completions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
)

// Complete 非流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Chat Completions 格式，
// 通过底层 SDK 发起同步请求，返回完整的 CompletionResult。
func (p *CompletionsProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 在消息列表前插入系统提示，然后发起同步对话。
func (p *CompletionsProvider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	params := p.buildParams(systemPrompt, messages, config)

	provider.DebugRequest(
		"openai-completions",
		"POST /chat/completions (non-stream, model="+config.Model+")",
		nil,
		params,
	)

	response, err := p.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, xError.NewError(ctx, nil, formatUpstreamError(err), false, err)
	}

	// 检查响应
	if len(response.Choices) == 0 {
		if provider.DebugEnabled {
			log.Printf("[provider/openai-completions] 上游返回空响应, raw=%s", truncateResponseJSON(response))
		}
		diag := fmt.Sprintf(
			"OpenAI Completions 返回空响应 (choices=0), model=%s, legacyCompat=%v, tools=%d, maxTokens=%d, resp=%s",
			config.Model, p.legacyCompat, len(config.Tools), config.MaxTokens, truncateResponseJSON(response),
		)
		return nil, xError.NewError(ctx, nil, diag, false, nil)
	}

	choice := response.Choices[0]

	// 解析响应内容
	result := &provider.CompletionResult{
		Content:      choice.Message.Content,
		FinishReason: mapFinishReason(choice.FinishReason),
		Usage: provider.UsageData{
			InputTokens:          response.Usage.PromptTokens,
			OutputTokens:         response.Usage.CompletionTokens,
			CacheReadInputTokens: response.Usage.PromptTokensDetails.CachedTokens,
		},
	}

	// 推理内容提取：兼容 reasoning_content 和 reasoning 两种字段名
	reasoningRaw := ""
	if field, ok := choice.Message.JSON.ExtraFields["reasoning_content"]; ok && field.Raw() != "" {
		reasoningRaw = field.Raw()
	} else if field, ok := choice.Message.JSON.ExtraFields["reasoning"]; ok && field.Raw() != "" {
		reasoningRaw = field.Raw()
	}
	if reasoningRaw != "" {
		var reasoning string
		if err := json.Unmarshal([]byte(reasoningRaw), &reasoning); err == nil && reasoning != "" {
			result.Thinking = reasoning
		}
	}

	// 解析工具调用
	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: provider.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return result, nil
}

// mapFinishReason 将上游停止原因映射为统一的 FinishReason。
//
// 标准 OpenAI 值（stop/length/tool_calls）直接映射。
// 同时兼容智谱 GLM 等第三方端点的非标准 finish_reason 值：
//   - network_error：推理过程中网络中断，语义等价于 stop（流已结束）
//   - sensitive：内容策略违规，语义等价于 content_filter
//
// 未知值降级为 FinishReasonStop，保证流式传输不会因枚举校验而中断。
func mapFinishReason(reason string) provider.FinishReason {
	switch reason {
	case "stop":
		return provider.FinishReasonStop
	case "length":
		return provider.FinishReasonLength
	case "tool_calls":
		return provider.FinishReasonToolCalls
	case "network_error":
		// GLM 推理过程中网络中断，流已终止，降级为 stop
		return provider.FinishReasonStop
	case "sensitive", "content_filter":
		// GLM 内容策略违规，等价于 OpenAI content_filter
		return provider.FinishReasonStop
	default:
		return provider.FinishReasonStop
	}
}

// maxResponseLogLen 响应体日志最大长度（字符数），超过截断。
const maxResponseLogLen = 500

// truncateResponseJSON 将 OpenAI 响应序列化为 JSON 并截断，用于错误日志。
//
// 仅在错误路径调用，帮助诊断 GLM 等第三方端点返回空响应的原因。
func truncateResponseJSON[T any](resp T) string {
	raw, err := json.Marshal(resp)
	if err != nil {
		return "<marshal error>"
	}
	s := string(raw)
	if len(s) <= maxResponseLogLen {
		return s
	}
	return s[:maxResponseLogLen] + "...(truncated)"
}

// maxUpstreamDumpLen openai.Error DumpResponse 快照最大长度。
const maxUpstreamDumpLen = 1000

// formatUpstreamError 从 openai-go SDK 错误中提取 HTTP 状态码和响应快照，
// 生成包含完整诊断信息的错误消息。
//
// openai-go v3 SDK 在非 200 时返回 *openai.Error，包含 StatusCode、Request、Response。
// 若不是该类型，退化为原始错误消息。
func formatUpstreamError(err error) string {
	var apierr *openai.Error
	if errors.As(err, &apierr) {
		dump := string(apierr.DumpResponse(true))
		if len(dump) > maxUpstreamDumpLen {
			dump = dump[:maxUpstreamDumpLen] + "...(truncated)"
		}
		return fmt.Sprintf("OpenAI Completions 上游错误 (HTTP %d): %s", apierr.StatusCode, dump)
	}
	return "OpenAI Completions 非流式对话失败"
}

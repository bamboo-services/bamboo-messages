package completions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Complete 非流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Chat Completions 格式，
// 通过 httpClient 发起同步请求，返回完整的 CompletionResult。
func (p *CompletionsProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 在消息列表前插入系统提示，然后发起同步对话。
// 使用 httpClient.DoWithDebug 发送 HTTP 请求，不依赖 openai-go SDK。
func (p *CompletionsProvider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	params := p.buildParams(systemPrompt, messages, config)

	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Completions 请求参数序列化失败: %v", err), 0)
	}

	endpoint := "POST /chat/completions (non-stream, model=" + config.Model + ")"
	resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, "/chat/completions", bodyBytes, string(p.GetProviderType()), endpoint)
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Completions 非流式对话请求失败: %v", err), 0)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Completions 读取响应体失败: %v", err), 0)
	}
	provider.DebugResponse("openai", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header), body)

	// HTTP 状态码检查
	if resp.StatusCode >= 400 {
		return nil, pkgErrors.NewBambooError("上游", formatUpstreamError(resp.StatusCode, body), resp.StatusCode)
	}

	// 解析响应
	var chatResp chatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		if provider.DebugEnabled {
			xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
				fmt.Sprintf("响应 JSON 解析失败, raw=%s", truncateBody(body)))
		}
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf(
			"OpenAI Completions 响应解析失败: %v, raw=%s", err, truncateBody(body),
		), 0)
	}

	// 检查响应
	if len(chatResp.Choices) == 0 {
		if provider.DebugEnabled {
			xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
				fmt.Sprintf("上游返回空响应, raw=%s", truncateBody(body)))
		}
		diag := fmt.Sprintf(
			"OpenAI Completions 返回空响应 (choices=0), model=%s, legacyCompat=%v, tools=%d, maxTokens=%d, resp=%s",
			config.Model, p.legacyCompat, len(config.Tools), config.MaxTokens, truncateBody(body),
		)
		return nil, pkgErrors.NewBambooError("上游", diag, 0)
	}

	choice := chatResp.Choices[0]

	// 解析响应内容
	result := &provider.CompletionResult{
		Content: choice.Message.Content,
	}

	// FinishReason 映射
	if choice.FinishReason != nil {
		result.FinishReason = mapFinishReason(*choice.FinishReason)
	}

	// Usage 统计
	if chatResp.Usage != nil {
		var cached int
		if chatResp.Usage.PromptTokensDetails != nil {
			cached = chatResp.Usage.PromptTokensDetails.CachedTokens
		}
		result.Usage = provider.UsageData{
			InputTokens:          int64(chatResp.Usage.PromptTokens),
			OutputTokens:         int64(chatResp.Usage.CompletionTokens),
			CacheReadInputTokens: int64(cached),
		}
	}

	// 推理内容提取：兼容 reasoning_content 和 reasoning 两种字段名
	if reasoningStr := parseReasoningRaw(choice.Message.ReasoningContent); reasoningStr != "" {
		result.Thinking = reasoningStr
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

	// 响应 ID
	result.ResponseID = chatResp.ID

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

// truncateBody 将响应体截断到最大日志长度，用于错误日志。
//
// 仅在错误路径调用，帮助诊断 GLM 等第三方端点返回空响应的原因。
func truncateBody(body []byte) string {
	s := string(body)
	if len(s) <= maxResponseLogLen {
		return s
	}
	return s[:maxResponseLogLen] + "...(truncated)"
}

// maxUpstreamDumpLen 上游错误响应快照最大长度。
const maxUpstreamDumpLen = 1000

// formatUpstreamError 从 HTTP 状态码和响应体中提取错误信息，
// 生成包含完整诊断信息的错误消息。
//
// 尝试解析响应体为 openaiError 结构体提取结构化错误信息；
// 解析失败时退化为原始响应体文本。
func formatUpstreamError(statusCode int, body []byte) string {
	var apiErr openaiError
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
		return fmt.Sprintf("OpenAI Completions 上游错误 (HTTP %d): %s", statusCode, apiErr.Error.Message)
	}

	dump := string(body)
	if len(dump) > maxUpstreamDumpLen {
		dump = dump[:maxUpstreamDumpLen] + "...(truncated)"
	}
	return fmt.Sprintf("OpenAI Completions 上游错误 (HTTP %d): %s", statusCode, dump)
}

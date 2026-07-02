package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bamboo-services/bamboo-base-go/common/error"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Complete 非流式对话。
//
// 无系统提示的非流式对话，内部调用 CompleteWithSystem 并传入空 systemPrompt。
// 同步返回完整响应和可能的错误。
func (p *Provider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 将统一 provider.Message 转换为 Gemini 协议格式，
// 通过 httpClient 直接调用 Gemini REST API（generateContent 端点），
// 返回 CompletionResult。
// 支持系统提示、温度、TopP、MaxTokens、Stop 序列、工具调用、Thinking 配置、ToolChoice 等参数。
func (p *Provider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	// 构建请求体
	reqBody := p.buildRequestBody(messages, systemPrompt, config, false)
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, xError.NewError(ctx, nil, xError.ErrMessage("Gemini 非流式对话请求序列化失败"), false, err)
	}

	// Gemini 非流式端点：/v1beta/models/{model}:generateContent
	endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", config.Model)

	resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, endpoint, bodyBytes, "gemini", endpoint)
	if err != nil {
		return nil, xError.NewError(ctx, nil, xError.ErrMessage("Gemini 非流式对话请求失败"), false, err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, xError.NewError(ctx, nil, xError.ErrMessage("Gemini 非流式对话响应读取失败"), false, err)
	}

	// HTTP 状态码 >= 400 → 解析错误响应
	if resp.StatusCode >= 400 {
		var errResp geminiErrorResponse
		_ = json.Unmarshal(respBytes, &errResp)
		errMsg := "Gemini API 返回错误"
		if errResp.Error != nil && errResp.Error.Message != "" {
			errMsg = errResp.Error.Message
		}
		return nil, pkgErrors.NewHTTPError(resp.StatusCode, errMsg,
			fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes)))
	}

	// 反序列化响应
	var geminiResp generateContentResponse
	if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
		return nil, xError.NewError(ctx, nil, xError.ErrMessage("Gemini 非流式对话响应解析失败"), false, err)
	}

	result := &provider.CompletionResult{
		FinishReason: provider.FinishReasonStop,
	}

	// 提取 Token 用量
	if geminiResp.UsageMetadata != nil {
		result.Usage = provider.UsageData{
			InputTokens:          int64(geminiResp.UsageMetadata.PromptTokenCount),
			OutputTokens:         int64(geminiResp.UsageMetadata.CandidatesTokenCount),
			CacheReadInputTokens: int64(geminiResp.UsageMetadata.CachedContentTokenCount),
		}
	}

	// 遍历响应内容
	if len(geminiResp.Candidates) > 0 {
		candidate := &geminiResp.Candidates[0]
		result.FinishReason = mapFinishReason(candidate.FinishReason)

		if candidate.Content != nil {
			for i, part := range candidate.Content.Parts {
				// 推理内容（Thought 标记）
				if part.Thought && part.Text != "" {
					result.Thinking += part.Text
				}
				// 文本内容（忽略 Thought==true 的推理内容）
				if !part.Thought && part.Text != "" {
					result.Content += part.Text
				}
				// 工具调用
				if part.FunctionCall != nil {
					id := part.FunctionCall.ID
					if id == "" {
						id = fmt.Sprintf("gemini_call_%s_%d", part.FunctionCall.Name, i)
					}
					argsStr := string(part.FunctionCall.Args)
					if argsStr == "" {
						argsStr = "{}"
					}
					result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
						ID:   id,
						Type: "function",
						Function: provider.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: argsStr,
						},
					})
					// 如果有工具调用且 FinishReason 未明确指定，设为 ToolCalls
					if result.FinishReason == provider.FinishReasonStop {
						result.FinishReason = provider.FinishReasonToolCalls
					}
				}
			}
		}
	}

	return result, nil
}

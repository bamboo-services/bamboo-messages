package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
// 将统一 provider.Message 转换为 Anthropic 协议格式，
// 通过 httpClient 发起同步 HTTP 请求，返回 CompletionResult。
// 支持系统提示、温度、TopP、Stop 序列、工具调用、Thinking 配置、TopK、ToolChoice 等参数。
func (p *Provider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	params := p.buildParams(systemPrompt, messages, config)

	body, err := json.Marshal(params)
	if err != nil {
		return nil, pkgErrors.NewError(ctx, nil, "Anthropic 请求参数序列化失败", false, err)
	}

	resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, "/v1/messages", body, "anthropic", "POST /v1/messages (non-stream, model="+config.Model+")")
	if err != nil {
		return nil, pkgErrors.NewError(ctx, nil, "Anthropic 非流式对话请求失败", false, err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, pkgErrors.NewError(ctx, nil, "Anthropic 非流式响应读取失败", false, err)
	}

	// 检查 HTTP 状态码，解析错误响应
	if resp.StatusCode >= 400 {
		var errResp anthropicErrorResponse
		if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil && errResp.Error != nil {
			return nil, pkgErrors.NewHTTPError(resp.StatusCode,
				fmt.Sprintf("Anthropic API 错误 [%d]: %s", resp.StatusCode, errResp.Error.Message))
		}
		return nil, pkgErrors.NewHTTPError(resp.StatusCode,
			fmt.Sprintf("Anthropic API 返回错误状态码 %d", resp.StatusCode))
	}

	// 解析响应
	var msgResp messageResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, pkgErrors.NewError(ctx, nil, "Anthropic 非流式响应解析失败", false, err)
	}

	// 构建 CompletionResult
	result := &provider.CompletionResult{
		ResponseID: msgResp.ID,
	}

	// 映射停止原因
	if msgResp.StopReason != nil {
		result.FinishReason = mapFinishReason(*msgResp.StopReason)
	}

	// 提取 Token 用量（保留缓存统计）
	if msgResp.Usage != nil {
		result.Usage = provider.UsageData{
			InputTokens:              int64(msgResp.Usage.InputTokens),
			OutputTokens:             int64(msgResp.Usage.OutputTokens),
			CacheCreationInputTokens: int64(msgResp.Usage.CacheCreationInputTokens),
			CacheReadInputTokens:     int64(msgResp.Usage.CacheReadInputTokens),
		}
	}

	// 遍历响应内容块，提取文本、思考和工具调用
	for _, block := range msgResp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "thinking":
			result.Thinking += block.Thinking
			// 保留 thinking 签名（用于多轮对话验证）
			if block.Signature != "" {
				result.ThinkingSignature = block.Signature
			}
		case "tool_use":
			inputStr := string(block.Input)
			if len(block.Input) == 0 {
				inputStr = "{}"
			}
			result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: provider.FunctionCall{
					Name:      block.Name,
					Arguments: inputStr,
				},
			})
		}
	}

	return result, nil
}

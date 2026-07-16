package bamboo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
// 将统一 provider.Message 转换为 bamboo 原生协议格式，
// 通过 httpClient 发起同步 HTTP 请求，返回 CompletionResult。
func (p *Provider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	params := buildParams(systemPrompt, messages, config)

	body, err := json.Marshal(params)
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("bamboo 请求参数序列化失败: %v", err), 0)
	}

	resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, "/v1/bamboo", body, "bamboo", "POST /v1/bamboo (non-stream, model="+config.Model+")")
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("bamboo 非流式对话请求失败: %v", err), 0)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("bamboo 非流式响应读取失败: %v", err), 0)
	}
	provider.DebugResponse("bamboo", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header), respBody)

	// HTTP >= 400 时解析错误响应
	if resp.StatusCode >= 400 {
		var errResp wireErrorPayload
		if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			return nil, pkgErrors.NewBambooError("上游",
				fmt.Sprintf("bamboo: %s", errResp.Error.Message), resp.StatusCode)
		}
		return nil, pkgErrors.NewBambooError("上游", "bamboo: 未知错误", resp.StatusCode)
	}

	// 解析响应
	var wireResp wireResponse
	if err := json.Unmarshal(respBody, &wireResp); err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("bamboo 非流式响应解析失败: %v", err), 0)
	}

	// 构建 CompletionResult
	result := &provider.CompletionResult{
		ResponseID: wireResp.ResponseID,
	}

	// 映射停止原因
	result.FinishReason = mapBambooFinishReason(wireResp.StopReason)

	// 提取 Token 用量
	result.Usage = provider.UsageData{
		InputTokens:              wireResp.Usage.InputTokens,
		OutputTokens:             wireResp.Usage.OutputTokens,
		CacheCreationInputTokens: wireResp.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     wireResp.Usage.CacheReadInputTokens,
	}

	// 遍历响应内容块
	var signatures []string
	for _, block := range wireResp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "thinking":
			result.Thinking += block.Thinking
			if block.Signature != "" {
				signatures = append(signatures, block.Signature)
			}
		case "redacted_thinking":
			result.RedactedThinking = append(result.RedactedThinking, block.Data)
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

	if len(signatures) > 0 {
		result.ThinkingSignature = strings.Join(signatures, "\n---\n")
	}

	return result, nil
}

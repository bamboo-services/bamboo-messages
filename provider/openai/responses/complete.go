package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 非流式响应辅助类型
// ==============================

// completeResponse 是非流式响应的本地解析结构体。
//
// 由于 types.go 的 responseObjectItem.Summary 为 string 类型，
// 而 OpenAI Responses API 非流式响应中 reasoning item 的 summary 为数组
// （[{"type":"summary_text","text":"..."}]），
// 此处定义本地结构体以正确解析 summary 数组和 content 数组。
//
// 其他字段复用 types.go 中的 responsesUsage 和 openaiError。
type completeResponse struct {
	// ID 响应 ID。
	ID string `json:"id,omitempty"`
	// Object 对象类型。
	Object string `json:"object,omitempty"`
	// Status 响应状态（completed / incomplete / failed）。
	Status string `json:"status,omitempty"`
	// Model 使用的模型 ID。
	Model string `json:"model,omitempty"`
	// Output 输出项列表。
	Output []completeResponseItem `json:"output,omitempty"`
	// Usage Token 用量统计。
	Usage *responsesUsage `json:"usage,omitempty"`
	// CreatedAt 响应创建时间戳。
	CreatedAt float64 `json:"created_at,omitempty"`
	// IncompleteDetails 未完成详情。
	IncompleteDetails *struct {
		// Reason 未完成原因（max_output_tokens / content_filter）。
		Reason string `json:"reason,omitempty"`
	} `json:"incomplete_details,omitempty"`
	// Error 错误信息（response.failed 时携带）。
	Error *struct {
		// Message 错误消息。
		Message string `json:"message,omitempty"`
		// Type 错误类型。
		Type string `json:"type,omitempty"`
		// Code 错误码。
		Code string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

// completeResponseItem 是非流式响应中输出项的本地解析结构体。
type completeResponseItem struct {
	// Type 输出项类型：message / function_call / reasoning。
	Type string `json:"type,omitempty"`
	// ID 输出项 ID。
	ID string `json:"id,omitempty"`
	// Role 消息角色（message 类型使用）。
	Role string `json:"role,omitempty"`
	// Status 输出项状态。
	Status string `json:"status,omitempty"`
	// Content 内容块列表（message 和 reasoning 类型共用，结构相同）。
	Content []responseContentPart `json:"content,omitempty"`
	// Name 工具名称（function_call 类型使用）。
	Name string `json:"name,omitempty"`
	// CallID 工具调用 ID（function_call 类型使用）。
	CallID string `json:"call_id,omitempty"`
	// Arguments 工具调用参数（function_call 类型使用）。
	Arguments string `json:"arguments,omitempty"`
	// Summary 推理摘要文本数组（reasoning 类型使用）。
	Summary []completeSummaryPart `json:"summary,omitempty"`
	// EncryptedContent 服务端加密的推理内容（reasoning 类型使用）。
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// completeSummaryPart 推理摘要或内容的一部分。
type completeSummaryPart struct {
	// Type 摘要类型（summary_text / reasoning_text 等）。
	Type string `json:"type,omitempty"`
	// Text 文本内容。
	Text string `json:"text,omitempty"`
}

// ==============================
// 非流式对话实现
// ==============================

// Complete 非流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Responses 格式，
// 通过 HTTPClient 发起同步请求，返回完整响应结果和错误信息。
func (p *ResponsesProvider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 在消息前插入系统提示，然后调用 OpenAI Responses API 发起同步请求，
// 返回包含文本内容、工具调用、Token 用量等信息的完整结果。
//
// 去 SDK 化实现：通过 httpClient.DoWithDebug 发送 HTTP 请求，
// 使用 json.Unmarshal 解析响应体，不依赖 openai-go SDK。
func (p *ResponsesProvider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	// 构建请求参数
	params := p.buildParams(config.Model, systemPrompt, messages, config, false)

	body, err := json.Marshal(params)
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses 请求参数序列化失败: %v", err), 0)
	}

	// 发送 HTTP 请求（含 debug 日志）
	resp, err := p.httpClient.DoWithDebug(ctx, "POST", "/responses", body, "openai-responses", "POST /responses (non-stream, model="+config.Model+")")
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses 非流式对话请求失败: %v", err), 0)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses 响应体读取失败: %v", err), 0)
	}

	provider.DebugResponse("openai-responses", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header), respBody)

	// 检查 HTTP 状态码，解析错误响应
	if resp.StatusCode >= 400 {
		var apiErr openaiError
		_ = json.Unmarshal(respBody, &apiErr)
		errMsg := "OpenAI Responses"
		if apiErr.Error.Message != "" {
			errMsg += ": " + apiErr.Error.Message
		}
		return nil, pkgErrors.NewBambooError("上游", errMsg, resp.StatusCode)
	}

	// 解析响应体
	var response completeResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Responses 响应体解析失败: %v", err), 0)
	}

	// 构建结果
	result := &provider.CompletionResult{
		ResponseID: response.ID,
	}

	// 遍历输出项提取内容
	for _, item := range response.Output {
		switch item.Type {
		case "message":
			// 消息项：提取 output_text 内容
			for _, content := range item.Content {
				if content.Type == "output_text" {
					result.Content += content.Text
				}
			}
		case "reasoning":
			// 推理项：提取 ID、摘要文本和加密内容
			// reasoning item 的 ID（如 "rs_xxx"），用于多轮对话中引用推理上下文。
			// 多个 reasoning items 时取最后一个（与 OpenAI Responses 多轮对话语义一致）。
			if item.ID != "" {
				result.ReasoningID = item.ID
			}
			// 优先从 summary 数组提取推理文本
			for _, sum := range item.Summary {
				if sum.Text != "" {
					result.Thinking += sum.Text
				}
			}
			// summary 为空时回退到 content 数组
			if result.Thinking == "" {
				for _, content := range item.Content {
					if content.Text != "" {
						result.Thinking += content.Text
					}
				}
			}
			// encrypted_content 是 OpenAI 服务端加密的不透明 token，
			// Responses → Responses 直连时原样传回可保持推理上下文连续性。
			if item.EncryptedContent != "" {
				result.ThinkingSignature = item.EncryptedContent
			}
		case "function_call":
			// 函数调用项：提取工具调用信息
			result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: provider.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}

	// 设置完成原因（与流式 mapResponseFinishReason 逻辑一致）
	result.FinishReason = mapCompleteFinishReason(&response, len(result.ToolCalls) > 0)

	// 设置用量统计
	if response.Usage != nil {
		result.Usage = provider.UsageData{
			InputTokens:  int64(response.Usage.InputTokens),
			OutputTokens: int64(response.Usage.OutputTokens),
		}
		if response.Usage.InputTokensDetails != nil {
			result.Usage.CacheReadInputTokens = int64(response.Usage.InputTokensDetails.CachedTokens)
		}
	}

	return result, nil
}

// mapCompleteFinishReason 根据非流式响应状态和输出推断完成原因。
//
// 与流式 mapResponseFinishReason 逻辑一致，但使用 completeResponse 类型
// 并支持 IncompleteDetails.Reason 字段：
//   - incomplete + max_output_tokens → Length
//   - incomplete + content_filter + tool_calls → ToolCalls
//   - incomplete + content_filter + 无 tool_calls → Stop
//   - incomplete + 其他 + tool_calls → ToolCalls
//   - incomplete + 其他 + 无 tool_calls → Length
//   - completed + tool_calls → ToolCalls
//   - completed + 无 tool_calls → Stop
func mapCompleteFinishReason(resp *completeResponse, hasToolCalls bool) provider.FinishReason {
	if resp.Status == "incomplete" {
		reason := ""
		if resp.IncompleteDetails != nil {
			reason = resp.IncompleteDetails.Reason
		}
		switch reason {
		case "max_output_tokens":
			return provider.FinishReasonLength
		case "content_filter":
			if hasToolCalls {
				return provider.FinishReasonToolCalls
			}
			return provider.FinishReasonStop
		default:
			if hasToolCalls {
				return provider.FinishReasonToolCalls
			}
			return provider.FinishReasonLength
		}
	}

	if hasToolCalls {
		return provider.FinishReasonToolCalls
	}
	return provider.FinishReasonStop
}

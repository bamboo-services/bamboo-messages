package responses

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── OpenAI Responses 响应 JSON 结构体 ──

// responsesOutput OpenAI Responses API 响应 JSON 结构。
type responsesOutput struct {
	ID        string         `json:"id"`
	Object    string         `json:"object"`
	CreatedAt int64          `json:"created_at"`
	Model     string         `json:"model"`
	Status    string         `json:"status"`
	Output    []outputItem   `json:"output"`
	Usage     responsesUsage `json:"usage"`
}

// outputItem output 数组元素，通过 type 区分 message / reasoning / function_call。
type outputItem struct {
	Type string `json:"type"`
	// message 专用
	ID      string          `json:"id,omitempty"`
	Role    string          `json:"role,omitempty"`
	Status  string          `json:"status,omitempty"`
	Content []outputContent `json:"content,omitempty"`
	// reasoning 专用 — OpenAI SDK ResponseReasoningItem.Summary 标记 api:"required"
	Summary          []outputReasoningSummary `json:"summary"`
	EncryptedContent string                   `json:"encrypted_content,omitempty"`
	// function_call 专用
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// outputContent message 项目中的 content 元素。
type outputContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// outputReasoningSummary reasoning item 的 summary 元素（type 固定为 "summary_text"）。
type outputReasoningSummary struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// responsesUsage Responses 格式的 Token 用量。
type responsesUsage struct {
	InputTokens         int64                     `json:"input_tokens"`
	OutputTokens        int64                     `json:"output_tokens"`
	TotalTokens         int64                     `json:"total_tokens"`
	InputTokensDetails  *responsesInputTokensDet  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *responsesOutputTokensDet `json:"output_tokens_details,omitempty"`
}

type responsesInputTokensDet struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type responsesOutputTokensDet struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

// serializeResponse 将 Bamboo Response 序列化为 OpenAI Responses JSON。
func serializeResponse(resp *bamboo.Response) ([]byte, error) {
	var output []outputItem

	var textParts []string
	var reasoningItems []outputItem
	var functionCallItems []outputItem

	for _, block := range resp.Content {
		switch b := block.(type) {
		case *bamboo.TextBlock:
			textParts = append(textParts, b.Text)

		case *bamboo.ThinkingBlock:
			reasoningItems = append(reasoningItems, outputItem{
				Type:    "reasoning",
				ID:      "rs_" + b.Signature,
				Content: []outputContent{},
				Summary: []outputReasoningSummary{
					{Type: "summary_text", Text: b.Thinking},
				},
			})

		case *bamboo.ToolUseBlock:
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			functionCallItems = append(functionCallItems, outputItem{
				Type:      "function_call",
				ID:        "fc_" + b.ID,
				CallID:    b.ID,
				Name:      b.Name,
				Arguments: args,
			})

		case *bamboo.ImageBlock:
			// OpenAI Responses 响应中不支持图片内容块，记录警告并跳过
			log.Printf("[codec/responses] warning: ImageBlock in assistant response is non-standard, skipped")

		case *bamboo.DocumentBlock:
			// OpenAI Responses 响应中不支持文档内容块，记录警告并跳过
			log.Printf("[codec/responses] warning: DocumentBlock in assistant response is non-standard, skipped")

		case *bamboo.ToolResultBlock:
			// ToolResultBlock 不应出现在 assistant 响应中，记录警告并跳过
			log.Printf("[codec/responses] warning: ToolResultBlock should not appear in assistant response, skipped (tool_use_id=%s)", b.ToolUseID)
		}
	}

	// reasoning 项目排在前面（与 OpenAI 行为一致）
	output = append(output, reasoningItems...)

	// 所有 TextBlock 合并到一个 message 项目
	if len(textParts) > 0 {
		output = append(output, outputItem{
			Type:    "message",
			ID:      "msg_" + resp.ID,
			Role:    "assistant",
			Status:  "completed",
			Content: []outputContent{{Type: "output_text", Text: strings.Join(textParts, "")}},
		})
	}

	// function_call 项目
	output = append(output, functionCallItems...)

	// 如果 output 为空，添加一个空 message 项目保持结构完整
	if len(output) == 0 {
		output = append(output, outputItem{
			Type:    "message",
			ID:      "msg_" + resp.ID,
			Role:    "assistant",
			Status:  "completed",
			Content: []outputContent{{Type: "output_text", Text: ""}},
		})
	}

	usage := responsesUsage{
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		TotalTokens:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
		InputTokensDetails:  &responsesInputTokensDet{CachedTokens: resp.Usage.CacheReadInputTokens},
		OutputTokensDetails: &responsesOutputTokensDet{},
	}

	out := responsesOutput{
		ID:        resp.ID,
		Object:    "response",
		CreatedAt: resp.CreatedAt,
		Model:     resp.Model,
		Status:    mapStatus(resp.StopReason),
		Output:    output,
		Usage:     usage,
	}

	return json.Marshal(out)
}

// mapStatus 将 Bamboo FinishReason 映射为 Responses status 字段。
func mapStatus(reason bamboo.FinishReason) string {
	switch reason {
	case bamboo.FinishReasonEndTurn:
		return "completed"
	case bamboo.FinishReasonMaxTokens:
		return "incomplete"
	case bamboo.FinishReasonToolUse:
		return "completed"
	case bamboo.FinishReasonStopSequence:
		return "completed"
	default:
		return "completed"
	}
}

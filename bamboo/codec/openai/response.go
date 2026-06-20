package openai

import (
	"encoding/json"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// openaiResponse OpenAI Chat Completions 响应 JSON 结构。
type openaiResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiChoice struct {
	Index        int          `json:"index"`
	Message      openaiMsgOut `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

type openaiMsgOut struct {
	Role             string          `json:"role"`
	Content          *string         `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolOut `json:"tool_calls,omitempty"`
}

type openaiToolOut struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function openaiToolOutFn `json:"function"`
}

type openaiToolOutFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiUsage struct {
	PromptTokens        int64                  `json:"prompt_tokens"`
	CompletionTokens    int64                  `json:"completion_tokens"`
	TotalTokens         int64                  `json:"total_tokens"`
	PromptTokensDetails *openaiPromptTokensDet `json:"prompt_tokens_details,omitempty"`
}

type openaiPromptTokensDet struct {
	CachedTokens int64 `json:"cached_tokens,omitempty"`
}

// serializeResponse 将 Bamboo Response 序列化为 OpenAI Chat Completions JSON。
func serializeResponse(resp *bamboo.Response) ([]byte, error) {
	choice := openaiChoice{
		Index:        0,
		FinishReason: mapFinishReason(resp.StopReason),
		Message:      openaiMsgOut{Role: "assistant"},
	}

	var textParts []string
	var reasoningParts []string
	var toolCalls []openaiToolOut

	for _, block := range resp.Content {
		switch b := block.(type) {
		case *bamboo.TextBlock:
			textParts = append(textParts, b.Text)
		case *bamboo.ThinkingBlock:
			reasoningParts = append(reasoningParts, b.Thinking)
		case *bamboo.ToolUseBlock:
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, openaiToolOut{
				ID:   b.ID,
				Type: "function",
				Function: openaiToolOutFn{
					Name:      b.Name,
					Arguments: args,
				},
			})
		}
	}

	// content
	if len(textParts) > 0 {
		content := strings.Join(textParts, "")
		choice.Message.Content = &content
	} else if len(toolCalls) == 0 {
		// 无文本也无工具调用时 content 设为空字符串指针
		empty := ""
		choice.Message.Content = &empty
	}

	// reasoning_content
	if len(reasoningParts) > 0 {
		choice.Message.ReasoningContent = strings.Join(reasoningParts, "")
	}

	// tool_calls
	if len(toolCalls) > 0 {
		choice.Message.ToolCalls = toolCalls
	}

	promptTokens := resp.Usage.InputTokens
	completionTokens := resp.Usage.OutputTokens

	usage := openaiUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	if resp.Usage.CacheCreationInputTokens > 0 || resp.Usage.CacheReadInputTokens > 0 {
		usage.PromptTokensDetails = &openaiPromptTokensDet{
			CachedTokens: resp.Usage.CacheReadInputTokens,
		}
	}

	out := openaiResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: resp.CreatedAt,
		Model:   resp.Model,
		Choices: []openaiChoice{choice},
		Usage:   usage,
	}

	return json.Marshal(out)
}

// mapFinishReason 将 Bamboo FinishReason 映射为 OpenAI finish_reason。
func mapFinishReason(reason bamboo.FinishReason) string {
	switch reason {
	case bamboo.FinishReasonEndTurn:
		return "stop"
	case bamboo.FinishReasonMaxTokens:
		return "length"
	case bamboo.FinishReasonToolUse:
		return "tool_calls"
	case bamboo.FinishReasonStopSequence:
		return "stop"
	default:
		return "stop"
	}
}

package anthropic

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// anthropicResponse Anthropic Messages 响应 JSON 结构。
type anthropicResponse struct {
	ID           string              `json:"id"`
	Type         string              `json:"type"`
	Role         string              `json:"role"`
	Content      []json.RawMessage   `json:"content"`
	Model        string              `json:"model"`
	StopReason   bamboo.FinishReason `json:"stop_reason"`
	StopSequence *string             `json:"stop_sequence"`
	Usage        anthropicUsage      `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
}

// serializeResponse 将 Bamboo Response 序列化为 Anthropic Messages JSON。
//
// bamboo 的 FinishReason 枚举值与 Anthropic stop_reason 一致（end_turn/max_tokens/tool_use/stop_sequence），
// 可直接 1:1 映射。Content blocks 同样保持 Anthropic 原生结构。
func serializeResponse(resp *bamboo.Response) ([]byte, error) {
	contentBlocks := make([]json.RawMessage, 0, len(resp.Content))
	for _, block := range resp.Content {
		raw, err := serializeContentBlock(block)
		if err != nil {
			return nil, err
		}
		if raw != nil {
			contentBlocks = append(contentBlocks, raw)
		}
	}

	// stop_sequence: bamboo.Response.StopSequence 为 string，Anthropic 期望 null 或 string
	var stopSeq *string
	if resp.StopSequence != "" {
		stopSeq = &resp.StopSequence
	}

	out := anthropicResponse{
		ID:           resp.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      contentBlocks,
		Model:        resp.Model,
		StopReason:   resp.StopReason,
		StopSequence: stopSeq,
		Usage: anthropicUsage{
			InputTokens:              resp.Usage.InputTokens,
			OutputTokens:             resp.Usage.OutputTokens,
			CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
		},
	}

	return json.Marshal(out)
}

// serializeContentBlock 将单个 bamboo.ContentBlock 序列化为 Anthropic content block JSON。
func serializeContentBlock(block bamboo.ContentBlock) (json.RawMessage, error) {
	switch b := block.(type) {
	case *bamboo.TextBlock:
		return json.Marshal(map[string]any{
			"type": "text",
			"text": b.Text,
		})

	case *bamboo.ThinkingBlock:
		return json.Marshal(map[string]any{
			"type":      "thinking",
			"thinking":  b.Thinking,
			"signature": b.Signature,
		})

	case *bamboo.ToolUseBlock:
		// input 为 json.RawMessage，需转为 JSON object 输出
		input := b.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		// 验证是否为合法 JSON，否则降级为空对象
		var check any
		if err := json.Unmarshal(input, &check); err != nil {
			input = json.RawMessage(`{}`)
		}
		return json.Marshal(map[string]any{
			"type":  "tool_use",
			"id":    b.ID,
			"name":  b.Name,
			"input": input,
		})

	case *bamboo.ImageBlock:
		if b.Source == nil {
			return nil, nil
		}
		return json.Marshal(map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       b.Source.Type,
				"media_type": b.Source.MediaType,
				"data":       b.Source.Data,
				"url":        b.Source.URL,
			},
		})
	}
	return nil, nil
}

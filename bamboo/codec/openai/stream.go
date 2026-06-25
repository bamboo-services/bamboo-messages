package openai

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// openaiChunk OpenAI 流式 chunk JSON 结构。
type openaiChunk struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []openaiDelta    `json:"choices"`
	Usage   *openaiStreamUsage `json:"usage,omitempty"`
}

type openaiStreamUsage struct {
	PromptTokens        int64                  `json:"prompt_tokens"`
	CompletionTokens    int64                  `json:"completion_tokens"`
	TotalTokens         int64                  `json:"total_tokens"`
	PromptTokensDetails *openaiPromptTokensDet `json:"prompt_tokens_details,omitempty"`
}

type openaiDelta struct {
	Index        int            `json:"index"`
	Delta        openaiDeltaMsg `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}

type openaiDeltaMsg struct {
	Role    string          `json:"role,omitempty"`
	Content string          `json:"content,omitempty"`
	// ReasoningContent 思考/推理内容增量。
	// 注意：此字段为 DeepSeek/vLLM 兼容性扩展，非官方 OpenAI Chat Completions 规范字段。
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiDeltaTC `json:"tool_calls,omitempty"`
}

type openaiDeltaTC struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function openaiDeltaTCFn `json:"function"`
}

type openaiDeltaTCFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// openaiStreamSerializer OpenAI 流式序列化器，有状态。
type openaiStreamSerializer struct {
	id               string
	created          int64
	model            string
	toolIndex        int
	currentToolIndex int
	started          bool
}

// newStreamSerializer 创建一个新的 OpenAI 流式序列化器实例。
func newStreamSerializer() *openaiStreamSerializer {
	return &openaiStreamSerializer{
		id:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		created: time.Now().Unix(),
	}
}

// Serialize 将单个 StreamEvent 序列化为 OpenAI SSE 数据帧。
func (s *openaiStreamSerializer) Serialize(event bamboo.StreamEvent) ([]byte, error) {
	switch event.Type {
	case bamboo.EventMessageStart:
		return s.handleMessageStart(event)

	case bamboo.EventContentBlockStart:
		return s.handleContentBlockStart(event)

	case bamboo.EventContentBlockDelta:
		return s.handleContentBlockDelta(event)

	case bamboo.EventContentBlockStop:
		// OpenAI 没有 content_block_stop 概念，静默消费
		return nil, nil

	case bamboo.EventMessageDelta:
		return s.handleMessageDelta(event)

	case bamboo.EventMessageStop:
		// 由 Flush 处理终止标记
		return nil, nil

	case bamboo.EventPing:
		return nil, nil

	case bamboo.EventError:
		return s.handleError(event)

	default:
		return nil, nil
	}
}

// Flush 输出 SSE 终止标记。
func (s *openaiStreamSerializer) Flush() ([]byte, error) {
	return []byte("data: [DONE]\n\n"), nil
}

// handleMessageStart 处理 message_start 事件。
func (s *openaiStreamSerializer) handleMessageStart(event bamboo.StreamEvent) ([]byte, error) {
	s.started = true
	if event.Message != nil {
		// 不修改 model，model 由 message_delta 或默认值确定
	}

	chunk := openaiChunk{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: s.created,
		Choices: []openaiDelta{{
			Index: 0,
			Delta: openaiDeltaMsg{Role: "assistant"},
		}},
	}
	return s.marshalChunk(chunk)
}

// handleContentBlockStart 处理 content_block_start 事件。
func (s *openaiStreamSerializer) handleContentBlockStart(event bamboo.StreamEvent) ([]byte, error) {
	if event.ContentBlock == nil {
		return nil, nil
	}
	switch event.ContentBlock.BlockType() {
	case bamboo.ContentBlockToolUse:
		toolUse, ok := event.ContentBlock.(*bamboo.ToolUseBlock)
		if !ok {
			return nil, nil
		}
		s.currentToolIndex = s.toolIndex
		chunk := openaiChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: s.created,
			Choices: []openaiDelta{{
				Index: 0,
				Delta: openaiDeltaMsg{
					ToolCalls: []openaiDeltaTC{{
						Index: s.toolIndex,
						ID:    toolUse.ID,
						Type:  "function",
						Function: openaiDeltaTCFn{
							Name: toolUse.Name,
						},
					}},
				},
			}},
		}
		s.toolIndex++
		return s.marshalChunk(chunk)
	}
	// text/thinking 的 block_start 在 OpenAI 中不输出
	return nil, nil
}

// handleContentBlockDelta 处理 content_block_delta 事件。
func (s *openaiStreamSerializer) handleContentBlockDelta(event bamboo.StreamEvent) ([]byte, error) {
	if !s.started {
		// 如果没有 message_start，补发一个 role chunk
		s.started = true
	}

	delta, ok := event.Delta.(*bamboo.StreamDelta)
	if !ok {
		return nil, nil
	}

	switch delta.Type {
	case bamboo.DeltaTextDelta:
		chunk := openaiChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: s.created,
			Choices: []openaiDelta{{
				Index: 0,
				Delta: openaiDeltaMsg{Content: delta.Text},
			}},
		}
		return s.marshalChunk(chunk)

	case bamboo.DeltaThinkingDelta:
		chunk := openaiChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: s.created,
			Choices: []openaiDelta{{
				Index: 0,
				Delta: openaiDeltaMsg{ReasoningContent: delta.Thinking},
			}},
		}
		return s.marshalChunk(chunk)

	case bamboo.DeltaInputJSON:
		chunk := openaiChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: s.created,
			Choices: []openaiDelta{{
				Index: 0,
				Delta: openaiDeltaMsg{
					ToolCalls: []openaiDeltaTC{{
						Index:    s.currentToolIndex,
						Function: openaiDeltaTCFn{Arguments: delta.PartialJSON},
					}},
				},
			}},
		}
		return s.marshalChunk(chunk)

	case bamboo.DeltaSignature:
		// OpenAI Chat Completions 无 signature 字段，跨协议转换时记录 warning 后跳过
		log.Printf("[codec/openai] warning: signature_delta has no equivalent in OpenAI Chat Completions protocol, dropped")
		return nil, nil
	}

	return nil, nil
}

// handleMessageDelta 处理 message_delta 事件。
//
// 注意：仅当 StopReason 非空时才设置 finish_reason 字段。
// UsageDelta 会产生 StopReason 为空的 EventMessageDelta（仅携带 usage），
// 此时不应输出 finish_reason，避免产生多个终止事件。
// 真正的 finish_reason 仅由 handleStop 产生的非空 StopReason 事件决定。
func (s *openaiStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok {
		return nil, nil
	}

	chunk := openaiChunk{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: s.created,
		Choices: []openaiDelta{{
			Index: 0,
			Delta: openaiDeltaMsg{},
		}},
	}

	// 仅当 StopReason 非空时才映射 finish_reason
	if msgDelta.StopReason != "" {
		finishReason := mapFinishReason(msgDelta.StopReason)
		chunk.Choices[0].FinishReason = &finishReason
	}

	if event.Usage != nil {
		usage := &openaiStreamUsage{
			PromptTokens:     event.Usage.InputTokens,
			CompletionTokens: event.Usage.OutputTokens,
			TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
		}
		// OpenAI 无原生 cache_creation_input_tokens 字段，仅映射 CacheReadInputTokens 到 cached_tokens。
		// CacheCreationInputTokens 在跨协议转换中会丢失，此为已知限制。
		if event.Usage.CacheCreationInputTokens > 0 || event.Usage.CacheReadInputTokens > 0 {
			usage.PromptTokensDetails = &openaiPromptTokensDet{
				CachedTokens: event.Usage.CacheReadInputTokens,
			}
		}
		chunk.Usage = usage
	}

	return s.marshalChunk(chunk)
}

// handleError 处理 error 事件。
func (s *openaiStreamSerializer) handleError(event bamboo.StreamEvent) ([]byte, error) {
	errMsg := "unknown error"
	if event.Error != nil {
		errMsg = event.Error.Message
	}
	errPayload := map[string]any{
		"error": map[string]any{
			"message": errMsg,
			"type":    "api_error",
		},
	}
	data, _ := json.Marshal(errPayload)
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

// marshalChunk 将 chunk 序列化为 SSE data 行。
func (s *openaiStreamSerializer) marshalChunk(chunk openaiChunk) ([]byte, error) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal stream chunk: %w", err)
	}
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

package openai

import (
	"encoding/json"
	"fmt"
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
}

type openaiDelta struct {
	Index        int            `json:"index"`
	Delta        openaiDeltaMsg `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}

type openaiDeltaMsg struct {
	Role             string           `json:"role,omitempty"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiDeltaTC  `json:"tool_calls,omitempty"`
}

type openaiDeltaTC struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function openaiDeltaTCFn  `json:"function"`
}

type openaiDeltaTCFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// openaiStreamSerializer OpenAI 流式序列化器，有状态。
type openaiStreamSerializer struct {
	id        string
	created   int64
	model     string
	toolIndex int
	started   bool
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
						Index:    event.Index,
						Function: openaiDeltaTCFn{Arguments: delta.PartialJSON},
					}},
				},
			}},
		}
		return s.marshalChunk(chunk)
	}

	return nil, nil
}

// handleMessageDelta 处理 message_delta 事件。
func (s *openaiStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok {
		return nil, nil
	}

	finishReason := mapFinishReason(msgDelta.StopReason)
	chunk := openaiChunk{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: s.created,
		Choices: []openaiDelta{{
			Index:        0,
			Delta:        openaiDeltaMsg{},
			FinishReason: &finishReason,
		}},
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

package openai

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

// openaiChunk OpenAI 流式 chunk JSON 结构。
type openaiChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []openaiDelta      `json:"choices"`
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
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	// ReasoningContent 思考/推理内容增量。
	// 注意：此字段为 DeepSeek/vLLM 兼容性扩展，非官方 OpenAI Chat Completions 规范字段。
	ReasoningContent  string          `json:"reasoning_content,omitempty"`
	ThinkingSignature string          `json:"thinking_signature,omitempty"`
	ThinkingProvider  string          `json:"thinking_provider,omitempty"`
	ReasoningID       string          `json:"reasoning_id,omitempty"`
	ToolCalls         []openaiDeltaTC `json:"tool_calls,omitempty"`
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
func newStreamSerializer(model string) *openaiStreamSerializer {
	return &openaiStreamSerializer{
		id:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		created: time.Now().Unix(),
		model:   model,
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
		return []byte(": keep-alive\n\n"), nil

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
		if delta.Signature == "" && delta.SignatureProvider == "" {
			return nil, nil
		}
		chunk := openaiChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: s.created,
			Choices: []openaiDelta{{
				Index: 0,
				Delta: openaiDeltaMsg{
					ThinkingSignature: delta.Signature,
					ThinkingProvider:  delta.SignatureProvider,
				},
			}},
		}
		return s.marshalChunk(chunk)
	}

	return nil, nil
}

// handleMessageDelta 处理 message_delta 事件。
//
// 行为规范（对齐真实 OpenAI API）：
//   - finish_reason 和 usage 必须分离为独立的 SSE data 行；
//   - finish_reason chunk: {"choices":[{"index":0,"delta":{},"finish_reason":"..."}]}
//   - usage chunk:         {"choices":[],"usage":{...}}
//   - 仅 StopReason 非空时才输出 finish_reason chunk；
//   - 仅 Usage 非空时才输出 usage chunk；
//   - 两者皆空时返回 nil（不产生任何输出）。
//
// 背景：UsageDelta 会产生 StopReason 为空的 EventMessageDelta（仅携带 usage），
// 不应输出 finish_reason，避免产生多个终止事件。真正的 finish_reason 仅由
// 非空 StopReason 决定。
func (s *openaiStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok {
		return nil, nil
	}

	var chunks []openaiChunk

	// finish_reason chunk（仅当 StopReason 非空）
	if msgDelta.StopReason != "" {
		finishReason := mapFinishReason(msgDelta.StopReason)
		chunks = append(chunks, openaiChunk{
			Choices: []openaiDelta{{
				Index:        0,
				Delta:        openaiDeltaMsg{},
				FinishReason: &finishReason,
			}},
		})
	}

	// usage chunk（仅当 Usage 非空），choices 为空数组
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
		chunks = append(chunks, openaiChunk{
			Choices: []openaiDelta{},
			Usage:   usage,
		})
	}

	if len(chunks) == 0 {
		return nil, nil
	}
	return s.marshalChunks(chunks)
}

// handleError 处理 error 事件。
func (s *openaiStreamSerializer) handleError(event bamboo.StreamEvent) ([]byte, error) {
	errMsg := "unknown error"
	errType := "api_error"
	if event.Error != nil {
		errMsg = event.Error.Message
		errType = mapStatusCodeToOpenAIType(event.Error.StatusCode)
	}
	errPayload := map[string]any{
		"error": map[string]any{
			"message": errMsg,
			"type":    errType,
			"code":    errType,
		},
	}
	data, _ := json.Marshal(errPayload)
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

// marshalChunk 将单个 chunk 序列化为 SSE data 行。
//
// 统一注入 s.model / s.id / s.created / object，保证所有 chunk 携带完整的元信息，
// 与真实 OpenAI API 行为一致（每个 chunk 都含 model 字段）。
func (s *openaiStreamSerializer) marshalChunk(chunk openaiChunk) ([]byte, error) {
	chunk.ID = s.id
	chunk.Object = "chat.completion.chunk"
	chunk.Created = s.created
	chunk.Model = s.model
	data, err := json.Marshal(chunk)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal stream chunk: %v", err), 0)
	}
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

// marshalChunks 将多个 chunk 序列化为连续的 SSE data 行。
//
// 用于 message_delta 事件中 finish_reason 和 usage 分离输出的场景：
// 真实 OpenAI API 将 finish_reason 和 usage 拆分为两个独立的 chunk，
// 分别为 {"choices":[{"index":0,"delta":{},"finish_reason":"..."}]} 和
// {"choices":[],"usage":{...}}。
func (s *openaiStreamSerializer) marshalChunks(chunks []openaiChunk) ([]byte, error) {
	var output []byte
	for _, c := range chunks {
		data, err := s.marshalChunk(c)
		if err != nil {
			return nil, err
		}
		output = append(output, data...)
	}
	return output, nil
}

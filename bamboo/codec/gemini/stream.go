package gemini

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── Gemini 流式 chunk JSON 结构（复用 response.go 的部分类型）──

// geminiStreamChunk 流式 GenerateContentResponse。
//
// 与非流式结构一致，但每个 chunk 是独立的 JSON 对象（SSE `data: {json}\n\n`）。
type geminiStreamChunk struct {
	Candidates    []geminiStreamCandidate `json:"candidates,omitempty"`
	UsageMetadata *geminiUsageMeta        `json:"usageMetadata,omitempty"`
}

// geminiStreamCandidate 流式候选。
type geminiStreamCandidate struct {
	Content      *geminiContentOut `json:"content,omitempty"`
	FinishReason string            `json:"finishReason,omitempty"`
	Index        int               `json:"index"`
}

// geminiStreamError 流式错误响应。
type geminiStreamError struct {
	Error geminiStreamErrorBody `json:"error"`
}

type geminiStreamErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ── StreamSerializer 实现 ──

// geminiStreamSerializer Gemini 流式序列化器，有状态。
//
// 核心差异：Gemini 的 functionCall.args 是完整 JSON 对象，不分增量。
// 当收到 input_json_delta 时必须累积到 pendingCallArgs，
// 在 content_block_stop 时一次性输出完整的 functionCall。
type geminiStreamSerializer struct {
	model           string
	pendingCallName string
	pendingCallID   string
	pendingCallArgs strings.Builder
	accumulating    bool
	inputTokens     int64
	outputTokens    int64
	cacheReadTokens int64
	started         bool
}

// newStreamSerializer 创建一个新的 Gemini 流式序列化器实例。
func newStreamSerializer(model string) *geminiStreamSerializer {
	return &geminiStreamSerializer{
		model: model,
	}
}

// Serialize 将单个 StreamEvent 序列化为 Gemini SSE 数据帧。
func (s *geminiStreamSerializer) Serialize(event bamboo.StreamEvent) ([]byte, error) {
	switch event.Type {
	case bamboo.EventMessageStart:
		return s.handleMessageStart(event)

	case bamboo.EventContentBlockStart:
		return s.handleContentBlockStart(event)

	case bamboo.EventContentBlockDelta:
		return s.handleContentBlockDelta(event)

	case bamboo.EventContentBlockStop:
		return s.handleContentBlockStop(event)

	case bamboo.EventMessageDelta:
		return s.handleMessageDelta(event)

	case bamboo.EventMessageStop:
		return nil, nil

	case bamboo.EventPing:
		return []byte(": keep-alive\n\n"), nil

	case bamboo.EventError:
		return s.handleError(event)

	default:
		return nil, nil
	}
}

// Flush 刷新缓冲区。
//
// Gemini 流式没有特殊的终止标记（不像 OpenAI 的 [DONE]），
// 返回 nil 即可。
func (s *geminiStreamSerializer) Flush() ([]byte, error) {
	return nil, nil
}

// handleMessageStart 处理 message_start 事件。
//
// 记录 model 和 inputTokens，不输出数据帧（等待第一个 delta）。
func (s *geminiStreamSerializer) handleMessageStart(event bamboo.StreamEvent) ([]byte, error) {
	s.started = true
	if event.Message != nil {
		// 从 message 中提取 model（如果有）
		// message_start 通常不携带 model，由 message_delta 携带
	}
	if event.Usage != nil {
		s.inputTokens = event.Usage.InputTokens
		s.cacheReadTokens = event.Usage.CacheReadInputTokens
	}
	return nil, nil
}

// handleContentBlockStart 处理 content_block_start 事件。
//
// 对于 tool_use 类型，记录 name 和 id，开启累积模式。
// text 和 thinking 类型不输出（Gemini 没有 block_start 概念）。
func (s *geminiStreamSerializer) handleContentBlockStart(event bamboo.StreamEvent) ([]byte, error) {
	if event.ContentBlock == nil {
		return nil, nil
	}

	switch event.ContentBlock.BlockType() {
	case bamboo.ContentBlockToolUse:
		if toolUse, ok := event.ContentBlock.(*bamboo.ToolUseBlock); ok {
			s.pendingCallName = toolUse.Name
			s.pendingCallID = toolUse.ID
			s.accumulating = true
			s.pendingCallArgs.Reset()
		}
		// 不输出，等待 content_block_stop
		return nil, nil

	case bamboo.ContentBlockText, bamboo.ContentBlockThinking:
		// 不输出
		return nil, nil
	}

	return nil, nil
}

// handleContentBlockDelta 处理 content_block_delta 事件。
//
// 根据增量类型分发：
//   - text_delta     → {candidates:[{content:{role:"model",parts:[{text}]}}]}
//   - thinking_delta → {candidates:[{content:{parts:[{text, thought:true}]}}]}
//   - input_json_delta → 累积到 pendingCallArgs，不输出
func (s *geminiStreamSerializer) handleContentBlockDelta(event bamboo.StreamEvent) ([]byte, error) {
	delta, ok := event.Delta.(*bamboo.StreamDelta)
	if !ok {
		return nil, nil
	}

	switch delta.Type {
	case bamboo.DeltaTextDelta:
		chunk := geminiStreamChunk{
			Candidates: []geminiStreamCandidate{{
				Index: 0,
				Content: &geminiContentOut{
					Role:  "model",
					Parts: []geminiPartOut{{Text: delta.Text}},
				},
			}},
		}
		return s.marshalChunk(chunk)

	case bamboo.DeltaThinkingDelta:
		chunk := geminiStreamChunk{
			Candidates: []geminiStreamCandidate{{
				Index: 0,
				Content: &geminiContentOut{
					Role: "model",
					Parts: []geminiPartOut{{
						Text:    delta.Thinking,
						Thought: true,
					}},
				},
			}},
		}
		return s.marshalChunk(chunk)

	case bamboo.DeltaInputJSON:
		// 累积 functionCall 参数，不输出
		if s.accumulating {
			s.pendingCallArgs.WriteString(delta.PartialJSON)
		}
		return nil, nil

	case bamboo.DeltaSignature:
		log.Printf("[codec/gemini] warning: signature_delta has no equivalent in Gemini protocol, dropped")
		return nil, nil
	}

	return nil, nil
}

// handleContentBlockStop 处理 content_block_stop 事件。
//
// 如果正在累积 functionCall 参数，在此时一次性输出完整的 functionCall。
func (s *geminiStreamSerializer) handleContentBlockStop(event bamboo.StreamEvent) ([]byte, error) {
	if !s.accumulating {
		return nil, nil
	}

	// 解析累积的参数 JSON
	argsRaw := json.RawMessage(s.pendingCallArgs.String())
	if len(argsRaw) == 0 {
		argsRaw = json.RawMessage(`{}`)
	}

	// 构建 functionCall part
	part := geminiPartOut{
		FunctionCall: &geminiFuncCallOut{
			Name: s.pendingCallName,
			Args: argsRaw,
		},
	}
	if s.pendingCallID != "" {
		part.FunctionCall.ID = s.pendingCallID
	}

	chunk := geminiStreamChunk{
		Candidates: []geminiStreamCandidate{{
			Index: 0,
			Content: &geminiContentOut{
				Role:  "model",
				Parts: []geminiPartOut{part},
			},
		}},
	}

	// 重置累积器
	s.accumulating = false
	s.pendingCallName = ""
	s.pendingCallID = ""
	s.pendingCallArgs.Reset()

	return s.marshalChunk(chunk)
}

// handleMessageDelta 处理 message_delta 事件。
//
// 输出 finishReason 和 usageMetadata。
func (s *geminiStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok {
		return nil, nil
	}

	finishReason := mapFinishReasonToGemini(msgDelta.StopReason)

	// 提取 usage（如果有）
	if event.Usage != nil {
		s.outputTokens = event.Usage.OutputTokens
		if event.Usage.InputTokens > 0 {
			s.inputTokens = event.Usage.InputTokens
		}
		if event.Usage.CacheReadInputTokens > 0 {
			s.cacheReadTokens = event.Usage.CacheReadInputTokens
		}
	}

	// Gemini 无原生 cache_creation_input_tokens 字段，仅映射 CacheReadInputTokens 到 cachedContentTokenCount。
	// CacheCreationInputTokens 在跨协议转换中会丢失，此为已知限制。
	chunk := geminiStreamChunk{
		Candidates: []geminiStreamCandidate{{
			Index:        0,
			FinishReason: finishReason,
		}},
		UsageMetadata: &geminiUsageMeta{
			PromptTokenCount:        s.inputTokens,
			CandidatesTokenCount:    s.outputTokens,
			TotalTokenCount:         s.inputTokens + s.outputTokens,
			CachedContentTokenCount: s.cacheReadTokens,
		},
	}
	return s.marshalChunk(chunk)
}

// handleError 处理 error 事件。
func (s *geminiStreamSerializer) handleError(event bamboo.StreamEvent) ([]byte, error) {
	code := 500
	status := "INTERNAL"
	message := "unknown error"

	if event.Error != nil {
		message = event.Error.Message
		code, status = mapBambooErrorToGemini(event.Error)
	}

	payload := geminiStreamError{
		Error: geminiStreamErrorBody{
			Code:    code,
			Message: message,
			Status:  status,
		},
	}
	data, _ := json.Marshal(payload)
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

// marshalChunk 将 chunk 序列化为 SSE data 行。
func (s *geminiStreamSerializer) marshalChunk(chunk geminiStreamChunk) ([]byte, error) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini stream chunk: %w", err)
	}
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

// mapBambooErrorToGemini 将 BambooError 类型映射为 Gemini error code + status。
func mapBambooErrorToGemini(err *bamboo.BambooError) (int, string) {
	switch err.Type {
	case bamboo.ErrorTypeInvalidRequest:
		return 400, "INVALID_ARGUMENT"
	case bamboo.ErrorTypeAuthentication:
		return 401, "UNAUTHENTICATED"
	case bamboo.ErrorTypeRateLimit:
		return 429, "RESOURCE_EXHAUSTED"
	case bamboo.ErrorTypeAPI, bamboo.ErrorTypeProvider:
		return 500, "INTERNAL"
	default:
		return 500, "INTERNAL"
	}
}

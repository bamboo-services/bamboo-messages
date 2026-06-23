package responses

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── OpenAI Responses 流式事件 JSON 结构体 ──

// responseObj response.created / response.completed 中的 response 对象。
type responseObj struct {
	ID     string          `json:"id"`
	Object string          `json:"object"`
	Status string          `json:"status,omitempty"`
	Model  string          `json:"model,omitempty"`
	Output []outputItem    `json:"output,omitempty"`
	Usage  *responsesUsage `json:"usage,omitempty"`
	Error  *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

// outputItemAdded response.output_item.added 事件。
type outputItemAdded struct {
	OutputIndex int        `json:"output_index"`
	Item        outputItem `json:"item"`
}

// textDelta response.output_text.delta 事件。
type textDeltaEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// textDone response.output_text.done 事件。
type textDoneEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

// reasoningDelta response.reasoning_text.delta 事件。
type reasoningDeltaEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// reasoningDone response.reasoning_text.done 事件。
type reasoningDoneEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

// functionCallArgsDelta response.function_call_arguments.delta 事件。
type functionCallArgsDelta struct {
	OutputIndex int    `json:"output_index"`
	Delta       string `json:"delta"`
}

// functionCallArgsDone response.function_call_arguments.done 事件。
type functionCallArgsDone struct {
	OutputIndex int    `json:"output_index"`
	Arguments   string `json:"arguments"`
}

// responsesStreamSerializer OpenAI Responses 流式序列化器（有状态）。
//
// Responses 的流式格式比 Chat Completions 复杂得多：
//   - 文本内容合并到一个 message output 项目
//   - ThinkingBlock / ToolUseBlock 各自独立为 output 项目
//   - 每种内容块有对应的 output_item.added / *.delta / *.done 事件
//   - 流以 response.created 开头、response.completed 结尾
type responsesStreamSerializer struct {
	// response 元数据
	responseID string
	model      string
	createdAt  int64

	// message output 项目状态（所有 TextBlock 合并到一个 message 项目）
	messageItemID string
	messageAdded  bool
	messageIndex  int // message 项目的 output_index
	messageText   strings.Builder

	// reasoning output 项目状态
	reasoningText   strings.Builder
	reasoningItemID string
	reasoningIndex  int

	// function_call output 项目状态
	currentCallID     string
	currentCallName   string
	currentCallArgs   strings.Builder
	functionCallItem  string // 当前 function_call item ID
	functionCallIdx   int    // 当前 function_call 的 output_index
	functionCallCount int    // 已处理的 function_call 总数

	// output_index 全局计数器
	outputIndexCounter int

	// 是否已发送 response.created
	created bool

	// 是否已发送 response.completed（防止重复发送）
	completedSent bool

	// 最后一次 content_block_start 的 block 类型，用于 content_block_stop 分发
	lastBlockTypeStr string
}

// newStreamSerializer 创建一个新的 Responses 流式序列化器实例。
func newStreamSerializer() *responsesStreamSerializer {
	return &responsesStreamSerializer{
		responseID: fmt.Sprintf("resp-%d", time.Now().UnixNano()),
		createdAt:  time.Now().Unix(),
	}
}

// Serialize 将单个 StreamEvent 序列化为 OpenAI Responses SSE 数据帧。
func (s *responsesStreamSerializer) Serialize(event bamboo.StreamEvent) ([]byte, error) {
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
		// 由 Flush 或 response.completed 覆盖
		return nil, nil

	case bamboo.EventPing:
		return nil, nil

	case bamboo.EventError:
		return s.handleError(event)

	default:
		return nil, nil
	}
}

// Flush 在流结束时调用。
//
// Responses 流由 response.completed 事件终结（在 message_delta 时已发出），
// Flush 不需要额外输出。
func (s *responsesStreamSerializer) Flush() ([]byte, error) {
	return nil, nil
}

// handleMessageStart 处理 message_start 事件 → response.created。
func (s *responsesStreamSerializer) handleMessageStart(event bamboo.StreamEvent) ([]byte, error) {
	s.created = true

	// 从 message 中提取 ID（如果有）
	if event.Message != nil {
		// message_start 不携带 ID，使用默认值
	}

	resp := responseObj{
		ID:     s.responseID,
		Object: "response",
		Status: "in_progress",
		Model:  s.model,
		Output: []outputItem{},
	}

	return s.marshalSSEWithResponse("response.created", resp)
}

// handleContentBlockStart 处理 content_block_start 事件。
//
// 根据 ContentBlock 类型分发到 message / reasoning / function_call 的 output_item.added。
func (s *responsesStreamSerializer) handleContentBlockStart(event bamboo.StreamEvent) ([]byte, error) {
	if event.ContentBlock == nil {
		return nil, nil
	}

	switch event.ContentBlock.BlockType() {
	case bamboo.ContentBlockText:
		s.lastBlockTypeStr = "text"
		if !s.messageAdded {
			s.messageAdded = true
			s.messageIndex = s.outputIndexCounter
			s.outputIndexCounter++
			s.messageItemID = fmt.Sprintf("msg-%d", time.Now().UnixNano())

			item := outputItem{
				Type:   "message",
				ID:     s.messageItemID,
				Role:   "assistant",
				Status: "in_progress",
			}
			added := outputItemAdded{OutputIndex: s.messageIndex, Item: item}
			return s.marshalSSE("response.output_item.added", added)
		}
		return nil, nil

	case bamboo.ContentBlockThinking:
		s.lastBlockTypeStr = "thinking"
		s.reasoningIndex = s.outputIndexCounter
		s.outputIndexCounter++
		s.reasoningItemID = fmt.Sprintf("rs-%d", time.Now().UnixNano())

		item := outputItem{
			Type:    "reasoning",
			ID:      s.reasoningItemID,
			Summary: []outputReasoningSummary{},
		}
		added := outputItemAdded{OutputIndex: s.reasoningIndex, Item: item}
		return s.marshalSSE("response.output_item.added", added)

	case bamboo.ContentBlockToolUse:
		s.lastBlockTypeStr = "tool_use"
		toolUse, ok := event.ContentBlock.(*bamboo.ToolUseBlock)
		if !ok {
			return nil, nil
		}
		s.currentCallID = toolUse.ID
		s.currentCallName = toolUse.Name
		s.currentCallArgs.Reset()
		s.functionCallItem = fmt.Sprintf("fc-%d", time.Now().UnixNano())
		s.functionCallIdx = s.outputIndexCounter
		s.outputIndexCounter++
		s.functionCallCount++

		item := outputItem{
			Type:      "function_call",
			ID:        s.functionCallItem,
			CallID:    toolUse.ID,
			Name:      toolUse.Name,
			Arguments: "",
		}
		added := outputItemAdded{OutputIndex: s.functionCallIdx, Item: item}
		return s.marshalSSE("response.output_item.added", added)
	}

	return nil, nil
}

// handleContentBlockDelta 处理 content_block_delta 事件。
func (s *responsesStreamSerializer) handleContentBlockDelta(event bamboo.StreamEvent) ([]byte, error) {
	// 如果还没有发送 response.created，先补发
	if !s.created {
		s.created = true
	}

	delta, ok := event.Delta.(*bamboo.StreamDelta)
	if !ok {
		return nil, nil
	}

	switch delta.Type {
	case bamboo.DeltaTextDelta:
		// 确保 message 项目已添加
		if !s.messageAdded {
			s.messageAdded = true
			s.messageIndex = s.outputIndexCounter
			s.outputIndexCounter++
			s.messageItemID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
		}
		s.messageText.WriteString(delta.Text)

		ev := textDeltaEvent{
			OutputIndex:  s.messageIndex,
			ContentIndex: 0,
			Delta:        delta.Text,
		}
		return s.marshalSSE("response.output_text.delta", ev)

	case bamboo.DeltaThinkingDelta:
		s.reasoningItemID = s.ensureReasoningItem()
		s.reasoningText.WriteString(delta.Thinking)

		ev := reasoningDeltaEvent{
			OutputIndex:  s.reasoningIndex,
			ContentIndex: 0,
			Delta:        delta.Thinking,
		}
		return s.marshalSSE("response.reasoning_text.delta", ev)

	case bamboo.DeltaInputJSON:
		// function_call 参数增量
		s.currentCallArgs.WriteString(delta.PartialJSON)

		ev := functionCallArgsDelta{
			OutputIndex: s.functionCallIdx,
			Delta:       delta.PartialJSON,
		}
		return s.marshalSSE("response.function_call_arguments.delta", ev)

	case bamboo.DeltaSignature:
		// signature_delta 为 Anthropic Extended Thinking 特有的签名增量，
		// OpenAI Responses 协议无对应字段，跨协议转换时丢弃。
		return nil, nil
	}

	return nil, nil
}

// handleContentBlockStop 处理 content_block_stop 事件。
//
// 每种内容块先发送对应的 content-done 事件（output_text.done / reasoning_text.done / function_call_arguments.done），
// 再追加 output_item.done 事件（携带完整 item，status 为 "completed"）。两个 SSE 帧拼接到同一个 []byte 返回。
func (s *responsesStreamSerializer) handleContentBlockStop(event bamboo.StreamEvent) ([]byte, error) {
	switch s.lastBlockTypeStr {
	case "text":
		if s.messageAdded {
			// 1. output_text.done
			ev := textDoneEvent{
				OutputIndex:  s.messageIndex,
				ContentIndex: 0,
				Text:         s.messageText.String(),
			}
			contentDoneBytes, err := s.marshalSSE("response.output_text.done", ev)
			if err != nil {
				return nil, err
			}

			// 2. output_item.done（message item，status=completed，携带完整 content）
			item := outputItem{
				Type:   "message",
				ID:     s.messageItemID,
				Role:   "assistant",
				Status: "completed",
				Content: []outputContent{
					{Type: "output_text", Text: s.messageText.String()},
				},
			}
			done := outputItemAdded{OutputIndex: s.messageIndex, Item: item}
			outputItemDoneBytes, err := s.marshalSSE("response.output_item.done", done)
			if err != nil {
				return nil, err
			}

			return append(contentDoneBytes, outputItemDoneBytes...), nil
		}

	case "thinking":
		// 1. reasoning_text.done
		ev := reasoningDoneEvent{
			OutputIndex:  s.reasoningIndex,
			ContentIndex: 0,
			Text:         s.reasoningText.String(),
		}
		contentDoneBytes, err := s.marshalSSE("response.reasoning_text.done", ev)
		if err != nil {
			return nil, err
		}

		// 2. output_item.done（reasoning item，status=completed，携带完整 content）
		item := outputItem{
			Type:   "reasoning",
			ID:     s.reasoningItemID,
			Status: "completed",
			Content: []outputContent{
				{Type: "reasoning_text", Text: s.reasoningText.String()},
			},
			Summary: []outputReasoningSummary{},
		}
		done := outputItemAdded{OutputIndex: s.reasoningIndex, Item: item}
		outputItemDoneBytes, err := s.marshalSSE("response.output_item.done", done)
		if err != nil {
			return nil, err
		}

		return append(contentDoneBytes, outputItemDoneBytes...), nil

	case "tool_use":
		// 1. function_call_arguments.done
		ev := functionCallArgsDone{
			OutputIndex: s.functionCallIdx,
			Arguments:   s.currentCallArgs.String(),
		}
		contentDoneBytes, err := s.marshalSSE("response.function_call_arguments.done", ev)
		if err != nil {
			return nil, err
		}

		// 2. output_item.done（function_call item，status=completed）
		item := outputItem{
			Type:      "function_call",
			ID:        s.functionCallItem,
			CallID:    s.currentCallID,
			Name:      s.currentCallName,
			Arguments: s.currentCallArgs.String(),
			Status:    "completed",
		}
		done := outputItemAdded{OutputIndex: s.functionCallIdx, Item: item}
		outputItemDoneBytes, err := s.marshalSSE("response.output_item.done", done)
		if err != nil {
			return nil, err
		}

		return append(contentDoneBytes, outputItemDoneBytes...), nil
	}

	return nil, nil
}

// handleMessageDelta 处理 message_delta 事件 → response.completed。
func (s *responsesStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	if s.completedSent {
		return nil, nil
	}
	s.completedSent = true

	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok {
		// 仍然发送 response.completed
	}

	status := "completed"
	if ok && msgDelta.StopReason == bamboo.FinishReasonMaxTokens {
		status = "incomplete"
	}

	usage := &responsesUsage{}
	if event.Usage != nil {
		usage = &responsesUsage{
			InputTokens:  event.Usage.InputTokens,
			OutputTokens: event.Usage.OutputTokens,
			TotalTokens:  event.Usage.InputTokens + event.Usage.OutputTokens,
		}
		// Responses 无原生 cache_creation_input_tokens 字段，仅映射 CacheReadInputTokens 到 cached_tokens。
		// CacheCreationInputTokens 在跨协议转换中会丢失，此为已知限制。
		if event.Usage.CacheCreationInputTokens > 0 || event.Usage.CacheReadInputTokens > 0 {
			usage.InputTokensDetails = &responsesInputTokensDet{
				CachedTokens: event.Usage.CacheReadInputTokens,
			}
		}
	}

	resp := responseObj{
		ID:     s.responseID,
		Object: "response",
		Status: status,
		Model:  s.model,
		Usage:  usage,
	}

	return s.marshalSSEWithResponse("response.completed", resp)
}

// handleError 处理 error 事件 → response.failed。
func (s *responsesStreamSerializer) handleError(event bamboo.StreamEvent) ([]byte, error) {
	errMsg := "unknown error"
	errType := "server_error"
	if event.Error != nil {
		errMsg = event.Error.Message
		errType = event.Error.Type
	}

	resp := responseObj{
		ID: s.responseID,
		Error: &responseError{
			Message: errMsg,
			Type:    errType,
		},
	}

	return s.marshalSSEWithResponse("response.failed", resp)
}

// ── 内部辅助方法 ──

// ensureReasoningItem 确保 reasoning output 项目已添加，返回 item ID。
func (s *responsesStreamSerializer) ensureReasoningItem() string {
	if s.reasoningItemID != "" {
		return s.reasoningItemID
	}
	s.reasoningIndex = s.outputIndexCounter
	s.outputIndexCounter++
	s.reasoningItemID = fmt.Sprintf("rs-%d", time.Now().UnixNano())
	return s.reasoningItemID
}

// marshalSSE 将事件序列化为 SSE 数据帧，并在 JSON payload 中注入 `"type": eventType` 字段。
//
// 输出格式：
//
//	event: {type}
//	data: {"type":"{eventType}",...payloadFields}
//
// 参考 bamboo/relay/smooth_parser.go 的 buildResponsesDeltaFrame 模式：
// codex 的 Rust SSE 解析器依赖 data JSON 的 "type" 字段路由事件。
func (s *responsesStreamSerializer) marshalSSE(eventType string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s event: %w", eventType, err)
	}

	// 将 payload 反序列化为 map 以注入 type 字段（保持原字段完整）
	merged := make(map[string]any, 8)
	if err := json.Unmarshal(raw, &merged); err != nil {
		// 非 object payload（理论上不会发生），回退为直接包装
		return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, raw)), nil
	}
	merged["type"] = eventType

	data, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal %s event with type: %w", eventType, err)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}

// marshalSSEWithResponse 将生命周期事件序列化为带 response 嵌套包装的 SSE 数据帧。
//
// 输出格式（codex serde_json::from_value::<ResponseCompleted> 期望的嵌套结构）：
//
//	event: {eventType}
//	data: {"type":"{eventType}","response":{...responseObj}}
func (s *responsesStreamSerializer) marshalSSEWithResponse(eventType string, resp responseObj) ([]byte, error) {
	respRaw, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response for %s event: %w", eventType, err)
	}

	var respMap map[string]any
	if err := json.Unmarshal(respRaw, &respMap); err != nil {
		return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, respRaw)), nil
	}

	payload := map[string]any{
		"type":     eventType,
		"response": respMap,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s event payload: %w", eventType, err)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}

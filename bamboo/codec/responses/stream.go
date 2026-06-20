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

	return s.marshalSSE("response.created", resp)
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
			Type: "reasoning",
			ID:   s.reasoningItemID,
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
	}

	return nil, nil
}

// handleContentBlockStop 处理 content_block_stop 事件。
func (s *responsesStreamSerializer) handleContentBlockStop(event bamboo.StreamEvent) ([]byte, error) {
	switch s.lastBlockTypeStr {
	case "text":
		if s.messageAdded {
			ev := textDoneEvent{
				OutputIndex:  s.messageIndex,
				ContentIndex: 0,
				Text:         s.messageText.String(),
			}
			return s.marshalSSE("response.output_text.done", ev)
		}
	case "thinking":
		ev := reasoningDoneEvent{
			OutputIndex:  s.reasoningIndex,
			ContentIndex: 0,
			Text:         "",
		}
		return s.marshalSSE("response.reasoning_text.done", ev)
	case "tool_use":
		ev := functionCallArgsDone{
			OutputIndex: s.functionCallIdx,
			Arguments:   s.currentCallArgs.String(),
		}
		return s.marshalSSE("response.function_call_arguments.done", ev)
	}

	return nil, nil
}

// handleMessageDelta 处理 message_delta 事件 → response.completed。
func (s *responsesStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok {
		// 仍然发送 response.completed
	}

	status := "completed"
	if ok && msgDelta.StopReason == bamboo.FinishReasonMaxTokens {
		status = "incomplete"
	}

	var usage *responsesUsage
	if event.Usage != nil {
		u := &responsesUsage{
			InputTokens:  event.Usage.InputTokens,
			OutputTokens: event.Usage.OutputTokens,
		}
		if event.Usage.CacheCreationInputTokens > 0 || event.Usage.CacheReadInputTokens > 0 {
			u.InputTokensDetails = &responsesInputTokensDet{
				CachedTokens: event.Usage.CacheReadInputTokens,
			}
		}
		usage = u
	}

	resp := responseObj{
		ID:     s.responseID,
		Object: "response",
		Status: status,
		Model:  s.model,
		Usage:  usage,
	}

	return s.marshalSSE("response.completed", resp)
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

	return s.marshalSSE("response.failed", resp)
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

// marshalSSE 将事件序列化为 SSE 数据帧。
//
// 格式：`event: {type}\ndata: {json}\n\n`
func (s *responsesStreamSerializer) marshalSSE(eventType string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s event: %w", eventType, err)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}

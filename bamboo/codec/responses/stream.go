package responses

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

type responseObj struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	CreatedAt int64           `json:"created_at"`
	Status    string          `json:"status"`
	Model     string          `json:"model"`
	Output    []outputItem    `json:"output"`
	Usage     *responsesUsage `json:"usage,omitempty"`
	Error     *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	// Code 不能 omitempty：Grok Build 等严格客户端把 error.code 当成必填 string。
	Code string `json:"code"`
}

type outputItemAdded struct {
	OutputIndex int        `json:"output_index"`
	Item        outputItem `json:"item"`
}

type textDeltaEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	ItemID       string `json:"item_id"`
	Delta        string `json:"delta"`
	Logprobs     []any  `json:"logprobs"`
}

type textDoneEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	ItemID       string `json:"item_id"`
	Text         string `json:"text"`
	Logprobs     []any  `json:"logprobs"`
}

type reasoningDeltaEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	ItemID       string `json:"item_id"`
	Delta        string `json:"delta"`
}

type reasoningDoneEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	ItemID       string `json:"item_id"`
	Text         string `json:"text"`
}

// reasoningSummaryDeltaEvent 是 response.reasoning_summary_text.delta 的 payload。
//
// summary_index 不能 omitempty：Codex / OpenAI SDK 把它当必填 int，缺字段会直接反序列化失败。
type reasoningSummaryDeltaEvent struct {
	OutputIndex  int    `json:"output_index"`
	SummaryIndex int    `json:"summary_index"`
	ItemID       string `json:"item_id"`
	Delta        string `json:"delta"`
}

type reasoningSummaryDoneEvent struct {
	OutputIndex  int    `json:"output_index"`
	SummaryIndex int    `json:"summary_index"`
	ItemID       string `json:"item_id"`
	Text         string `json:"text"`
}

type reasoningSummaryPartEvent struct {
	OutputIndex  int    `json:"output_index"`
	SummaryIndex int    `json:"summary_index"`
	ItemID       string `json:"item_id"`
	Part         any    `json:"part"`
}

type functionCallArgsDelta struct {
	OutputIndex int    `json:"output_index"`
	ItemID      string `json:"item_id"`
	CallID      string `json:"call_id"`
	Delta       string `json:"delta"`
}

type functionCallArgsDone struct {
	OutputIndex int    `json:"output_index"`
	ItemID      string `json:"item_id"`
	CallID      string `json:"call_id"`
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
}

type contentPartAddedEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	ItemID       string `json:"item_id"`
	Part         any    `json:"part"`
}

type contentPartDoneEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	ItemID       string `json:"item_id"`
	Part         any    `json:"part"`
}

type responsesStreamSerializer struct {
	responseID string
	model      string
	createdAt  int64

	sequenceNumber int64

	outputIndexCounter int
	completedOutput    []outputItem

	messageItemID string
	messageAdded  bool
	messageIndex  int
	messageText   strings.Builder

	reasoningText   strings.Builder
	reasoningItemID string
	reasoningIndex  int

	currentCallID    string
	currentCallName  string
	currentCallArgs  strings.Builder
	functionCallItem string
	functionCallIdx  int

	blocks map[int]*responsesStreamBlock

	created       bool
	completedSent bool
}

type responsesStreamBlock struct {
	kind             string
	outputIndex      int
	itemID           string
	callID           string
	name             string
	args             strings.Builder
	encryptedContent string
}

func newStreamSerializer(model string) *responsesStreamSerializer {
	return &responsesStreamSerializer{
		responseID: fmt.Sprintf("resp_%d", time.Now().UnixNano()),
		createdAt:  time.Now().Unix(),
		model:      model,
		blocks:     make(map[int]*responsesStreamBlock),
	}
}

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
		return nil, nil
	case bamboo.EventPing:
		return []byte(": keep-alive\n\n"), nil
	case bamboo.EventError:
		return s.handleError(event)
	default:
		return nil, nil
	}
}

func (s *responsesStreamSerializer) Flush() ([]byte, error) {
	return nil, nil
}

func (s *responsesStreamSerializer) handleMessageStart(event bamboo.StreamEvent) ([]byte, error) {
	if md, ok := event.Delta.(*bamboo.MessageDelta); ok {
		s.applyReasoningMeta(md)
	}
	return s.ensureCreated()
}

func (s *responsesStreamSerializer) ensureCreated() ([]byte, error) {
	if s.created {
		return nil, nil
	}
	s.created = true
	resp := responseObj{
		ID:        s.responseID,
		Object:    "response",
		CreatedAt: s.createdAt,
		Status:    "in_progress",
		Model:     s.model,
		Output:    []outputItem{},
	}
	return s.marshalSSEWithResponse("response.created", resp)
}

func (s *responsesStreamSerializer) applyReasoningMeta(md *bamboo.MessageDelta) {
	if md == nil {
		return
	}
	if md.ResponseID != "" {
		s.responseID = md.ResponseID
	}
	if md.ReasoningID != "" {
		s.reasoningItemID = md.ReasoningID
		for _, block := range s.blocks {
			if block != nil && block.kind == "thinking" {
				block.itemID = md.ReasoningID
			}
		}
		for i := range s.completedOutput {
			if s.completedOutput[i].Type == "reasoning" {
				s.completedOutput[i].ID = md.ReasoningID
			}
		}
	}
	if md.EncryptedContent != "" {
		for _, block := range s.blocks {
			if block != nil && block.kind == "thinking" {
				block.encryptedContent = md.EncryptedContent
			}
		}
		for i := range s.completedOutput {
			if s.completedOutput[i].Type == "reasoning" {
				s.completedOutput[i].EncryptedContent = md.EncryptedContent
			}
		}
	}
}

func (s *responsesStreamSerializer) handleContentBlockStart(event bamboo.StreamEvent) ([]byte, error) {
	if event.ContentBlock == nil {
		return nil, nil
	}

	created, err := s.ensureCreated()
	if err != nil {
		return nil, err
	}

	switch event.ContentBlock.BlockType() {
	case bamboo.ContentBlockText:
		messageAlreadyAdded := s.messageAdded
		block := s.ensureMessageBlock(event.Index)
		if messageAlreadyAdded || block.outputIndex != s.messageIndex {
			return nil, nil
		}
		item := outputItem{
			Type:   "message",
			ID:     s.messageItemID,
			Role:   "assistant",
			Status: "in_progress",
		}
		itemAddedBytes, err := s.marshalSSE("response.output_item.added", outputItemAdded{OutputIndex: s.messageIndex, Item: item})
		if err != nil {
			return nil, err
		}
		partAdded := contentPartAddedEvent{
			OutputIndex:  s.messageIndex,
			ContentIndex: 0,
			ItemID:       s.messageItemID,
			Part:         map[string]any{"type": "output_text", "text": ""},
		}
		partAddedBytes, err := s.marshalSSE("response.content_part.added", partAdded)
		if err != nil {
			return nil, err
		}
		return append(created, append(itemAddedBytes, partAddedBytes...)...), nil

	case bamboo.ContentBlockThinking:
		block := s.ensureReasoningBlock(event.Index)
		item := outputItem{
			Type:    "reasoning",
			ID:      block.itemID,
			Status:  "in_progress",
			Summary: []outputReasoningSummary{},
		}
		itemAddedBytes, err := s.marshalSSE("response.output_item.added", outputItemAdded{OutputIndex: block.outputIndex, Item: item})
		if err != nil {
			return nil, err
		}
		partAdded := reasoningSummaryPartEvent{
			OutputIndex:  block.outputIndex,
			SummaryIndex: 0,
			ItemID:       block.itemID,
			Part:         map[string]any{"type": "summary_text", "text": ""},
		}
		partAddedBytes, err := s.marshalSSE("response.reasoning_summary_part.added", partAdded)
		if err != nil {
			return nil, err
		}
		return append(created, append(itemAddedBytes, partAddedBytes...)...), nil

	case bamboo.ContentBlockToolUse:
		toolUse, ok := event.ContentBlock.(*bamboo.ToolUseBlock)
		if !ok {
			return nil, nil
		}
		block := s.ensureToolBlock(event.Index, toolUse.ID, toolUse.Name)
		item := outputItem{
			Type:      "function_call",
			ID:        block.itemID,
			CallID:    block.callID,
			Name:      block.name,
			Arguments: "",
			Status:    "in_progress",
		}
		itemBytes, err := s.marshalSSE("response.output_item.added", outputItemAdded{OutputIndex: block.outputIndex, Item: item})
		if err != nil {
			return nil, err
		}
		return append(created, itemBytes...), nil
	default:
		return created, nil
	}
}

func (s *responsesStreamSerializer) handleContentBlockDelta(event bamboo.StreamEvent) ([]byte, error) {
	delta, ok := event.Delta.(*bamboo.StreamDelta)
	if !ok {
		return nil, nil
	}

	switch delta.Type {
	case bamboo.DeltaTextDelta:
		block := s.ensureMessageBlock(event.Index)
		s.messageText.WriteString(delta.Text)
		ev := textDeltaEvent{
			OutputIndex:  block.outputIndex,
			ContentIndex: 0,
			ItemID:       block.itemID,
			Delta:        delta.Text,
			Logprobs:     []any{},
		}
		return s.marshalSSE("response.output_text.delta", ev)

	case bamboo.DeltaThinkingDelta:
		created, err := s.ensureCreated()
		if err != nil {
			return nil, err
		}
		block := s.ensureReasoningBlock(event.Index)
		s.reasoningText.WriteString(delta.Thinking)
		raw := reasoningDeltaEvent{
			OutputIndex:  block.outputIndex,
			ContentIndex: 0,
			ItemID:       block.itemID,
			Delta:        delta.Thinking,
		}
		// 双轨并行：raw 给听 reasoning_text 的客户端；
		// summary 给 Codex / OpenAI SDK（官方展示轨，必须带 summary_index）。
		rawBytes, err := s.marshalSSE("response.reasoning_text.delta", raw)
		if err != nil {
			return nil, err
		}
		summary := reasoningSummaryDeltaEvent{
			OutputIndex:  block.outputIndex,
			SummaryIndex: 0,
			ItemID:       block.itemID,
			Delta:        delta.Thinking,
		}
		summaryBytes, err := s.marshalSSE("response.reasoning_summary_text.delta", summary)
		if err != nil {
			return nil, err
		}
		out := append(created, rawBytes...)
		return append(out, summaryBytes...), nil

	case bamboo.DeltaInputJSON:
		block := s.ensureToolBlock(event.Index, s.currentCallID, s.currentCallName)
		block.args.WriteString(delta.PartialJSON)
		s.currentCallArgs.WriteString(delta.PartialJSON)
		ev := functionCallArgsDelta{
			OutputIndex: block.outputIndex,
			ItemID:      block.itemID,
			CallID:      block.callID,
			Delta:       delta.PartialJSON,
		}
		return s.marshalSSE("response.function_call_arguments.delta", ev)

	case bamboo.DeltaSignature:
		block := s.ensureReasoningBlock(event.Index)
		block.encryptedContent = delta.Signature
		return nil, nil
	default:
		return nil, nil
	}
}

func (s *responsesStreamSerializer) handleContentBlockStop(event bamboo.StreamEvent) ([]byte, error) {
	block := s.blocks[event.Index]
	if block == nil {
		if s.messageAdded {
			block = s.ensureMessageBlock(event.Index)
		} else if s.reasoningItemID != "" {
			block = s.ensureReasoningBlock(event.Index)
		} else {
			return nil, nil
		}
	}
	delete(s.blocks, event.Index)

	switch block.kind {
	case "text":
		text := s.messageText.String()
		done := textDoneEvent{
			OutputIndex:  block.outputIndex,
			ContentIndex: 0,
			ItemID:       block.itemID,
			Text:         text,
			Logprobs:     []any{},
		}
		textDoneBytes, err := s.marshalSSE("response.output_text.done", done)
		if err != nil {
			return nil, err
		}
		partDone := contentPartDoneEvent{
			OutputIndex:  block.outputIndex,
			ContentIndex: 0,
			ItemID:       block.itemID,
			Part:         map[string]any{"type": "output_text", "text": text},
		}
		partDoneBytes, err := s.marshalSSE("response.content_part.done", partDone)
		if err != nil {
			return nil, err
		}
		item := outputItem{
			Type:   "message",
			ID:     block.itemID,
			Role:   "assistant",
			Status: "completed",
			Content: []outputContent{
				{Type: "output_text", Text: text},
			},
		}
		s.completedOutput = append(s.completedOutput, item)
		itemDoneBytes, err := s.marshalSSE("response.output_item.done", outputItemAdded{OutputIndex: block.outputIndex, Item: item})
		if err != nil {
			return nil, err
		}
		out := append(textDoneBytes, partDoneBytes...)
		return append(out, itemDoneBytes...), nil

	case "thinking":
		text := s.reasoningText.String()
		rawDone := reasoningDoneEvent{
			OutputIndex:  block.outputIndex,
			ContentIndex: 0,
			ItemID:       block.itemID,
			Text:         text,
		}
		rawDoneBytes, err := s.marshalSSE("response.reasoning_text.done", rawDone)
		if err != nil {
			return nil, err
		}
		summaryDone := reasoningSummaryDoneEvent{
			OutputIndex:  block.outputIndex,
			SummaryIndex: 0,
			ItemID:       block.itemID,
			Text:         text,
		}
		summaryDoneBytes, err := s.marshalSSE("response.reasoning_summary_text.done", summaryDone)
		if err != nil {
			return nil, err
		}
		partDone := reasoningSummaryPartEvent{
			OutputIndex:  block.outputIndex,
			SummaryIndex: 0,
			ItemID:       block.itemID,
			Part:         map[string]any{"type": "summary_text", "text": text},
		}
		partDoneBytes, err := s.marshalSSE("response.reasoning_summary_part.done", partDone)
		if err != nil {
			return nil, err
		}
		item := outputItem{
			Type:             "reasoning",
			ID:               block.itemID,
			Status:           "completed",
			Content:          buildReasoningContent(text),
			Summary:          buildReasoningSummary(text),
			EncryptedContent: block.encryptedContent,
		}
		s.completedOutput = append(s.completedOutput, item)
		itemDoneBytes, err := s.marshalSSE("response.output_item.done", outputItemAdded{OutputIndex: block.outputIndex, Item: item})
		if err != nil {
			return nil, err
		}
		out := append(rawDoneBytes, summaryDoneBytes...)
		out = append(out, partDoneBytes...)
		return append(out, itemDoneBytes...), nil

	case "tool_use":
		args := block.args.String()
		done := functionCallArgsDone{
			OutputIndex: block.outputIndex,
			ItemID:      block.itemID,
			CallID:      block.callID,
			Name:        block.name,
			Arguments:   args,
		}
		contentDoneBytes, err := s.marshalSSE("response.function_call_arguments.done", done)
		if err != nil {
			return nil, err
		}
		item := outputItem{
			Type:      "function_call",
			ID:        block.itemID,
			CallID:    block.callID,
			Name:      block.name,
			Arguments: args,
			Status:    "completed",
		}
		s.completedOutput = append(s.completedOutput, item)
		itemDoneBytes, err := s.marshalSSE("response.output_item.done", outputItemAdded{OutputIndex: block.outputIndex, Item: item})
		if err != nil {
			return nil, err
		}
		return append(contentDoneBytes, itemDoneBytes...), nil
	default:
		return nil, nil
	}
}

func (s *responsesStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	msgDelta, ok := event.Delta.(*bamboo.MessageDelta)
	if ok {
		s.applyReasoningMeta(msgDelta)
	}
	if (!ok || msgDelta.StopReason == "") && event.Usage == nil {
		return s.ensureCreated()
	}
	if s.completedSent {
		return nil, nil
	}
	s.completedSent = true

	created, err := s.ensureCreated()
	if err != nil {
		return nil, err
	}

	status := "completed"
	if ok && msgDelta.StopReason == bamboo.FinishReasonMaxTokens {
		status = "incomplete"
	}

	resp := responseObj{
		ID:        s.responseID,
		Object:    "response",
		CreatedAt: s.createdAt,
		Status:    status,
		Model:     s.model,
		Output:    append([]outputItem{}, s.completedOutput...),
		Usage:     buildResponsesUsage(event.Usage),
	}
	completed, err := s.marshalSSEWithResponse("response.completed", resp)
	if err != nil {
		return nil, err
	}
	return append(created, completed...), nil
}

func (s *responsesStreamSerializer) handleError(event bamboo.StreamEvent) ([]byte, error) {
	errMsg := "unknown error"
	errCode := "server_error"
	if event.Error != nil {
		errMsg = event.Error.Message
		errCode = mapStatusCodeToResponsesCode(event.Error.StatusCode)
	}
	resp := responseObj{
		ID:        s.responseID,
		Object:    "response",
		CreatedAt: s.createdAt,
		Status:    "failed",
		Output:    []outputItem{},
		Error: &responseError{
			Message: errMsg,
			Type:    errCode,
			Code:    errCode,
		},
	}
	return s.marshalSSEWithResponse("response.failed", resp)
}

func (s *responsesStreamSerializer) ensureMessageBlock(index int) *responsesStreamBlock {
	if block := s.blocks[index]; block != nil {
		return block
	}
	if !s.messageAdded {
		s.messageAdded = true
		s.messageIndex = s.outputIndexCounter
		s.outputIndexCounter++
		s.messageItemID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	block := &responsesStreamBlock{
		kind:        "text",
		outputIndex: s.messageIndex,
		itemID:      s.messageItemID,
	}
	s.blocks[index] = block
	return block
}

func (s *responsesStreamSerializer) ensureReasoningBlock(index int) *responsesStreamBlock {
	if block := s.blocks[index]; block != nil {
		return block
	}
	if s.reasoningItemID == "" {
		s.reasoningIndex = s.outputIndexCounter
		s.outputIndexCounter++
		s.reasoningItemID = fmt.Sprintf("rs_%d", time.Now().UnixNano())
	}
	block := &responsesStreamBlock{
		kind:        "thinking",
		outputIndex: s.reasoningIndex,
		itemID:      s.reasoningItemID,
	}
	s.blocks[index] = block
	return block
}

func (s *responsesStreamSerializer) ensureToolBlock(index int, callID, name string) *responsesStreamBlock {
	if block := s.blocks[index]; block != nil {
		return block
	}
	if callID == "" {
		callID = fmt.Sprintf("call_%d", time.Now().UnixNano())
	}
	itemID := fmt.Sprintf("fc_%d", time.Now().UnixNano())
	block := &responsesStreamBlock{
		kind:        "tool_use",
		outputIndex: s.outputIndexCounter,
		itemID:      itemID,
		callID:      callID,
		name:        name,
	}
	s.outputIndexCounter++
	s.blocks[index] = block

	s.currentCallID = callID
	s.currentCallName = name
	s.functionCallItem = itemID
	s.functionCallIdx = block.outputIndex
	s.currentCallArgs.Reset()
	return block
}

func buildResponsesUsage(usage *bamboo.Usage) *responsesUsage {
	if usage == nil {
		return &responsesUsage{
			InputTokensDetails:  &responsesInputTokensDet{},
			OutputTokensDetails: &responsesOutputTokensDet{},
		}
	}
	return &responsesUsage{
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		TotalTokens:         usage.InputTokens + usage.OutputTokens,
		InputTokensDetails:  &responsesInputTokensDet{CachedTokens: usage.CacheReadInputTokens},
		OutputTokensDetails: &responsesOutputTokensDet{ReasoningTokens: usage.ReasoningTokens},
	}
}

func (s *responsesStreamSerializer) marshalSSE(eventType string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal %s event: %v", eventType, err), 0)
	}

	merged := make(map[string]any, 8)
	if err := json.Unmarshal(raw, &merged); err != nil {
		return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, raw)), nil
	}
	s.injectCommonFields(eventType, merged)

	data, err := json.Marshal(merged)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to re-marshal %s event with type: %v", eventType, err), 0)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}

func (s *responsesStreamSerializer) marshalSSEWithResponse(eventType string, resp responseObj) ([]byte, error) {
	respRaw, err := json.Marshal(resp)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal response for %s event: %v", eventType, err), 0)
	}

	var respMap map[string]any
	if err := json.Unmarshal(respRaw, &respMap); err != nil {
		return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, respRaw)), nil
	}

	payload := map[string]any{
		"response": respMap,
	}
	s.injectCommonFields(eventType, payload)

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal %s event payload: %v", eventType, err), 0)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)), nil
}

func (s *responsesStreamSerializer) injectCommonFields(eventType string, payload map[string]any) {
	s.sequenceNumber++
	payload["type"] = eventType
	payload["sequence_number"] = s.sequenceNumber
	payload["response_id"] = s.responseID
}

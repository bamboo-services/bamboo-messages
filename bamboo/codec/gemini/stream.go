package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

type geminiStreamChunk struct {
	Candidates    []geminiStreamCandidate `json:"candidates,omitempty"`
	UsageMetadata *geminiUsageMeta        `json:"usageMetadata,omitempty"`
}

type geminiStreamCandidate struct {
	Content      *geminiContentOut `json:"content,omitempty"`
	FinishReason string            `json:"finishReason,omitempty"`
	Index        int               `json:"index"`
}

type geminiStreamError struct {
	Error geminiStreamErrorBody `json:"error"`
}

type geminiStreamErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type geminiPendingCall struct {
	id, name, signature string
	arguments           strings.Builder
	args                json.RawMessage
	completed, emitted  bool
}

type geminiThinkingState struct {
	provider            string
	nonempty, completed bool
}

type geminiPendingSignature struct {
	value    string
	thinking *geminiThinkingState
}

// geminiStreamSerializer 按块索引保存工具参数，按首次声明顺序输出完整调用。
type geminiStreamSerializer struct {
	calls                                      map[int]*geminiPendingCall
	order                                      []int
	nextCall                                   int
	thinking                                   map[int]*geminiThinkingState
	signatures                                 []geminiPendingSignature
	failed, finished                           bool
	inputTokens, outputTokens, cacheReadTokens int64
}

func newStreamSerializer(_ string) *geminiStreamSerializer {
	return &geminiStreamSerializer{
		calls:    make(map[int]*geminiPendingCall),
		thinking: make(map[int]*geminiThinkingState),
	}
}

func (s *geminiStreamSerializer) Serialize(event bamboo.StreamEvent) (data []byte, err error) {
	if s.failed || s.finished {
		return nil, nil
	}
	// relay 在出错时丢弃本次字节并继续消费；失败后不能再补发成功序列。
	defer func() {
		if err != nil {
			s.fail()
			data = nil
		}
	}()
	switch event.Type {
	case bamboo.EventMessageStart:
		if event.Usage != nil {
			s.inputTokens = event.Usage.InputTokens
			s.cacheReadTokens = event.Usage.CacheReadInputTokens
		}
	case bamboo.EventContentBlockStart:
		return s.handleContentBlockStart(event)
	case bamboo.EventContentBlockDelta:
		return s.handleContentBlockDelta(event)
	case bamboo.EventContentBlockStop:
		return s.handleContentBlockStop(event)
	case bamboo.EventMessageDelta:
		return s.handleMessageDelta(event)
	case bamboo.EventMessageStop:
		return s.Flush()
	case bamboo.EventPing:
		return []byte(": keep-alive\n\n"), nil
	case bamboo.EventError:
		s.fail()
		return s.handleError(event)
	}
	return nil, nil
}

func (s *geminiStreamSerializer) fail() {
	s.failed = true
	s.calls = nil
	s.order = nil
	s.thinking = nil
	s.signatures = nil
}

func (s *geminiStreamSerializer) Flush() ([]byte, error) {
	if s.failed || s.finished {
		return nil, nil
	}
	parts, err := s.terminalParts()
	if err != nil {
		s.fail()
		return nil, err
	}
	s.finished = true
	return s.marshalParts(parts)
}

func (s *geminiStreamSerializer) handleContentBlockStart(event bamboo.StreamEvent) ([]byte, error) {
	switch block := event.ContentBlock.(type) {
	case *bamboo.ToolUseBlock:
		if _, exists := s.calls[event.Index]; exists {
			return nil, nil
		}
		s.calls[event.Index] = &geminiPendingCall{id: block.ID, name: block.Name, signature: s.takeSignature()}
		s.order = append(s.order, event.Index)
	case *bamboo.ThinkingBlock:
		state := &geminiThinkingState{provider: block.SignatureProvider, nonempty: block.Thinking != ""}
		s.thinking[event.Index] = state
		if state.provider != bamboo.SignatureProviderGemini {
			return nil, nil
		}
		if state.nonempty {
			return s.marshalParts([]geminiPartOut{{Text: block.Thinking, Thought: true, ThoughtSignature: block.Signature}})
		}
		if block.Signature != "" {
			s.signatures = append(s.signatures, geminiPendingSignature{value: block.Signature, thinking: state})
		}
	}
	return nil, nil
}

func (s *geminiStreamSerializer) handleContentBlockDelta(event bamboo.StreamEvent) ([]byte, error) {
	delta, ok := event.Delta.(*bamboo.StreamDelta)
	if !ok {
		return nil, nil
	}
	switch delta.Type {
	case bamboo.DeltaTextDelta:
		text := delta.Text
		return s.marshalParts([]geminiPartOut{{explicitText: &text, ThoughtSignature: s.takeSignature()}})
	case bamboo.DeltaThinkingDelta:
		state := s.thinking[event.Index]
		if state == nil || state.provider != bamboo.SignatureProviderGemini || delta.Thinking == "" {
			return nil, nil
		}
		state.nonempty = true
		return s.marshalParts([]geminiPartOut{{Text: delta.Thinking, Thought: true}})
	case bamboo.DeltaInputJSON:
		call := s.calls[event.Index]
		if call == nil || call.completed {
			return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("gemini arguments for unknown or closed index %d", event.Index), 0)
		}
		call.arguments.WriteString(delta.PartialJSON)
	case bamboo.DeltaSignature:
		state := s.thinking[event.Index]
		provenance := delta.SignatureProvider
		if provenance == "" && state != nil {
			provenance = state.provider
		}
		if provenance != bamboo.SignatureProviderGemini || delta.Signature == "" {
			return nil, nil
		}
		if state == nil {
			state = &geminiThinkingState{provider: provenance}
			s.thinking[event.Index] = state
		}
		s.signatures = append(s.signatures, geminiPendingSignature{value: delta.Signature, thinking: state})
	}
	return nil, nil
}

func (s *geminiStreamSerializer) takeSignature() string {
	for i, signature := range s.signatures {
		if signature.thinking.completed && !signature.thinking.nonempty {
			s.signatures = append(s.signatures[:i], s.signatures[i+1:]...)
			return signature.value
		}
	}
	return ""
}

func (s *geminiStreamSerializer) handleContentBlockStop(event bamboo.StreamEvent) ([]byte, error) {
	if state := s.thinking[event.Index]; state != nil {
		state.completed = true
	}
	call := s.calls[event.Index]
	if call == nil || call.completed {
		return nil, nil
	}
	raw := call.arguments.String()
	if raw == "" {
		raw = "{}"
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil || object == nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("gemini arguments at index %d must be a JSON object", event.Index), 0)
	}
	call.args = json.RawMessage(raw)
	call.completed = true
	var parts []geminiPartOut
	for s.nextCall < len(s.order) {
		next := s.calls[s.order[s.nextCall]]
		if !next.completed {
			break
		}
		parts = append(parts, geminiPartOut{FunctionCall: &geminiFuncCallOut{ID: next.id, Name: next.name, Args: next.args}, ThoughtSignature: next.signature})
		next.emitted = true
		next.arguments.Reset()
		s.nextCall++
	}
	return s.marshalParts(parts)
}

func (s *geminiStreamSerializer) terminalParts() ([]geminiPartOut, error) {
	for _, index := range s.order {
		if !s.calls[index].emitted {
			return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("incomplete gemini function call at index %d", index), 0)
		}
	}
	var parts []geminiPartOut
	for _, signature := range s.signatures {
		empty := ""
		parts = append(parts, geminiPartOut{explicitText: &empty, ThoughtSignature: signature.value})
	}
	s.signatures = nil
	s.thinking = nil
	return parts, nil
}

func (s *geminiStreamSerializer) handleMessageDelta(event bamboo.StreamEvent) ([]byte, error) {
	delta, ok := event.Delta.(*bamboo.MessageDelta)
	if !ok || (delta.StopReason == "" && event.Usage == nil) {
		return nil, nil
	}
	var parts []geminiPartOut
	finishReason := ""
	if delta.StopReason != "" {
		var err error
		parts, err = s.terminalParts()
		if err != nil {
			return nil, err
		}
		finishReason = mapFinishReasonToGemini(delta.StopReason)
		s.finished = true
	}
	if event.Usage != nil {
		s.outputTokens = event.Usage.OutputTokens
		if event.Usage.InputTokens > 0 {
			s.inputTokens = event.Usage.InputTokens
		}
		if event.Usage.CacheReadInputTokens > 0 {
			s.cacheReadTokens = event.Usage.CacheReadInputTokens
		}
	}
	candidate := geminiStreamCandidate{Index: 0, FinishReason: finishReason}
	if len(parts) > 0 {
		candidate.Content = &geminiContentOut{Role: "model", Parts: parts}
	}
	return s.marshalChunk(geminiStreamChunk{
		Candidates:    []geminiStreamCandidate{candidate},
		UsageMetadata: &geminiUsageMeta{PromptTokenCount: s.inputTokens, CandidatesTokenCount: s.outputTokens, TotalTokenCount: s.inputTokens + s.outputTokens, CachedContentTokenCount: s.cacheReadTokens},
	})
}

func (s *geminiStreamSerializer) handleError(event bamboo.StreamEvent) ([]byte, error) {
	code, status, message := 500, "INTERNAL", "unknown error"
	if event.Error != nil {
		message = event.Error.Message
		code, status = mapStatusCodeToGemini(event.Error.StatusCode)
	}
	data, err := json.Marshal(geminiStreamError{Error: geminiStreamErrorBody{Code: code, Message: message, Status: status}})
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal gemini error: %v", err), 0)
	}
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

func (s *geminiStreamSerializer) marshalParts(parts []geminiPartOut) ([]byte, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	return s.marshalChunk(geminiStreamChunk{Candidates: []geminiStreamCandidate{{Index: 0, Content: &geminiContentOut{Role: "model", Parts: parts}}}})
}

func (s *geminiStreamSerializer) marshalChunk(chunk geminiStreamChunk) ([]byte, error) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal gemini stream chunk: %v", err), 0)
	}
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

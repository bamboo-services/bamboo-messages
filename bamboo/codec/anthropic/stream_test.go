package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// helper: 解析 SSE event + data 行为 map
func parseSSEEvent(t *testing.T, raw []byte) (eventType string, payload map[string]any) {
	t.Helper()
	str := string(raw)
	lines := strings.Split(strings.TrimRight(str, "\n"), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(dataStr), &payload); err != nil {
				t.Fatalf("failed to unmarshal SSE data: %v\nraw: %s", err, dataStr)
			}
		}
	}
	if eventType == "" {
		t.Fatalf("missing event: line in SSE: %q", str)
	}
	if payload == nil {
		t.Fatalf("missing data: line in SSE: %q", str)
	}
	return eventType, payload
}

// sseFrame 表示一个解析后的 SSE 帧。
type sseFrame struct {
	eventType string
	payload   map[string]any
}

// parseSSEFrames 解析包含多个 SSE 帧的缓冲区。
//
// message_start 帧会被延迟到下一个事件到达时一起输出，
// 因此一次 Serialize 可能返回多个帧，需要逐帧校验。
func parseSSEFrames(t *testing.T, raw []byte) []sseFrame {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var frames []sseFrame
	for _, block := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		var frame sseFrame
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event: ") {
				frame.eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				dataStr := strings.TrimPrefix(line, "data: ")
				if err := json.Unmarshal([]byte(dataStr), &frame.payload); err != nil {
					t.Fatalf("failed to unmarshal SSE data: %v\nraw: %s", err, dataStr)
				}
			}
		}
		if frame.eventType == "" {
			t.Fatalf("missing event: line in SSE: %q", block)
		}
		if frame.payload == nil {
			t.Fatalf("missing data: line in SSE: %q", block)
		}
		frames = append(frames, frame)
	}
	return frames
}

func TestStreamSerializer_TextStream(t *testing.T) {
	s := newStreamSerializer("")

	// 1. message_start
	//
	// message_start 帧被延迟输出：Anthropic 要求该帧携带输入与缓存统计，
	// 而这些统计通常在随后的 usage 增量中才到达，因此这里不立即产出帧。
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
		Usage:   &bamboo.Usage{InputTokens: 10, OutputTokens: 0},
	})
	if err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("message_start should defer output, got %q", data)
	}

	// 2. content_block_start (text)：与延迟的 message_start 帧一起输出
	frame, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	frames := parseSSEFrames(t, frame)
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames (message_start + content_block_start), got %d", len(frames))
	}
	if frames[0].eventType != "message_start" {
		t.Errorf("first frame eventType = %q, want %q", frames[0].eventType, "message_start")
	}
	payload := frames[0].payload
	if payload["type"] != "message_start" {
		t.Errorf("payload type = %v", payload["type"])
	}
	msg, ok := payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message field should be object")
	}
	if msg["role"] != "assistant" {
		t.Errorf("role = %v", msg["role"])
	}
	if msg["type"] != "message" {
		t.Errorf("message.type = %v", msg["type"])
	}
	usage, ok := msg["usage"].(map[string]any)
	if !ok {
		t.Fatalf("message.usage should be object")
	}
	if in, ok := usage["input_tokens"].(float64); !ok || in != 10 {
		t.Errorf("usage.input_tokens = %v, want 10", usage["input_tokens"])
	}
	eventType := frames[1].eventType
	payload = frames[1].payload
	if eventType != "content_block_start" {
		t.Errorf("eventType = %q", eventType)
	}
	if idx, ok := payload["index"].(float64); !ok || idx != 0 {
		t.Errorf("index = %v", payload["index"])
	}
	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block should be object")
	}
	if cb["type"] != "text" {
		t.Errorf("content_block.type = %v", cb["type"])
	}

	// 3. content_block_delta (text_delta)
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaTextDelta, Text: "Hello"},
	})
	if err != nil {
		t.Fatalf("Serialize(text_delta) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "content_block_delta" {
		t.Errorf("eventType = %q", eventType)
	}
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["type"] != "text_delta" {
		t.Errorf("delta.type = %v", delta["type"])
	}
	if delta["text"] != "Hello" {
		t.Errorf("delta.text = %v", delta["text"])
	}

	// 4. content_block_stop
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockStop,
		Index: 0,
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_stop) error = %v", err)
	}
	eventType, _ = parseSSEEvent(t, data)
	if eventType != "content_block_stop" {
		t.Errorf("eventType = %q", eventType)
	}

	// 5. message_delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
		Usage: &bamboo.Usage{InputTokens: 10, OutputTokens: 5},
	})
	if err != nil {
		t.Fatalf("Serialize(message_delta) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "message_delta" {
		t.Errorf("eventType = %q", eventType)
	}
	deltaObj, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if deltaObj["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v", deltaObj["stop_reason"])
	}
	if deltaObj["stop_sequence"] != nil {
		t.Errorf("stop_sequence should be null, got %v", deltaObj["stop_sequence"])
	}

	// 6. message_stop
	data, err = s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageStop,
	})
	if err != nil {
		t.Fatalf("Serialize(message_stop) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "message_stop" {
		t.Errorf("eventType = %q", eventType)
	}
	if payload["type"] != "message_stop" {
		t.Errorf("payload type = %v", payload["type"])
	}
}

func TestStreamSerializer_FlushReturnsNil(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if data != nil {
		t.Errorf("Flush() should return nil for Anthropic (no [DONE]), got %q", string(data))
	}
}

func TestStreamSerializer_ErrorEvent(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventError,
		Error: &bamboo.BambooError{
			Category:   "上游",
			Message:    "rate exceeded",
			StatusCode: 429,
		},
	})
	if err != nil {
		t.Fatalf("Serialize(error) error = %v", err)
	}
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "error" {
		t.Errorf("eventType = %q, want %q", eventType, "error")
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field should be object")
	}
	if errObj["type"] != "rate_limit_error" {
		t.Errorf("error.type = %v", errObj["type"])
	}
	if errObj["message"] != "rate exceeded" {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestStreamSerializer_ToolUseStream(t *testing.T) {
	s := newStreamSerializer("")

	// message_start
	_, _ = s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// content_block_start (tool_use)
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewToolUseBlock("call_abc", "get_weather", nil),
	})
	if err != nil {
		t.Fatalf("Serialize(tool_use start) error = %v", err)
	}
	_, payload := parseSSEEvent(t, data)
	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block should be object")
	}
	if cb["type"] != "tool_use" {
		t.Errorf("type = %v", cb["type"])
	}
	if cb["id"] != "call_abc" {
		t.Errorf("id = %v", cb["id"])
	}
	if cb["name"] != "get_weather" {
		t.Errorf("name = %v", cb["name"])
	}

	// input_json_delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaInputJSON, PartialJSON: `{"city":"SF"}`},
	})
	if err != nil {
		t.Fatalf("Serialize(input_json_delta) error = %v", err)
	}
	_, payload = parseSSEEvent(t, data)
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta should be object")
	}
	if delta["type"] != "input_json_delta" {
		t.Errorf("delta.type = %v", delta["type"])
	}
	if delta["partial_json"] != `{"city":"SF"}` {
		t.Errorf("partial_json = %v", delta["partial_json"])
	}
}

func TestStreamSerializer_ThinkingStream(t *testing.T) {
	s := newStreamSerializer("")

	_, _ = s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})

	// thinking content_block_start
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewThinkingBlock("", ""),
	})
	if err != nil {
		t.Fatalf("Serialize(thinking start) error = %v", err)
	}
	_, payload := parseSSEEvent(t, data)
	cb, _ := payload["content_block"].(map[string]any)
	if cb["type"] != "thinking" {
		t.Errorf("type = %v, want %q", cb["type"], "thinking")
	}

	// thinking_delta
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventContentBlockDelta,
		Index: 0,
		Delta: &bamboo.StreamDelta{Type: bamboo.DeltaThinkingDelta, Thinking: "hmm..."},
	})
	if err != nil {
		t.Fatalf("Serialize(thinking_delta) error = %v", err)
	}
	_, payload = parseSSEEvent(t, data)
	delta, _ := payload["delta"].(map[string]any)
	if delta["thinking"] != "hmm..." {
		t.Errorf("thinking = %v", delta["thinking"])
	}
}

func TestStreamSerializer_PingEvent(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventPing,
	})
	if err != nil {
		t.Fatalf("Serialize(ping) error = %v", err)
	}
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "ping" {
		t.Errorf("eventType = %q, want %q", eventType, "ping")
	}
	if payload["type"] != "ping" {
		t.Errorf("payload type = %v", payload["type"])
	}
}

// TestStreamSerializer_MessageStartCarriesCacheStats 验证 message_start 帧携带缓存统计。
//
// 回归场景：message_start 此前立即输出且 usage 为空，下游按 Anthropic 惯例从
// message_start 读取输入与缓存统计时只能拿到 0，表现为「缓存无法记录」。
// 现在 message_start 会等待 usage 就绪（ping 事件）后再输出，并回填统计。
func TestStreamSerializer_MessageStartCarriesCacheStats(t *testing.T) {
	s := newStreamSerializer("claude-sonnet-4")

	// message_start 只登记状态，不立即产出帧
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})
	if err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("message_start should defer output, got %q", data)
	}

	// ping 事件携带 usage：message_start 帧随之输出
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:  bamboo.EventPing,
		Usage: &bamboo.Usage{InputTokens: 175, CacheCreationInputTokens: 50, CacheReadInputTokens: 25},
	})
	if err != nil {
		t.Fatalf("Serialize(ping) error = %v", err)
	}
	frames := parseSSEFrames(t, data)
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames (message_start + ping), got %d", len(frames))
	}
	if frames[0].eventType != "message_start" {
		t.Fatalf("first frame = %q, want %q", frames[0].eventType, "message_start")
	}
	if frames[1].eventType != "ping" {
		t.Fatalf("second frame = %q, want %q", frames[1].eventType, "ping")
	}

	msg, ok := frames[0].payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message field should be object")
	}
	usage, ok := msg["usage"].(map[string]any)
	if !ok {
		t.Fatalf("message.usage should be object")
	}
	// Anthropic 语义：input_tokens 不含缓存部分
	if in, ok := usage["input_tokens"].(float64); !ok || in != 100 {
		t.Errorf("input_tokens = %v, want 100", usage["input_tokens"])
	}
	if cc, ok := usage["cache_creation_input_tokens"].(float64); !ok || cc != 50 {
		t.Errorf("cache_creation_input_tokens = %v, want 50", usage["cache_creation_input_tokens"])
	}
	if cr, ok := usage["cache_read_input_tokens"].(float64); !ok || cr != 25 {
		t.Errorf("cache_read_input_tokens = %v, want 25", usage["cache_read_input_tokens"])
	}
}

func TestStreamSerializer_MessageDeltaUsage(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonMaxTokens,
		},
		Usage: &bamboo.Usage{InputTokens: 10, OutputTokens: 100},
	})
	if err != nil {
		t.Fatalf("Serialize error = %v", err)
	}
	_, payload := parseSSEEvent(t, data)
	delta, _ := payload["delta"].(map[string]any)
	if delta["stop_reason"] != "max_tokens" {
		t.Errorf("stop_reason = %v", delta["stop_reason"])
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage should be object")
	}
	if in, ok := usage["input_tokens"].(float64); !ok || in != 10 {
		t.Errorf("input_tokens = %v", usage["input_tokens"])
	}
	if out, ok := usage["output_tokens"].(float64); !ok || out != 100 {
		t.Errorf("output_tokens = %v", usage["output_tokens"])
	}
}

// TestStreamSerializer_MessageDeltaUsageWithCache 验证流式 message_delta 中
// input_tokens 正确减去缓存部分（Anthropic 语义：input_tokens 不含缓存）。
func TestStreamSerializer_MessageDeltaUsageWithCache(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type: bamboo.EventMessageDelta,
		Delta: &bamboo.MessageDelta{
			StopReason: bamboo.FinishReasonEndTurn,
		},
		Usage: &bamboo.Usage{
			InputTokens:              99516,
			OutputTokens:             139,
			CacheReadInputTokens:     98880,
			CacheCreationInputTokens: 0,
		},
	})
	if err != nil {
		t.Fatalf("Serialize error = %v", err)
	}
	_, payload := parseSSEEvent(t, data)
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage should be object")
	}

	// input_tokens 应为 99516 - 0 - 98880 = 636（非缓存部分）
	if in, ok := usage["input_tokens"].(float64); !ok || in != 636 {
		t.Errorf("input_tokens = %v, want 636 (non-cached)", usage["input_tokens"])
	}
	if cr, ok := usage["cache_read_input_tokens"].(float64); !ok || cr != 98880 {
		t.Errorf("cache_read_input_tokens = %v, want 98880", usage["cache_read_input_tokens"])
	}
}

func TestMessageStartModelID(t *testing.T) {
	model := "claude-sonnet-4-20250514"
	s := newStreamSerializer(model)

	data, err := s.Serialize(bamboo.StreamEvent{
		Type:    bamboo.EventMessageStart,
		Message: &bamboo.BambooMessage{Role: bamboo.RoleAssistant},
	})
	if err != nil {
		t.Fatalf("Serialize(message_start) error = %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("message_start should defer output, got %q", data)
	}

	// message_start 帧随下一个事件一起输出
	data, err = s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewTextBlock(""),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	frames := parseSSEFrames(t, data)
	if len(frames) == 0 || frames[0].eventType != "message_start" {
		t.Fatalf("expected message_start frame first, got %+v", frames)
	}
	payload := frames[0].payload
	msg, ok := payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message field should be object")
	}
	if msg["model"] != model {
		t.Errorf("message.model = %q, want %q", msg["model"], model)
	}

	id, ok := msg["id"].(string)
	if !ok {
		t.Fatalf("message.id should be string, got %T", msg["id"])
	}
	if id == "" {
		t.Errorf("message.id should not be empty")
	}
}

func TestSerializeStreamContentBlock_RedactedThinking(t *testing.T) {
	block := bamboo.NewRedactedThinkingBlock("encrypted_data")
	got := serializeStreamContentBlock(block)
	want := map[string]any{
		"type": "redacted_thinking",
		"data": "encrypted_data",
	}
	if got["type"] != want["type"] {
		t.Errorf("type = %v, want %v", got["type"], want["type"])
	}
	if got["data"] != want["data"] {
		t.Errorf("data = %v, want %v", got["data"], want["data"])
	}
}

func TestHandleContentBlockStart_RedactedThinking(t *testing.T) {
	s := newStreamSerializer("")
	data, err := s.Serialize(bamboo.StreamEvent{
		Type:         bamboo.EventContentBlockStart,
		Index:        0,
		ContentBlock: bamboo.NewRedactedThinkingBlock("encrypted_data"),
	})
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}

	eventType, payload := parseSSEEvent(t, data)
	if eventType != "content_block_start" {
		t.Errorf("eventType = %q, want %q", eventType, "content_block_start")
	}

	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block should be object")
	}
	if cb["type"] != "redacted_thinking" {
		t.Errorf("content_block.type = %v, want %q", cb["type"], "redacted_thinking")
	}
	if cb["data"] != "encrypted_data" {
		t.Errorf("content_block.data = %v, want %q", cb["data"], "encrypted_data")
	}
}

// TestSerialize_SignatureOnly_ThinkingBlock_SSE 验证 omitted thinking 模式下，
// StreamConverter + Anthropic codec 能输出完整的 content_block_start/thinking 与
// content_block_delta/signature_delta SSE 帧。
func TestSerialize_SignatureOnly_ThinkingBlock_SSE(t *testing.T) {
	// 复现 omitted thinking 模式：Provider 只发送 signature_delta
	sc := bamboo.NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewSignatureDelta("test_sig"),
	})
	if len(events) != 2 {
		t.Fatalf("期望 2 个 bamboo 事件，实际 %d 个", len(events))
	}

	s := newStreamSerializer("claude-sonnet-4-20250514")

	// 1. content_block_start: type=thinking
	data, err := s.Serialize(events[0])
	if err != nil {
		t.Fatalf("Serialize(content_block_start) error = %v", err)
	}
	eventType, payload := parseSSEEvent(t, data)
	if eventType != "content_block_start" {
		t.Errorf("eventType = %q, want content_block_start", eventType)
	}
	cb, ok := payload["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("content_block 应为对象")
	}
	if cb["type"] != "thinking" {
		t.Errorf("content_block.type = %v, want thinking", cb["type"])
	}

	// 2. content_block_delta: type=signature_delta, signature=test_sig
	data, err = s.Serialize(events[1])
	if err != nil {
		t.Fatalf("Serialize(content_block_delta) error = %v", err)
	}
	eventType, payload = parseSSEEvent(t, data)
	if eventType != "content_block_delta" {
		t.Errorf("eventType = %q, want content_block_delta", eventType)
	}
	delta, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("delta 应为对象")
	}
	if delta["type"] != "signature_delta" {
		t.Errorf("delta.type = %v, want signature_delta", delta["type"])
	}
	if delta["signature"] != "test_sig" {
		t.Errorf("delta.signature = %v, want test_sig", delta["signature"])
	}
}

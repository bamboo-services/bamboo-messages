package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// httptest 辅助工具
// ==============================

// newMockProvider 创建指向 mock server 的 Provider 实例。
func newMockProvider(t *testing.T, server *httptest.Server) *Provider {
	t.Helper()
	p := NewProviderWithOptions(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)
	return p
}

// sseFixture 构建符合 Anthropic SSE 格式的事件序列。
//
// Anthropic SSE 每个事件由 event: 行和 data: 行组成，事件间用空行分隔。
func sseFixture(events ...[2]string) string {
	var sb strings.Builder
	for _, ev := range events {
		sb.WriteString("event: ")
		sb.WriteString(ev[0])
		sb.WriteString("\n")
		sb.WriteString("data: ")
		sb.WriteString(ev[1])
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// drainEvents 从 channel 收集所有事件直到关闭，带超时保护。
func drainEvents(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-time.After(5 * time.Second):
			return events
		}
	}
}

// findEventByType 在事件列表中查找指定类型的事件。
func findEventByType(events []provider.StreamEvent, t provider.StreamType) (provider.StreamEvent, bool) {
	for _, ev := range events {
		if ev.Type == t {
			return ev, true
		}
	}
	return provider.StreamEvent{}, false
}

// findDeltaByType 在事件列表中查找指定 delta 类型的 Delta 事件。
func findDeltaByType(events []provider.StreamEvent, dt provider.StreamDeltaType) (provider.StreamEvent, bool) {
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == dt {
			return ev, true
		}
	}
	return provider.StreamEvent{}, false
}

// ==============================
// Chat 流式测试
// ==============================

// TestChat_MessageStart 验证 message_start 事件触发 Start 事件。
func TestChat_MessageStart(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{
		Model:    "claude-sonnet-4-20250514",
		MaxTokens: 100,
	}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 Start 事件
	if _, ok := findEventByType(events, provider.StreamTypeStart); !ok {
		t.Error("expected StreamTypeStart event, not found")
	}

	// 应包含 Done 事件
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event, not found")
	}

	// 应包含 Stop 事件
	stopEv, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop event, not found")
	}
	if stopEv.FinishReason != provider.FinishReasonStop {
		t.Errorf("FinishReason = %v, want %v", stopEv.FinishReason, provider.FinishReasonStop)
	}
}

// TestChat_ContentBlockStart 验证 content_block_start 事件触发 BlockStart delta。
func TestChat_ContentBlockStart(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 BlockStart delta（text 类型）
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta, not found")
	}
	bsData, ok := blockStartEv.Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatalf("expected BlockStartData, got %T", blockStartEv.Delta.Data)
	}
	if bsData.BlockType != "text" {
		t.Errorf("BlockType = %q, want 'text'", bsData.BlockType)
	}
}

// TestChat_ContentBlockStart_Thinking 验证 thinking 类型的 content_block_start。
func TestChat_ContentBlockStart_Thinking(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"Let me think..."}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 thinking 类型的 BlockStart
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta for thinking, not found")
	}
	bsData, ok := blockStartEv.Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatalf("expected BlockStartData, got %T", blockStartEv.Delta.Data)
	}
	if bsData.BlockType != "thinking" {
		t.Errorf("BlockType = %q, want 'thinking'", bsData.BlockType)
	}

	// thinking content_block_start 带初始内容时应发出 ThinkingDelta
	thinkEv, ok := findDeltaByType(events, provider.StreamDeltaTypeThinking)
	if !ok {
		t.Fatal("expected ThinkingDelta, not found")
	}
	thinkData, ok := thinkEv.Delta.Data.(provider.ThinkingData)
	if !ok || string(thinkData) != "Let me think..." {
		t.Errorf("expected ThinkingData 'Let me think...', got %v", thinkEv.Delta.Data)
	}
}

// TestChat_ContentBlockStart_ToolUse 验证 tool_use 类型的 content_block_start。
func TestChat_ContentBlockStart_ToolUse(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 tool_use 类型的 BlockStart，携带 ID 和 Name
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta for tool_use, not found")
	}
	bsData, ok := blockStartEv.Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatalf("expected BlockStartData, got %T", blockStartEv.Delta.Data)
	}
	if bsData.BlockType != "tool_use" {
		t.Errorf("BlockType = %q, want 'tool_use'", bsData.BlockType)
	}
	if bsData.ID != "toolu_01" {
		t.Errorf("ID = %q, want 'toolu_01'", bsData.ID)
	}
	if bsData.Name != "get_weather" {
		t.Errorf("Name = %q, want 'get_weather'", bsData.Name)
	}
}

// TestChat_ContentBlockDelta 验证 content_block_delta 事件（text_delta / thinking_delta / input_json_delta）。
func TestChat_ContentBlockDelta(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含两个 TextOutput delta
	var textDeltas []provider.StreamEvent
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeTextOutput {
			textDeltas = append(textDeltas, ev)
		}
	}
	if len(textDeltas) != 2 {
		t.Fatalf("expected 2 text deltas, got %d", len(textDeltas))
	}

	d1, _ := textDeltas[0].Delta.Data.(provider.TextData)
	d2, _ := textDeltas[1].Delta.Data.(provider.TextData)
	if string(d1) != "Hello" {
		t.Errorf("first text delta = %q, want 'Hello'", string(d1))
	}
	if string(d2) != " world" {
		t.Errorf("second text delta = %q, want ' world'", string(d2))
	}
}

// TestChat_ContentBlockDelta_InputJSONDelta 验证 input_json_delta 事件。
func TestChat_ContentBlockDelta_InputJSONDelta(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Tokyo\"}"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含两个 ToolCallDelta
	var toolDeltas []provider.StreamEvent
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeToolCallDelta {
			toolDeltas = append(toolDeltas, ev)
		}
	}
	if len(toolDeltas) != 2 {
		t.Fatalf("expected 2 tool_call_delta events, got %d", len(toolDeltas))
	}

	d1, _ := toolDeltas[0].Delta.Data.(provider.ToolCallDeltaData)
	d2, _ := toolDeltas[1].Delta.Data.(provider.ToolCallDeltaData)
	if string(d1) != `{"city":` {
		t.Errorf("first partial_json = %q, want '{\"city\":'", string(d1))
	}
	if string(d2) != `"Tokyo"}` {
		t.Errorf("second partial_json = %q, want '\"Tokyo\"}'", string(d2))
	}
}

// TestChat_MessageStop 验证 message_stop 事件触发 Stop 事件并携带 FinishReason。
func TestChat_MessageStop(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 Stop 事件，FinishReason 为 Stop（end_turn → Stop）
	stopEv, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop event, not found")
	}
	if stopEv.FinishReason != provider.FinishReasonStop {
		t.Errorf("FinishReason = %v, want %v (end_turn → Stop)", stopEv.FinishReason, provider.FinishReasonStop)
	}
}

// TestChat_MessageStop_ToolUse 验证 tool_use stop_reason 映射为 ToolCalls。
func TestChat_MessageStop_ToolUse(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	stopEv, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop event, not found")
	}
	if stopEv.FinishReason != provider.FinishReasonToolCalls {
		t.Errorf("FinishReason = %v, want %v (tool_use → ToolCalls)", stopEv.FinishReason, provider.FinishReasonToolCalls)
	}
}

// TestChat_ErrorResponse 验证 HTTP 429 错误触发 Error 事件。
func TestChat_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 Error 事件
	errEv, ok := findEventByType(events, provider.StreamTypeError)
	if !ok {
		t.Fatal("expected StreamTypeError event for HTTP 429, not found")
	}
	if errEv.Err == nil {
		t.Error("expected non-nil Err in error event")
	}
}

// TestChat_ErrorResponse_500 验证 HTTP 500 错误触发 Error 事件。
func TestChat_ErrorResponse_500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	errEv, ok := findEventByType(events, provider.StreamTypeError)
	if !ok {
		t.Fatal("expected StreamTypeError event for HTTP 500, not found")
	}
	if errEv.Err == nil {
		t.Error("expected non-nil Err in error event")
	}
}

// TestChat_ThinkingWithSignature 验证 thinking block + signature_delta 全链路。
//
// Anthropic extended thinking 流程：
// 1. content_block_start (thinking) → BlockStart("thinking")
// 2. content_block_delta (thinking_delta) → ThinkingDelta
// 3. content_block_delta (signature_delta) → SignatureDelta
// 4. content_block_stop → BlockStop
func TestChat_ThinkingWithSignature(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Reasoning step 1..."}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"EvEFCu4F..."}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"The answer is 42."}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 1. 应包含 thinking 类型的 BlockStart
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta, not found")
	}
	bsData, _ := blockStartEv.Delta.Data.(provider.BlockStartData)
	if bsData.BlockType != "thinking" {
		t.Errorf("BlockType = %q, want 'thinking'", bsData.BlockType)
	}

	// 2. 应包含 ThinkingDelta
	thinkEv, ok := findDeltaByType(events, provider.StreamDeltaTypeThinking)
	if !ok {
		t.Fatal("expected ThinkingDelta, not found")
	}
	thinkData, _ := thinkEv.Delta.Data.(provider.ThinkingData)
	if string(thinkData) != "Reasoning step 1..." {
		t.Errorf("ThinkingData = %q, want 'Reasoning step 1...'", string(thinkData))
	}

	// 3. 应包含 SignatureDelta
	sigEv, ok := findDeltaByType(events, provider.StreamDeltaTypeSignature)
	if !ok {
		t.Fatal("expected SignatureDelta, not found")
	}
	sigData, _ := sigEv.Delta.Data.(provider.SignatureData)
	if string(sigData) != "EvEFCu4F..." {
		t.Errorf("SignatureData = %q, want 'EvEFCu4F...'", string(sigData))
	}

	// 4. 应包含 TextOutput delta（第二个 block）
	textEv, ok := findDeltaByType(events, provider.StreamDeltaTypeTextOutput)
	if !ok {
		t.Fatal("expected TextOutput delta, not found")
	}
	textData, _ := textEv.Delta.Data.(provider.TextData)
	if string(textData) != "The answer is 42." {
		t.Errorf("TextData = %q, want 'The answer is 42.'", string(textData))
	}
}

// TestChat_BlockStop 验证 content_block_stop 事件触发 BlockStop delta。
func TestChat_BlockStop(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 BlockStop delta，携带 index=0
	blockStopEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStop)
	if !ok {
		t.Fatal("expected BlockStop delta, not found")
	}
	bsData, ok := blockStopEv.Delta.Data.(provider.BlockStopData)
	if !ok {
		t.Fatalf("expected BlockStopData, got %T", blockStopEv.Delta.Data)
	}
	if bsData.Index != 0 {
		t.Errorf("BlockStop Index = %d, want 0", bsData.Index)
	}
	if !bsData.HasIndex {
		t.Error("BlockStop HasIndex = false, want true")
	}
}

// TestChat_PingEvent 验证 ping 事件被正确跳过。
func TestChat_PingEvent(t *testing.T) {
	fixture := sseFixture(
		[2]string{"ping", `{"type":"ping"}`},
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// ping 事件不应产生任何 delta，但 Start/Done 应正常
	if _, ok := findEventByType(events, provider.StreamTypeStart); !ok {
		t.Error("expected StreamTypeStart event despite leading ping, not found")
	}
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event, not found")
	}

	// 应有文本输出
	textEv, ok := findDeltaByType(events, provider.StreamDeltaTypeTextOutput)
	if !ok {
		t.Fatal("expected TextOutput delta, not found")
	}
	textData, _ := textEv.Delta.Data.(provider.TextData)
	if string(textData) != "Hi" {
		t.Errorf("TextData = %q, want 'Hi'", string(textData))
	}
}

// TestChat_MaxTokensFinishReason 验证 max_tokens stop_reason 映射为 Length。
func TestChat_MaxTokensFinishReason(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"..."}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	stopEv, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop event, not found")
	}
	if stopEv.FinishReason != provider.FinishReasonLength {
		t.Errorf("FinishReason = %v, want %v (max_tokens → Length)", stopEv.FinishReason, provider.FinishReasonLength)
	}
}

// TestChat_RequestFormat 验证 Chat 请求的 HTTP 方法和路径。
func TestChat_RequestFormat(t *testing.T) {
	var capturedMethod, capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseFixture(
			[2]string{"message_stop", `{"type":"message_stop"}`},
		)))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	_ = drainEvents(ch)

	if capturedMethod != http.MethodPost {
		t.Errorf("HTTP method = %q, want %q", capturedMethod, http.MethodPost)
	}
	if capturedPath != "/v1/messages" {
		t.Errorf("URL path = %q, want /v1/messages", capturedPath)
	}
}

// TestChat_AuthHeader 验证 x-api-key 认证头注入。
func TestChat_AuthHeader(t *testing.T) {
	var capturedAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseFixture(
			[2]string{"message_stop", `{"type":"message_stop"}`},
		)))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	_ = drainEvents(ch)

	if capturedAPIKey != "test-key" {
		t.Errorf("x-api-key header = %q, want 'test-key'", capturedAPIKey)
	}
}

// TestChat_AnthropicVersionHeader 验证 anthropic-version 请求头注入。
func TestChat_AnthropicVersionHeader(t *testing.T) {
	var capturedVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseFixture(
			[2]string{"message_stop", `{"type":"message_stop"}`},
		)))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	_ = drainEvents(ch)

	if capturedVersion != "2023-06-01" {
		t.Errorf("anthropic-version header = %q, want '2023-06-01'", capturedVersion)
	}
}

// TestChat_StreamFlagInBody 验证请求体中 stream=true。
func TestChat_StreamFlagInBody(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = readBody(r)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseFixture(
			[2]string{"message_stop", `{"type":"message_stop"}`},
		)))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	_ = drainEvents(ch)

	if !strings.Contains(string(capturedBody), `"stream":true`) {
		t.Errorf("request body should contain stream:true, got: %s", string(capturedBody))
	}
}

// TestChat_EmptyStream 验证空 SSE 流（无事件）不 panic，仍发送 Done。
func TestChat_EmptyStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 不写入任何事件，直接关闭
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 空流不应有 Start（无有效帧），但应有 Done
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event even for empty stream, not found")
	}
}

// TestChat_ContextCancellation 验证 context 取消时 goroutine 正确退出。
func TestChat_ContextCancellation(t *testing.T) {
	// 使用一个会阻塞的 server，模拟慢响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 发送一个事件后阻塞
		_, _ = w.Write([]byte(sseFixture(
			[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		)))
		// flush 后保持连接
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)

	// 等待收到 Start 事件
	select {
	case ev := <-ch:
		if ev.Type != provider.StreamTypeStart {
			t.Errorf("first event type = %v, want Start", ev.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for first event")
	}

	// 取消 context
	cancel()

	// channel 应关闭
	select {
	case _, ok := <-ch:
		if ok {
			// 可能还有残余事件，继续 drain
			for range ch {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed after context cancellation")
	}
}

// TestChat_ErrorEventInStream 验证 SSE 流中的 error 事件触发 Error 事件。
func TestChat_ErrorEventInStream(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"error", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 Error 事件
	errEv, ok := findEventByType(events, provider.StreamTypeError)
	if !ok {
		t.Fatal("expected StreamTypeError event for SSE error event, not found")
	}
	if errEv.Err == nil {
		t.Error("expected non-nil Err in error event")
	}
}

// TestChat_MultipleBlocks 验证多个内容块的完整流程。
func TestChat_MultipleBlocks(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`},
		// Block 0: text
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		// Block 1: tool_use
		[2]string{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"search"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"test\"}"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		// End
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "claude-sonnet-4-20250514", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应有 2 个 BlockStart（text + tool_use）
	var blockStarts []provider.BlockStartData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeBlockStart {
			if data, ok := ev.Delta.Data.(provider.BlockStartData); ok {
				blockStarts = append(blockStarts, data)
			}
		}
	}
	if len(blockStarts) != 2 {
		t.Fatalf("expected 2 BlockStart events, got %d", len(blockStarts))
	}
	if blockStarts[0].BlockType != "text" {
		t.Errorf("first BlockStart type = %q, want 'text'", blockStarts[0].BlockType)
	}
	if blockStarts[1].BlockType != "tool_use" {
		t.Errorf("second BlockStart type = %q, want 'tool_use'", blockStarts[1].BlockType)
	}

	// 应有 2 个 BlockStop
	var blockStops []provider.BlockStopData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeBlockStop {
			if data, ok := ev.Delta.Data.(provider.BlockStopData); ok {
				blockStops = append(blockStops, data)
			}
		}
	}
	if len(blockStops) != 2 {
		t.Fatalf("expected 2 BlockStop events, got %d", len(blockStops))
	}

	// FinishReason 应为 ToolCalls
	stopEv, _ := findEventByType(events, provider.StreamTypeStop)
	if stopEv.FinishReason != provider.FinishReasonToolCalls {
		t.Errorf("FinishReason = %v, want %v", stopEv.FinishReason, provider.FinishReasonToolCalls)
	}
}

// ==============================
// 辅助函数
// ==============================

// readBody 读取 http.Request 的 body 并返回字节。
func readBody(r *http.Request) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

// 确保 fmt 包被使用（避免 unused import）
var _ = fmt.Sprintf

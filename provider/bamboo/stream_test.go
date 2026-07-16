package bamboo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// Chat 流式测试
// ==============================

// TestChat_FullLifecycle 验证完整流式生命周期：
// Start → BlockStart → TextDelta → BlockStop → Stop(FinishReason) → Done
func TestChat_FullLifecycle(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 必须包含 Start
	if _, ok := findEventByType(events, provider.StreamTypeStart); !ok {
		t.Error("expected StreamTypeStart event, not found")
	}

	// 必须包含 Done
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event, not found")
	}

	// 必须包含 BlockStart delta (text)
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta, not found")
	}
	bsData, _ := blockStartEv.Delta.Data.(provider.BlockStartData)
	if bsData.BlockType != "text" {
		t.Errorf("BlockStart BlockType = %q, want 'text'", bsData.BlockType)
	}

	// 必须包含 BlockStop delta (index=0)
	blockStopEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStop)
	if !ok {
		t.Fatal("expected BlockStop delta, not found")
	}
	bstopData, _ := blockStopEv.Delta.Data.(provider.BlockStopData)
	if bstopData.Index != 0 || !bstopData.HasIndex {
		t.Errorf("BlockStop = %+v, want Index=0 HasIndex=true", bstopData)
	}

	// 必须包含 2 个 TextOutput delta
	var textDeltas []provider.TextData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeTextOutput {
			d, _ := ev.Delta.Data.(provider.TextData)
			textDeltas = append(textDeltas, d)
		}
	}
	if len(textDeltas) != 2 {
		t.Fatalf("expected 2 text deltas, got %d", len(textDeltas))
	}
	if string(textDeltas[0]) != "Hello" {
		t.Errorf("first text delta = %q, want 'Hello'", string(textDeltas[0]))
	}
	if string(textDeltas[1]) != " world" {
		t.Errorf("second text delta = %q, want ' world'", string(textDeltas[1]))
	}

	// Stop 事件携带 FinishReason
	stopEv, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop event, not found")
	}
	if stopEv.FinishReason != provider.FinishReasonStop {
		t.Errorf("FinishReason = %v, want %v (end_turn → Stop)", stopEv.FinishReason, provider.FinishReasonStop)
	}
}

// TestChat_MessageStartUsage 验证 message_start 的 usage 提取。
func TestChat_MessageStartUsage(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":42,"output_tokens":0,"cache_creation_input_tokens":3,"cache_read_input_tokens":7}}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应包含 Usage delta 携带 cache 统计
	usageEv, ok := findDeltaByType(events, provider.StreamDeltaTypeUsage)
	if !ok {
		t.Fatal("expected Usage delta, not found")
	}
	usageData, _ := usageEv.Delta.Data.(provider.UsageData)
	if usageData.InputTokens != 42 {
		t.Errorf("InputTokens = %d, want 42", usageData.InputTokens)
	}
	if usageData.CacheCreationInputTokens != 3 {
		t.Errorf("CacheCreationInputTokens = %d, want 3", usageData.CacheCreationInputTokens)
	}
	if usageData.CacheReadInputTokens != 7 {
		t.Errorf("CacheReadInputTokens = %d, want 7", usageData.CacheReadInputTokens)
	}
}

// TestChat_ThinkingBlock 验证 thinking content_block_start + thinking_delta + signature_delta。
func TestChat_ThinkingBlock(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"Initial thought"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" more reasoning"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_123"}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// thinking BlockStart
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta, not found")
	}
	bsData, _ := blockStartEv.Delta.Data.(provider.BlockStartData)
	if bsData.BlockType != "thinking" {
		t.Errorf("BlockType = %q, want 'thinking'", bsData.BlockType)
	}

	// thinking content_block_start 带初始内容时应有 ThinkingDelta
	// 应有 2 个 ThinkingDelta (初始 + delta)
	var thinkDeltas []provider.ThinkingData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeThinking {
			d, _ := ev.Delta.Data.(provider.ThinkingData)
			thinkDeltas = append(thinkDeltas, d)
		}
	}
	if len(thinkDeltas) != 2 {
		t.Fatalf("expected 2 thinking deltas, got %d", len(thinkDeltas))
	}
	if string(thinkDeltas[0]) != "Initial thought" {
		t.Errorf("first thinking delta = %q, want 'Initial thought'", string(thinkDeltas[0]))
	}

	// signature delta
	sigEv, ok := findDeltaByType(events, provider.StreamDeltaTypeSignature)
	if !ok {
		t.Fatal("expected SignatureDelta, not found")
	}
	sigData, _ := sigEv.Delta.Data.(provider.SignatureData)
	if string(sigData) != "sig_123" {
		t.Errorf("SignatureData = %q, want 'sig_123'", string(sigData))
	}
}

// TestChat_ToolUseBlock 验证 tool_use content_block_start 使用 NewBlockStartDeltaWithID。
func TestChat_ToolUseBlock(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Tokyo\"}"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// tool_use BlockStart 携带 ID 和 Name
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta, not found")
	}
	bsData, _ := blockStartEv.Delta.Data.(provider.BlockStartData)
	if bsData.BlockType != "tool_use" {
		t.Errorf("BlockType = %q, want 'tool_use'", bsData.BlockType)
	}
	if bsData.ID != "toolu_01" {
		t.Errorf("ID = %q, want 'toolu_01'", bsData.ID)
	}
	if bsData.Name != "get_weather" {
		t.Errorf("Name = %q, want 'get_weather'", bsData.Name)
	}

	// input_json_delta → ToolCallDeltaData
	var toolDeltas []provider.ToolCallDeltaData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeToolCallDelta {
			d, _ := ev.Delta.Data.(provider.ToolCallDeltaData)
			toolDeltas = append(toolDeltas, d)
		}
	}
	if len(toolDeltas) != 2 {
		t.Fatalf("expected 2 tool_call_delta, got %d", len(toolDeltas))
	}

	// tool_use → FinishReasonToolCalls
	stopEv, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop, not found")
	}
	if stopEv.FinishReason != provider.FinishReasonToolCalls {
		t.Errorf("FinishReason = %v, want %v (tool_use → ToolCalls)", stopEv.FinishReason, provider.FinishReasonToolCalls)
	}
}

// TestChat_RedactedThinking 验证 redacted_thinking 产生 BlockStart + RedactedThinkingDelta。
func TestChat_RedactedThinking(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"encrypted_blob_123"}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// BlockStart for redacted_thinking
	blockStartEv, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta, not found")
	}
	bsData, _ := blockStartEv.Delta.Data.(provider.BlockStartData)
	if bsData.BlockType != "redacted_thinking" {
		t.Errorf("BlockType = %q, want 'redacted_thinking'", bsData.BlockType)
	}

	// RedactedThinkingDelta 携带 data
	redactedEv, ok := findDeltaByType(events, provider.StreamDeltaTypeRedactedThinking)
	if !ok {
		t.Fatal("expected RedactedThinkingDelta, not found")
	}
	redactedData, _ := redactedEv.Delta.Data.(provider.RedactedThinkingData)
	if string(redactedData) != "encrypted_blob_123" {
		t.Errorf("RedactedThinkingData = %q, want 'encrypted_blob_123'", string(redactedData))
	}
}

// TestChat_PingSkipped 验证 ping 事件被跳过（不产生 provider 事件）。
func TestChat_PingSkipped(t *testing.T) {
	fixture := sseFixture(
		[2]string{"ping", `{"type":"ping"}`},
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// ping 不产生任何 delta，但 Start/Done 应正常
	if _, ok := findEventByType(events, provider.StreamTypeStart); !ok {
		t.Error("expected StreamTypeStart despite leading ping, not found")
	}
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone, not found")
	}
	// 文本输出正常
	textEv, ok := findDeltaByType(events, provider.StreamDeltaTypeTextOutput)
	if !ok {
		t.Fatal("expected TextOutput delta, not found")
	}
	textData, _ := textEv.Delta.Data.(provider.TextData)
	if string(textData) != "Hi" {
		t.Errorf("TextData = %q, want 'Hi'", string(textData))
	}
}

// TestChat_HTTPError 验证 HTTP 500 错误触发 Error 事件携带 StatusCode。
func TestChat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"category":"internal","message":"Internal server error","status_code":500}}`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	errEv, ok := findEventByType(events, provider.StreamTypeError)
	if !ok {
		t.Fatal("expected StreamTypeError for HTTP 500, not found")
	}
	if errEv.Err == nil {
		t.Error("expected non-nil Err in error event")
	}
	if errEv.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", errEv.StatusCode)
	}
}

// TestChat_SSEErrorEvent 验证 SSE 流中的 error 事件。
func TestChat_SSEErrorEvent(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"error", `{"type":"error","error":{"category":"overloaded","message":"Overloaded"}}`},
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	errEv, ok := findEventByType(events, provider.StreamTypeError)
	if !ok {
		t.Fatal("expected StreamTypeError for SSE error event, not found")
	}
	if errEv.Err == nil {
		t.Error("expected non-nil Err")
	}
	if !strings.Contains(errEv.Err.Message, "Overloaded") {
		t.Errorf("Err.Message = %q, want contains 'Overloaded'", errEv.Err.Message)
	}
}

// TestChat_MessageDeltaUsage 验证 message_delta 携带 usage 时发出 UsageDelta。
func TestChat_MessageDeltaUsage(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":42}}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 应有 2 个 Usage delta（message_start + message_delta）
	var usageDeltas []provider.UsageData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeUsage {
			d, _ := ev.Delta.Data.(provider.UsageData)
			usageDeltas = append(usageDeltas, d)
		}
	}
	if len(usageDeltas) != 2 {
		t.Fatalf("expected 2 usage deltas, got %d", len(usageDeltas))
	}
	// 第二个应携带 output_tokens=42
	if usageDeltas[1].OutputTokens != 42 {
		t.Errorf("second usage OutputTokens = %d, want 42", usageDeltas[1].OutputTokens)
	}
}

// TestChat_MaxTokensFinishReason 验证 max_tokens → Length。
func TestChat_MaxTokensFinishReason(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"..."}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"max_tokens"}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	stopEv, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop, not found")
	}
	if stopEv.FinishReason != provider.FinishReasonLength {
		t.Errorf("FinishReason = %v, want %v (max_tokens → Length)", stopEv.FinishReason, provider.FinishReasonLength)
	}
}

// TestChat_AllFinishReasons 参数化测试所有 7 种流式 FinishReason 映射。
func TestChat_AllFinishReasons(t *testing.T) {
	tests := []struct {
		wireReason string
		want       provider.FinishReason
	}{
		{"end_turn", provider.FinishReasonStop},
		{"max_tokens", provider.FinishReasonLength},
		{"tool_use", provider.FinishReasonToolCalls},
		{"stop_sequence", provider.FinishReasonStop},
		{"pause_turn", provider.FinishReasonPauseTurn},
		{"refusal", provider.FinishReasonRefusal},
		{"server_tool_use", provider.FinishReasonServerToolUse},
	}

	for _, tt := range tests {
		t.Run(tt.wireReason, func(t *testing.T) {
			fixture := sseFixture(
				[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
				[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
				[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
				[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"` + tt.wireReason + `"}}`},
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
			config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
			messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

			ch := p.Chat(ctx, messages, config)
			events := drainEvents(ch)

			stopEv, ok := findEventByType(events, provider.StreamTypeStop)
			if !ok {
				t.Fatal("expected StreamTypeStop, not found")
			}
			if stopEv.FinishReason != tt.want {
				t.Errorf("stop_reason %q → FinishReason = %v, want %v", tt.wireReason, stopEv.FinishReason, tt.want)
			}
		})
	}
}

// TestChat_StreamFlagInBody 验证请求体包含 stream:true。
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	_ = drainEvents(ch)

	if !strings.Contains(string(capturedBody), `"stream":true`) {
		t.Errorf("request body should contain stream:true, got: %s", string(capturedBody))
	}
}

// TestChat_RequestFormat 验证 HTTP 方法和路径。
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	_ = drainEvents(ch)

	if capturedMethod != http.MethodPost {
		t.Errorf("HTTP method = %q, want %q", capturedMethod, http.MethodPost)
	}
	if capturedPath != "/v1/bamboo" {
		t.Errorf("URL path = %q, want /v1/bamboo", capturedPath)
	}
}

// TestChat_EmptyStream 验证空 SSE 流仍发送 Done。
func TestChat_EmptyStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone even for empty stream, not found")
	}
}

// TestChat_ContextCancellation 验证 context 取消时 goroutine 正确退出。
func TestChat_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseFixture(
			[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		)))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)

	// 等待收到 Start
	select {
	case ev := <-ch:
		if ev.Type != provider.StreamTypeStart {
			t.Errorf("first event type = %v, want Start", ev.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for first event")
	}

	cancel()

	// channel 应关闭
	select {
	case _, ok := <-ch:
		if ok {
			for range ch {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed after context cancellation")
	}
}

// TestChat_MultipleBlocks 验证多内容块（text + tool_use）的完整 BlockStart/Stop 序列。
func TestChat_MultipleBlocks(t *testing.T) {
	fixture := sseFixture(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_001","usage":{"input_tokens":10,"output_tokens":0}}}`},
		// Block 0: text
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		// Block 1: tool_use
		[2]string{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"search"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"test\"}"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		// End
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`},
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
	config := &provider.ChatConfig{Model: "test-model", MaxTokens: 100}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := drainEvents(ch)

	// 2 个 BlockStart
	var blockStarts []provider.BlockStartData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeBlockStart {
			d, _ := ev.Delta.Data.(provider.BlockStartData)
			blockStarts = append(blockStarts, d)
		}
	}
	if len(blockStarts) != 2 {
		t.Fatalf("expected 2 BlockStart, got %d", len(blockStarts))
	}
	if blockStarts[0].BlockType != "text" {
		t.Errorf("first BlockStart = %q, want 'text'", blockStarts[0].BlockType)
	}
	if blockStarts[1].BlockType != "tool_use" {
		t.Errorf("second BlockStart = %q, want 'tool_use'", blockStarts[1].BlockType)
	}

	// 2 个 BlockStop
	var blockStops []provider.BlockStopData
	for _, ev := range events {
		if ev.Type == provider.StreamTypeDelta && ev.Delta.Type == provider.StreamDeltaTypeBlockStop {
			d, _ := ev.Delta.Data.(provider.BlockStopData)
			blockStops = append(blockStops, d)
		}
	}
	if len(blockStops) != 2 {
		t.Fatalf("expected 2 BlockStop, got %d", len(blockStops))
	}

	// FinishReason = ToolCalls
	stopEv, _ := findEventByType(events, provider.StreamTypeStop)
	if stopEv.FinishReason != provider.FinishReasonToolCalls {
		t.Errorf("FinishReason = %v, want %v", stopEv.FinishReason, provider.FinishReasonToolCalls)
	}
}

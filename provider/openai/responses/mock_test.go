package responses

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
// Mock HTTP 测试 — 基于 httptest.Server
// ==============================
//
// 以下测试通过 httptest.Server 模拟 OpenAI Responses API 的 SSE 流式响应，
// 验证 ChatWithSystem 的完整链路：HTTP 请求 → SSE 解析 → 事件分发 → channel 输出。
// 所有测试零 SDK 依赖，纯标准库实现。

// newMockProvider 创建指向 mock server 的 ResponsesProvider 实例。
func newMockProvider(t *testing.T, server *httptest.Server) *ResponsesProvider {
	t.Helper()
	return NewResponsesProviderWithOptions(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)
}

// drainEvents 从 channel 中收集所有事件直到关闭。
func drainEvents(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// sseFrame 构造一个 SSE 帧（event + data 行）。
func sseFrame(eventType, jsonData string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, jsonData)
}

// sseDataOnly 构造一个仅含 data 行的 SSE 帧。
func sseDataOnly(jsonData string) string {
	return fmt.Sprintf("data: %s\n\n", jsonData)
}

// findEventByType 在事件列表中查找指定类型的第一个事件。
func findEventByType(events []provider.StreamEvent, t provider.StreamType) (provider.StreamEvent, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}
	return provider.StreamEvent{}, false
}

// findDeltaByType 在事件列表中查找指定 Delta 类型的第一个事件。
func findDeltaByType(events []provider.StreamEvent, dt provider.StreamDeltaType) (provider.StreamEvent, bool) {
	for _, e := range events {
		if e.Delta.Type == dt {
			return e, true
		}
	}
	return provider.StreamEvent{}, false
}

// ==============================
// TestChat_ResponseCreated
// ==============================

// TestChat_ResponseCreated 验证 response.created 事件正确触发 MetadataDelta。
//
// 模拟 OpenAI Responses API 返回 response.created SSE 事件，
// 断言 channel 中包含 Start 事件和携带 ResponseID 的 MetadataDelta 事件。
func TestChat_ResponseCreated(t *testing.T) {
	createdJSON := `{"type":"response.created","response":{"id":"resp_mock_001","status":"in_progress","model":"gpt-4o","output":[]}}`
	completedJSON := `{"type":"response.completed","response":{"id":"resp_mock_001","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":10,"output_tokens":5}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseFrame("response.created", createdJSON))
		fmt.Fprint(w, sseFrame("response.completed", completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hello"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	// 验证 Start 事件存在
	if _, ok := findEventByType(events, provider.StreamTypeStart); !ok {
		t.Error("expected StreamTypeStart event")
	}

	// 验证 MetadataDelta 携带 ResponseID
	metaEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeMetadata)
	if !ok {
		t.Fatal("expected MetadataDelta event")
	}
	data, ok := metaEvent.Delta.Data.(provider.MetadataData)
	if !ok {
		t.Fatalf("expected MetadataData, got %T", metaEvent.Delta.Data)
	}
	if data.ResponseID != "resp_mock_001" {
		t.Errorf("ResponseID = %q, want resp_mock_001", data.ResponseID)
	}

	// 验证 Done 事件存在
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event")
	}
}

// ==============================
// TestChat_OutputTextDelta
// ==============================

// TestChat_OutputTextDelta 验证 output_text.delta 事件正确触发 BlockStart + TextDelta。
//
// 模拟多个 output_text.delta 事件，断言首个 delta 前有 BlockStart("text")，
// 后续 delta 为纯 TextOutput 事件。
func TestChat_OutputTextDelta(t *testing.T) {
	delta1 := `{"type":"response.output_text.delta","output_index":0,"content_index":0,"text":"Hello"}`
	delta2 := `{"type":"response.output_text.delta","output_index":0,"content_index":0,"text":" world"}`
	completedJSON := `{"type":"response.completed","response":{"id":"resp_002","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":10,"output_tokens":5}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseFrame("response.output_text.delta", delta1))
		fmt.Fprint(w, sseFrame("response.output_text.delta", delta2))
		fmt.Fprint(w, sseFrame("response.completed", completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Say hello"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	// 验证 BlockStart 事件存在且类型为 "text"
	blockStartEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart)
	if !ok {
		t.Fatal("expected BlockStart delta event")
	}
	bsData, ok := blockStartEvent.Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatalf("expected BlockStartData, got %T", blockStartEvent.Delta.Data)
	}
	if bsData.BlockType != "text" {
		t.Errorf("BlockStart type = %q, want 'text'", bsData.BlockType)
	}

	// 验证有两个 TextOutput delta
	textCount := 0
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeTextOutput {
			textCount++
		}
	}
	if textCount != 2 {
		t.Errorf("expected 2 TextOutput deltas, got %d", textCount)
	}

	// 验证 Done 事件存在
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event")
	}
}

// ==============================
// TestChat_FunctionCall
// ==============================

// TestChat_FunctionCall 验证 function_call_arguments.delta 事件正确触发 ToolCallDelta 事件。
//
// 模拟 output_item.added（function_call）+ function_call_arguments.delta 事件，
// 断言 channel 中包含 ToolCall 开始事件和 ToolCallDelta 参数增量事件。
func TestChat_FunctionCall(t *testing.T) {
	itemAdded := `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_001","call_id":"call_001","name":"get_weather","arguments":""}}`
	argDelta1 := `{"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"city\""}`
	argDelta2 := `{"type":"response.function_call_arguments.delta","output_index":0,"delta":":\"Beijing\"}"}`
	completedJSON := `{"type":"response.completed","response":{"id":"resp_003","status":"completed","model":"gpt-4o","output":[{"type":"function_call","id":"fc_001","call_id":"call_001","name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}],"usage":{"input_tokens":10,"output_tokens":5}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseFrame("response.output_item.added", itemAdded))
		fmt.Fprint(w, sseFrame("response.function_call_arguments.delta", argDelta1))
		fmt.Fprint(w, sseFrame("response.function_call_arguments.delta", argDelta2))
		fmt.Fprint(w, sseFrame("response.completed", completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "What's the weather in Beijing?"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	// 验证 ToolCall 开始事件（携带 call_id 和 name）
	toolCallEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeToolCall)
	if !ok {
		t.Fatal("expected ToolCall delta event")
	}
	tcData, ok := toolCallEvent.Delta.Data.(provider.ToolCallData)
	if !ok {
		t.Fatalf("expected ToolCallData, got %T", toolCallEvent.Delta.Data)
	}
	if tcData.ID != "call_001" {
		t.Errorf("ToolCallData.ID = %q, want call_001", tcData.ID)
	}
	if tcData.Name != "get_weather" {
		t.Errorf("ToolCallData.Name = %q, want get_weather", tcData.Name)
	}

	// 验证有两个 ToolCallDelta（参数增量）
	deltaCount := 0
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeToolCallDelta {
			deltaCount++
		}
	}
	if deltaCount != 2 {
		t.Errorf("expected 2 ToolCallDelta events, got %d", deltaCount)
	}

	// 验证 Stop 事件存在（response.completed 触发）
	if _, ok := findEventByType(events, provider.StreamTypeStop); !ok {
		t.Error("expected StreamTypeStop event")
	}
}

// ==============================
// TestChat_ResponseCompleted
// ==============================

// TestChat_ResponseCompleted 验证 response.completed 事件正确触发 Usage + Stop 事件。
//
// 模拟 response.completed 事件携带 usage 信息，
// 断言 channel 中包含 UsageDelta 和 StreamTypeStop 事件。
func TestChat_ResponseCompleted(t *testing.T) {
	completedJSON := `{"type":"response.completed","response":{"id":"resp_004","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseFrame("response.completed", completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	// 验证 UsageDelta 事件
	usageEvent, ok := findDeltaByType(events, provider.StreamDeltaTypeUsage)
	if !ok {
		t.Fatal("expected UsageDelta event")
	}
	usageData, ok := usageEvent.Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatalf("expected UsageData, got %T", usageEvent.Delta.Data)
	}
	if usageData.InputTokens != 100 {
		t.Errorf("UsageData.InputTokens = %d, want 100", usageData.InputTokens)
	}
	if usageData.OutputTokens != 50 {
		t.Errorf("UsageData.OutputTokens = %d, want 50", usageData.OutputTokens)
	}

	// 验证 Stop 事件存在
	stopEvent, ok := findEventByType(events, provider.StreamTypeStop)
	if !ok {
		t.Fatal("expected StreamTypeStop event")
	}
	if stopEvent.FinishReason != provider.FinishReasonStop {
		t.Errorf("FinishReason = %v, want %v", stopEvent.FinishReason, provider.FinishReasonStop)
	}

	// 验证 Done 事件存在
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event")
	}
}

// ==============================
// TestChat_ErrorResponse
// ==============================

// TestChat_ErrorResponse 验证 HTTP 500 错误响应正确触发 Error 事件。
//
// 模拟上游返回 HTTP 500 + JSON 错误体，
// 断言 channel 中包含 StreamTypeError 事件且不 panic。
func TestChat_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"Internal server error","type":"server_error","code":"internal_error"}}`)
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	// 验证 Error 事件存在
	errEvent, ok := findEventByType(events, provider.StreamTypeError)
	if !ok {
		t.Fatal("expected StreamTypeError event")
	}
	if errEvent.Err == nil {
		t.Error("expected non-nil Err in error event")
	}

	// 验证错误消息包含 HTTP 状态码
	errMsg := errEvent.Err.Error()
	if !strings.Contains(errMsg, "500") {
		t.Errorf("error message should contain HTTP 500, got: %s", errMsg)
	}

	// 验证 Done 事件存在
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event after error")
	}
}

// ==============================
// TestChat_DataOnlySSE
// ==============================

// TestChat_DataOnlySSE 验证无 event: 行的纯 data: 帧（OpenAI 官方格式）也能正确解析。
//
// OpenAI Responses API 的 SSE 事件通常只有 data: 行（JSON 内含 type 字段），
// 不包含 event: 行。此测试验证 SSEScanner + chat.go 的 eventType 补充逻辑。
func TestChat_DataOnlySSE(t *testing.T) {
	createdJSON := `{"type":"response.created","response":{"id":"resp_005","status":"in_progress","model":"gpt-4o","output":[]}}`
	deltaJSON := `{"type":"response.output_text.delta","output_index":0,"content_index":0,"text":"Hi there"}`
	completedJSON := `{"type":"response.completed","response":{"id":"resp_005","status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":5,"output_tokens":3}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 纯 data: 帧，无 event: 行（OpenAI 官方格式）
		fmt.Fprint(w, sseDataOnly(createdJSON))
		fmt.Fprint(w, sseDataOnly(deltaJSON))
		fmt.Fprint(w, sseDataOnly(completedJSON))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, &provider.ChatConfig{Model: "gpt-4o"})

	events := drainEvents(ch)

	// 验证 Start 事件
	if _, ok := findEventByType(events, provider.StreamTypeStart); !ok {
		t.Error("expected StreamTypeStart event")
	}

	// 验证 MetadataDelta（response.created）
	if _, ok := findDeltaByType(events, provider.StreamDeltaTypeMetadata); !ok {
		t.Error("expected MetadataDelta event from response.created")
	}

	// 验证 BlockStart + TextOutput（output_text.delta）
	if _, ok := findDeltaByType(events, provider.StreamDeltaTypeBlockStart); !ok {
		t.Error("expected BlockStart delta from output_text.delta")
	}
	if _, ok := findDeltaByType(events, provider.StreamDeltaTypeTextOutput); !ok {
		t.Error("expected TextOutput delta")
	}

	// 验证 UsageDelta + Stop（response.completed）
	if _, ok := findDeltaByType(events, provider.StreamDeltaTypeUsage); !ok {
		t.Error("expected UsageDelta from response.completed")
	}
	if _, ok := findEventByType(events, provider.StreamTypeStop); !ok {
		t.Error("expected StreamTypeStop event")
	}

	// 验证 Done
	if _, ok := findEventByType(events, provider.StreamTypeDone); !ok {
		t.Error("expected StreamTypeDone event")
	}
}

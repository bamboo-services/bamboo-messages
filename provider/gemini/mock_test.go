package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// mockGeminiServer 创建一个返回指定 SSE 帧序列的 httptest.Server。
//
// 每个 frame 是一个完整的 generateContentResponse JSON 对象，
// 会被包装为 "data: {json}\n\n" 格式的 SSE 帧。
// status 为 HTTP 响应状态码，400+ 表示错误响应（直接返回 body，不包装 SSE）。
func mockGeminiServer(t *testing.T, status int, frames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status >= 400 {
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":"test error","status":"INVALID_ARGUMENT"}}`, status)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}

		for _, frame := range frames {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
			flusher.Flush()
		}
	}))
}

// newTestProvider 创建指向 mock server 的 Provider 实例。
func newTestProvider(server *httptest.Server) *Provider {
	return NewProviderWithOptions(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)
}

// collectEvents 从 StreamEvent channel 收集所有事件直到关闭。
func collectEvents(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// findEventType 在事件列表中查找指定类型的事件，返回第一个匹配项的索引，未找到返回 -1。
func findEventType(events []provider.StreamEvent, typ provider.StreamType) int {
	for i, e := range events {
		if e.Type == typ {
			return i
		}
	}
	return -1
}

// findDeltaType 在事件列表中查找指定 Delta 类型的事件，返回第一个匹配项。
func findDeltaType(events []provider.StreamEvent, typ provider.StreamDeltaType) *provider.StreamEvent {
	for i := range events {
		if events[i].Delta.Type == typ {
			return &events[i]
		}
	}
	return nil
}

// ──────────────────────────────────────────────────────────────
// 测试用例
// ──────────────────────────────────────────────────────────────

// TestChat_TextStream 验证 Gemini 流式文本输出。
//
// Mock server 返回包含 text part 的 SSE 帧，
// 期望收到 Start → BlockStart("text") → TextDelta → Stop → Done 事件序列。
func TestChat_TextStream(t *testing.T) {
	frame := `{"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
	server := mockGeminiServer(t, 200, []string{frame})
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "gemini-2.5-flash"}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := collectEvents(ch)

	// 验证 Start 事件
	if findEventType(events, provider.StreamTypeStart) < 0 {
		t.Error("expected StreamTypeStart event")
	}

	// 验证 BlockStart("text")
	bs := findDeltaType(events, provider.StreamDeltaTypeBlockStart)
	if bs == nil {
		t.Fatal("expected BlockStart delta")
	}
	bsData, ok := bs.Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatal("BlockStart data type assertion failed")
	}
	if bsData.BlockType != "text" {
		t.Errorf("BlockStart type = %q, want text", bsData.BlockType)
	}

	// 验证 TextDelta
	td := findDeltaType(events, provider.StreamDeltaTypeTextOutput)
	if td == nil {
		t.Fatal("expected TextDelta")
	}
	textData, ok := td.Delta.Data.(provider.TextData)
	if !ok {
		t.Fatal("TextDelta data type assertion failed")
	}
	if string(textData) != "Hello" {
		t.Errorf("text = %q, want %q", string(textData), "Hello")
	}

	// 验证 Stop 事件（finishReason=STOP 在 handleCandidate 中不触发 Stop，由 chat.go 补发）
	if findEventType(events, provider.StreamTypeStop) < 0 {
		t.Error("expected StreamTypeStop event")
	}

	// 验证 Done 事件
	if findEventType(events, provider.StreamTypeDone) < 0 {
		t.Error("expected StreamTypeDone event")
	}

	// 验证 Usage
	ud := findDeltaType(events, provider.StreamDeltaTypeUsage)
	if ud == nil {
		t.Fatal("expected Usage delta")
	}
	usageData, ok := ud.Delta.Data.(provider.UsageData)
	if !ok {
		t.Fatal("Usage data type assertion failed")
	}
	if usageData.InputTokens != 10 || usageData.OutputTokens != 5 {
		t.Errorf("usage = input:%d output:%d, want input:10 output:5", usageData.InputTokens, usageData.OutputTokens)
	}
}

// TestChat_ThinkingParts 验证 Gemini 流式思考内容输出。
//
// Mock server 返回包含 thought=true 的 part，
// 期望收到 BlockStart("thinking") + ThinkingDelta 事件。
func TestChat_ThinkingParts(t *testing.T) {
	frame := `{"candidates":[{"content":{"parts":[{"text":"Let me think...","thought":true}],"role":"model"}}]}`
	server := mockGeminiServer(t, 200, []string{frame})
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "gemini-2.5-flash"}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Solve this problem"}}

	ch := p.Chat(ctx, messages, config)
	events := collectEvents(ch)

	// 验证 BlockStart("thinking")
	bs := findDeltaType(events, provider.StreamDeltaTypeBlockStart)
	if bs == nil {
		t.Fatal("expected BlockStart delta")
	}
	bsData, ok := bs.Delta.Data.(provider.BlockStartData)
	if !ok {
		t.Fatal("BlockStart data type assertion failed")
	}
	if bsData.BlockType != "thinking" {
		t.Errorf("BlockStart type = %q, want thinking", bsData.BlockType)
	}

	// 验证 ThinkingDelta
	td := findDeltaType(events, provider.StreamDeltaTypeThinking)
	if td == nil {
		t.Fatal("expected ThinkingDelta")
	}
	thinkingData, ok := td.Delta.Data.(provider.ThinkingData)
	if !ok {
		t.Fatal("ThinkingDelta data type assertion failed")
	}
	if string(thinkingData) != "Let me think..." {
		t.Errorf("thinking = %q, want %q", string(thinkingData), "Let me think...")
	}
}

// TestChat_FunctionCall 验证 Gemini 流式工具调用。
//
// Mock server 返回包含 functionCall 的 part，
// 期望收到 ToolCallDelta + ToolCallDeltaData 事件，且不发送 BlockStart。
func TestChat_FunctionCall(t *testing.T) {
	frame := `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_001","name":"get_weather","args":{"city":"Tokyo"}}}],"role":"model"},"finishReason":"STOP"}]}`
	server := mockGeminiServer(t, 200, []string{frame})
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "gemini-2.5-flash"}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "What's the weather?"}}

	ch := p.Chat(ctx, messages, config)
	events := collectEvents(ch)

	// 验证 ToolCallDelta（工具调用开始）
	tc := findDeltaType(events, provider.StreamDeltaTypeToolCall)
	if tc == nil {
		t.Fatal("expected ToolCallDelta")
	}
	tcData, ok := tc.Delta.Data.(provider.ToolCallData)
	if !ok {
		t.Fatal("ToolCallDelta data type assertion failed")
	}
	if tcData.Name != "get_weather" {
		t.Errorf("tool call name = %q, want get_weather", tcData.Name)
	}
	if tcData.ID != "call_001" {
		t.Errorf("tool call id = %q, want call_001", tcData.ID)
	}

	// 验证 ToolCallDeltaData（工具调用参数）
	tcd := findDeltaType(events, provider.StreamDeltaTypeToolCallDelta)
	if tcd == nil {
		t.Fatal("expected ToolCallDeltaData")
	}
	argsData, ok := tcd.Delta.Data.(provider.ToolCallDeltaData)
	if !ok {
		t.Fatal("ToolCallDeltaData type assertion failed")
	}
	// 验证 args 是有效的 JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(string(argsData)), &args); err != nil {
		t.Fatalf("tool call args is not valid JSON: %v", err)
	}
	if args["city"] != "Tokyo" {
		t.Errorf("args.city = %v, want Tokyo", args["city"])
	}

	// 验证工具调用不发送 BlockStart
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeBlockStart {
			bsData, ok := e.Delta.Data.(provider.BlockStartData)
			if ok && bsData.BlockType == "tool_use" {
				t.Error("BlockStart('tool_use') should not be emitted for FunctionCall (StreamConverter handles lifecycle)")
			}
		}
	}
}

// TestChat_FinishReason 验证 Gemini 流式响应的 FinishReason 透传。
//
// Mock server 返回 finishReason=MAX_TOKENS，
// 期望 Stop 事件携带 FinishReasonLength。
func TestChat_FinishReason(t *testing.T) {
	frame := `{"candidates":[{"content":{"parts":[{"text":"partial..."}],"role":"model"},"finishReason":"MAX_TOKENS"}]}`
	server := mockGeminiServer(t, 200, []string{frame})
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "gemini-2.5-flash"}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Tell me a long story"}}

	ch := p.Chat(ctx, messages, config)
	events := collectEvents(ch)

	// 查找 Stop 事件
	stopIdx := findEventType(events, provider.StreamTypeStop)
	if stopIdx < 0 {
		t.Fatal("expected StreamTypeStop event")
	}
	stopEvent := events[stopIdx]
	if stopEvent.FinishReason != provider.FinishReasonLength {
		t.Errorf("FinishReason = %q, want %q (MAX_TOKENS → Length)", stopEvent.FinishReason, provider.FinishReasonLength)
	}
}

// TestChat_ErrorResponse 验证 HTTP 400 错误响应处理。
//
// Mock server 返回 HTTP 400 + Gemini 错误 JSON，
// 期望收到 Error 事件且错误消息包含 "test error"。
func TestChat_ErrorResponse(t *testing.T) {
	server := mockGeminiServer(t, 400, nil)
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "gemini-2.5-flash"}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Hi"}}

	ch := p.Chat(ctx, messages, config)
	events := collectEvents(ch)

	// 验证 Error 事件
	errIdx := findEventType(events, provider.StreamTypeError)
	if errIdx < 0 {
		t.Fatal("expected StreamTypeError event")
	}
	errEvent := events[errIdx]
	if errEvent.Err == nil {
		t.Fatal("error event has nil Err")
	}
	errMsg := errEvent.Err.Error()
	if !contains(errMsg, "test error") {
		t.Errorf("error message = %q, want it to contain 'test error'", errMsg)
	}

	// 验证没有 Start 事件（错误在 Start 之前发生）
	if findEventType(events, provider.StreamTypeStart) >= 0 {
		t.Error("should not have StreamTypeStart event on HTTP error")
	}
}

// TestChat_MultiFrameTextAndThinking 验证多帧混合内容（thinking + text）的流式输出。
//
// Mock server 返回两帧：第一帧为 thinking，第二帧为 text + finishReason=STOP。
// 期望 BlockStart("thinking") → ThinkingDelta → BlockStart("text") → TextDelta → Stop → Done。
func TestChat_MultiFrameTextAndThinking(t *testing.T) {
	frame1 := `{"candidates":[{"content":{"parts":[{"text":"Reasoning step 1","thought":true}],"role":"model"}}]}`
	frame2 := `{"candidates":[{"content":{"parts":[{"text":"Final answer"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":10,"totalTokenCount":15}}`
	server := mockGeminiServer(t, 200, []string{frame1, frame2})
	defer server.Close()

	p := newTestProvider(server)
	ctx := context.Background()
	config := &provider.ChatConfig{Model: "gemini-2.5-flash"}
	messages := []provider.Message{{Role: provider.RoleUser, Content: "Think and answer"}}

	ch := p.Chat(ctx, messages, config)
	events := collectEvents(ch)

	// 统计 BlockStart 事件
	blockStarts := 0
	for _, e := range events {
		if e.Delta.Type == provider.StreamDeltaTypeBlockStart {
			blockStarts++
		}
	}
	if blockStarts != 2 {
		t.Errorf("expected 2 BlockStart events (thinking + text), got %d", blockStarts)
	}

	// 验证 ThinkingDelta
	td := findDeltaType(events, provider.StreamDeltaTypeThinking)
	if td == nil {
		t.Error("expected ThinkingDelta")
	}

	// 验证 TextDelta
	tx := findDeltaType(events, provider.StreamDeltaTypeTextOutput)
	if tx == nil {
		t.Error("expected TextDelta")
	}
	textData, _ := tx.Delta.Data.(provider.TextData)
	if string(textData) != "Final answer" {
		t.Errorf("text = %q, want %q", string(textData), "Final answer")
	}

	// 验证 Done
	if findEventType(events, provider.StreamTypeDone) < 0 {
		t.Error("expected StreamTypeDone event")
	}
}

// contains 检查字符串 s 是否包含 substr。
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

// indexOf 返回 substr 在 s 中的起始索引，未找到返回 -1。
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

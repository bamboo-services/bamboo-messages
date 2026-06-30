package completions

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// sseFrame 构造一个 SSE data 帧。
func sseFrame(data string) string {
	return fmt.Sprintf("data: %s\n\n", data)
}

// newMockProvider 创建指向 httptest.Server 的 CompletionsProvider。
func newMockProvider(t *testing.T, server *httptest.Server) *CompletionsProvider {
	t.Helper()
	return NewCompletionsProviderWithOptions(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)
}

// collectEvents 从 channel 收集所有事件直到关闭。
func collectEvents(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// TestChat_NormalStream 验证正常流式对话：3 个 SSE 帧（文本增量 + finish_reason + [DONE]）。
func TestChat_NormalStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseFrame(`{"id":"1","choices":[{"index":0,"delta":{"content":"Hello"}}]}`))
		fmt.Fprint(w, sseFrame(`{"id":"2","choices":[{"index":0,"delta":{"content":" world"}}]}`))
		fmt.Fprint(w, sseFrame(`{"id":"3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`))
		fmt.Fprint(w, sseFrame(`[DONE]`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := &provider.ChatConfig{Model: "gpt-4o", MaxTokens: 100}
	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, config)

	events := collectEvents(ch)

	// 期望事件序列：Start, BlockStart(text), TextDelta("Hello"), TextDelta(" world"), Stop, Done
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(events))
	}

	// 验证 Start 事件
	if events[0].Type != provider.StreamTypeStart {
		t.Errorf("event[0] type = %v, want Start", events[0].Type)
	}

	// 验证包含文本增量
	var textContent string
	for _, e := range events {
		if e.Type == provider.StreamTypeDelta && e.Delta.Type == provider.StreamDeltaTypeTextOutput {
			textContent += string(e.Delta.Data.(provider.TextData))
		}
	}
	if textContent != "Hello world" {
		t.Errorf("text content = %q, want %q", textContent, "Hello world")
	}

	// 验证包含 Stop 事件
	foundStop := false
	for _, e := range events {
		if e.Type == provider.StreamTypeStop {
			foundStop = true
			if e.FinishReason != provider.FinishReasonStop {
				t.Errorf("FinishReason = %v, want %v", e.FinishReason, provider.FinishReasonStop)
			}
		}
	}
	if !foundStop {
		t.Error("missing Stop event")
	}

	// 验证最后一个事件是 Done
	if events[len(events)-1].Type != provider.StreamTypeDone {
		t.Errorf("last event type = %v, want Done", events[len(events)-1].Type)
	}
}

// TestChat_BrokenFrameSkip 验证截断的 JSON 帧被跳过，有效帧正常处理。
func TestChat_BrokenFrameSkip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 截断的 JSON 帧（SSEScanner 会跳过）
		fmt.Fprint(w, "data: {\"id\":\"1\",\"choices\":[{\"inde\n\n")
		// 正常帧
		fmt.Fprint(w, sseFrame(`{"id":"2","choices":[{"index":0,"delta":{"content":"OK"}}]}`))
		fmt.Fprint(w, sseFrame(`{"id":"3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`))
		fmt.Fprint(w, sseFrame(`[DONE]`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := &provider.ChatConfig{Model: "gpt-4o", MaxTokens: 100}
	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, config)

	events := collectEvents(ch)

	// 验证有效帧的文本被正确处理
	var textContent string
	for _, e := range events {
		if e.Type == provider.StreamTypeDelta && e.Delta.Type == provider.StreamDeltaTypeTextOutput {
			textContent += string(e.Delta.Data.(provider.TextData))
		}
	}
	if textContent != "OK" {
		t.Errorf("text content = %q, want %q (broken frame should be skipped)", textContent, "OK")
	}

	// 验证有 Done 事件
	foundDone := false
	for _, e := range events {
		if e.Type == provider.StreamTypeDone {
			foundDone = true
		}
	}
	if !foundDone {
		t.Error("missing Done event")
	}
}

// TestChat_ToolCallStream 验证工具调用增量帧产生 ToolCallDelta 事件。
func TestChat_ToolCallStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 工具调用开始帧
		fmt.Fprint(w, sseFrame(`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-abc","function":{"name":"get_weather"}}]}}]}`))
		// 参数增量帧
		fmt.Fprint(w, sseFrame(`{"id":"2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc"}}]}}]}`))
		fmt.Fprint(w, sseFrame(`{"id":"3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation\":\"Tokyo\"}"}}]}}]}`))
		// finish_reason
		fmt.Fprint(w, sseFrame(`{"id":"4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`))
		fmt.Fprint(w, sseFrame(`[DONE]`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := &provider.ChatConfig{Model: "gpt-4o", MaxTokens: 100}
	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "What's the weather?"},
	}, config)

	events := collectEvents(ch)

	// 验证包含 ToolCallDelta（开始）事件
	foundToolCallStart := false
	foundToolCallDelta := false
	var argsContent string
	for _, e := range events {
		if e.Type == provider.StreamTypeDelta && e.Delta.Type == provider.StreamDeltaTypeToolCall {
			foundToolCallStart = true
		}
		if e.Type == provider.StreamTypeDelta && e.Delta.Type == provider.StreamDeltaTypeToolCallDelta {
			foundToolCallDelta = true
			// 当 Index 存在时，Data 类型为 IndexedToolCallDeltaData
			if idx, ok := e.Delta.Data.(provider.IndexedToolCallDeltaData); ok {
				argsContent += idx.PartialJSON
			} else if d, ok := e.Delta.Data.(provider.ToolCallDeltaData); ok {
				argsContent += string(d)
			}
		}
	}
	if !foundToolCallStart {
		t.Error("missing ToolCallDelta (start) event")
	}
	if !foundToolCallDelta {
		t.Error("missing ToolCallDeltaData event")
	}
	if argsContent != `{"location":"Tokyo"}` {
		t.Errorf("tool call arguments = %q, want %q", argsContent, `{"location":"Tokyo"}`)
	}

	// 验证 Stop 事件的 FinishReason 为 tool_calls
	for _, e := range events {
		if e.Type == provider.StreamTypeStop {
			if e.FinishReason != provider.FinishReasonToolCalls {
				t.Errorf("FinishReason = %v, want %v", e.FinishReason, provider.FinishReasonToolCalls)
			}
		}
	}
}

// TestChat_ErrorResponse 验证 HTTP 429 错误响应产生 Error 事件。
func TestChat_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error","code":"429"}}`)
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := &provider.ChatConfig{Model: "gpt-4o", MaxTokens: 100}
	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, config)

	events := collectEvents(ch)

	// 期望至少有一个 Error 事件
	foundError := false
	for _, e := range events {
		if e.Type == provider.StreamTypeError {
			foundError = true
			if e.Err == nil {
				t.Error("Error event should have non-nil Err")
			}
		}
	}
	if !foundError {
		t.Fatal("missing Error event for HTTP 429 response")
	}
}

// TestChat_UsageInLastChunk 验证最后一个 chunk 中的 usage 被正确提取为 UsageDelta 事件。
func TestChat_UsageInLastChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 文本增量
		fmt.Fprint(w, sseFrame(`{"id":"1","choices":[{"index":0,"delta":{"content":"Hi"}}]}`))
		// finish_reason + usage（最后一个 chunk）
		fmt.Fprint(w, sseFrame(`{"id":"2","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
		fmt.Fprint(w, sseFrame(`[DONE]`))
	}))
	defer server.Close()

	p := newMockProvider(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := &provider.ChatConfig{Model: "gpt-4o", MaxTokens: 100}
	ch := p.Chat(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: "Hi"},
	}, config)

	events := collectEvents(ch)

	// 验证包含 UsageDelta 事件
	foundUsage := false
	for _, e := range events {
		if e.Type == provider.StreamTypeDelta && e.Delta.Type == provider.StreamDeltaTypeUsage {
			foundUsage = true
			data, ok := e.Delta.Data.(provider.UsageData)
			if !ok {
				t.Fatal("UsageDelta Data is not UsageData")
			}
			if data.InputTokens != 10 {
				t.Errorf("UsageData.InputTokens = %d, want 10", data.InputTokens)
			}
			if data.OutputTokens != 5 {
				t.Errorf("UsageData.OutputTokens = %d, want 5", data.OutputTokens)
			}
		}
	}
	if !foundUsage {
		t.Fatal("missing UsageDelta event in last chunk")
	}
}

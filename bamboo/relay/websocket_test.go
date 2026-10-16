package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fmt"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/gemini"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/responses"
	"github.com/bamboo-services/bamboo-messages/pkg/ws"
	"github.com/bamboo-services/bamboo-messages/provider"
	geminiprovider "github.com/bamboo-services/bamboo-messages/provider/gemini"
	responsesprovider "github.com/bamboo-services/bamboo-messages/provider/openai/responses"
)

// TestUpstreamResponsesWebSocketProvider 验证 provider/openai/responses 的 WebSocket 客户端模式（wss:// / WithWebSocketMode）：
// 1. 发起 WebSocket 握手并发送 {"type":"response.create", ...}（不含 stream 字段）；
// 2. 接收服务端推送的 response.created / response.output_text.delta / response.completed 事件；
// 3. 支持 Chat 流式与 Complete 非流式聚合。
func TestUpstreamResponsesWebSocketProvider(t *testing.T) {
	var receivedType string
	var hasStreamField bool

	upServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer ws-test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := ws.Upgrade(w, r)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("ReadMessage error: %v", err)
			return
		}
		var reqMap map[string]any
		if err := json.Unmarshal(payload, &reqMap); err != nil {
			t.Errorf("unmarshal request: %v", err)
			return
		}
		receivedType, _ = reqMap["type"].(string)
		_, hasStreamField = reqMap["stream"]

		frames := []string{
			`{"type":"response.created","response":{"id":"resp_ws_001","status":"in_progress","model":"gpt-4o"}}`,
			`{"type":"response.output_text.delta","delta":"Hello from "}`,
			`{"type":"response.output_text.delta","delta":"Upstream WS!"}`,
			`{"type":"response.completed","response":{"id":"resp_ws_001","status":"completed","usage":{"input_tokens":12,"output_tokens":6}}}`,
		}
		for _, f := range frames {
			if err := conn.WriteText([]byte(f)); err != nil {
				t.Errorf("WriteText error: %v", err)
				return
			}
		}
	}))
	defer upServer.Close()

	wsBaseURL := "ws" + strings.TrimPrefix(upServer.URL, "http") + "/v1"
	p := responsesprovider.NewResponsesProviderWithOptions(
		responsesprovider.WithAPIKey("ws-test-key"),
		responsesprovider.WithBaseURL(wsBaseURL),
		responsesprovider.WithWebSocketMode(true),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := p.Complete(ctx, []provider.Message{{Role: provider.RoleUser, Content: "ping"}}, &provider.ChatConfig{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("Complete via WebSocket failed: %v", err)
	}
	if receivedType != "response.create" {
		t.Errorf("client frame type = %q, want response.create", receivedType)
	}
	if hasStreamField {
		t.Error("WebSocket response.create frame must omit 'stream' field")
	}
	if res.ResponseID != "resp_ws_001" {
		t.Errorf("ResponseID = %q, want resp_ws_001", res.ResponseID)
	}
	if res.Content != "Hello from Upstream WS!" {
		t.Errorf("Content = %q, want 'Hello from Upstream WS!'", res.Content)
	}
	if res.Usage.InputTokens != 12 || res.Usage.OutputTokens != 6 {
		t.Errorf("Usage = %+v, want 12/6", res.Usage)
	}
}

// TestDownstreamResponsesWebSocket_MultiTurnToolLoopAcrossStatelessProvider 验证下游使用
// OpenAI Responses WebSocket 协议（含 stream_id 与 previous_response_id 增量续接），
// 桥接到无状态上游 Provider 时：
// 1. 第 1 轮返回 function_call 并记录会话快照；
// 2. 第 2 轮仅发送 previous_response_id + function_call_output，桥接器自动拼接完整历史发给上游；
// 3. 每个出站 WebSocket 事件均携带对应的 stream_id；
// 4. 若发送未知的 previous_response_id，返回标准 previous_response_not_found 错误事件。
func TestDownstreamResponsesWebSocket_MultiTurnToolLoopAcrossStatelessProvider(t *testing.T) {
	var round atomic.Int32
	var secondRoundMessages []provider.Message

	mockUpstream := &dynamicMockProvider{
		chatFn: func(_ context.Context, messages []provider.Message, _ *provider.ChatConfig) []provider.StreamEvent {
			r := round.Add(1)
			if r == 1 {
				return []provider.StreamEvent{
					{Type: provider.StreamTypeStart},
					{Type: provider.StreamTypeDelta, Delta: provider.NewToolCallDelta("call_weather_1", "get_weather")},
					{Type: provider.StreamTypeDelta, Delta: provider.NewToolCallDeltaData(`{"city":"Tokyo"}`)},
					{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonToolCalls},
					{Type: provider.StreamTypeDone},
				}
			}
			secondRoundMessages = append([]provider.Message(nil), messages...)
			return []provider.StreamEvent{
				{Type: provider.StreamTypeStart},
				{Type: provider.StreamTypeDelta, Delta: provider.NewBlockStartDelta("text")},
				{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("Tokyo is sunny, 25C.")},
				{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonStop},
				{Type: provider.StreamTypeDone},
			}
		},
	}

	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = RelayResponsesWebSocket(r.Context(), w, r, mockUpstream)
	}))
	defer bridgeServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(bridgeServer.URL, "http")
	clientConn, err := ws.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws.Dial failed: %v", err)
	}
	defer clientConn.Close()

	// ── 第 1 轮：发送初始用户请求 + 工具定义 ──
	turn1Req := `{
		"type": "response.create",
		"stream_id": "stream-alpha",
		"model": "claude-sonnet-4-20250514",
		"input": [{"role": "user", "content": "What is the weather in Tokyo?"}],
		"tools": [{"type": "function", "name": "get_weather", "parameters": {"type": "object"}}]
	}`
	if err := clientConn.WriteText([]byte(turn1Req)); err != nil {
		t.Fatalf("WriteText turn 1 failed: %v", err)
	}

	var turn1RespID string
	var sawFunctionCall bool
	for {
		_, data, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage turn 1 failed: %v", err)
		}
		var ev map[string]any
		if err := json.Unmarshal(data, &ev); err != nil {
			t.Fatalf("invalid JSON from WS bridge: %v (%s)", err, data)
		}
		if ev["stream_id"] != "stream-alpha" {
			t.Errorf("stream_id = %v, want stream-alpha", ev["stream_id"])
		}
		evType, _ := ev["type"].(string)
		if evType == "response.output_item.done" {
			if item, ok := ev["item"].(map[string]any); ok && item["type"] == "function_call" {
				sawFunctionCall = true
			}
		}
		if evType == "response.completed" {
			if respObj, ok := ev["response"].(map[string]any); ok {
				turn1RespID, _ = respObj["id"].(string)
			}
			break
		}
	}

	if !sawFunctionCall || turn1RespID == "" {
		t.Fatalf("turn 1 did not complete with function_call and response_id (sawFC=%v, id=%q)", sawFunctionCall, turn1RespID)
	}

	// ── 第 2 轮：仅发送 previous_response_id + function_call_output ──
	turn2Req := `{
		"type": "response.create",
		"stream_id": "stream-alpha",
		"model": "claude-sonnet-4-20250514",
		"previous_response_id": "` + turn1RespID + `",
		"input": [{"type": "function_call_output", "call_id": "call_weather_1", "output": "Sunny 25C"}]
	}`
	if err := clientConn.WriteText([]byte(turn2Req)); err != nil {
		t.Fatalf("WriteText turn 2 failed: %v", err)
	}

	var finalText strings.Builder
	for {
		_, data, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage turn 2 failed: %v", err)
		}
		var ev map[string]any
		if err := json.Unmarshal(data, &ev); err != nil {
			t.Fatalf("invalid JSON in turn 2: %v", err)
		}
		evType, _ := ev["type"].(string)
		if evType == "response.output_text.delta" {
			if delta, ok := ev["delta"].(string); ok {
				finalText.WriteString(delta)
			}
		}
		if evType == "response.completed" {
			break
		}
	}

	if finalText.String() != "Tokyo is sunny, 25C." {
		t.Errorf("turn 2 finalText = %q, want 'Tokyo is sunny, 25C.'", finalText.String())
	}

	// 验证无状态上游在第 2 轮收到了拼接后的完整 3 条消息：user + assistant(tool_calls) + tool(result)
	if len(secondRoundMessages) != 3 {
		t.Fatalf("secondRoundMessages len = %d, want 3 (user + assistant + tool); got %+v", len(secondRoundMessages), secondRoundMessages)
	}
	if secondRoundMessages[0].Role != provider.RoleUser || secondRoundMessages[1].Role != provider.RoleAssistant || secondRoundMessages[2].Role != provider.RoleTool {
		t.Errorf("unexpected roles in reconstructed history: %+v", secondRoundMessages)
	}

	// ── 第 3 轮：测试未知的 previous_response_id 返回 previous_response_not_found ──
	badReq := `{
		"type": "response.create",
		"stream_id": "stream-alpha",
		"model": "claude-sonnet-4-20250514",
		"previous_response_id": "resp_non_existent",
		"input": "hello"
	}`
	if err := clientConn.WriteText([]byte(badReq)); err != nil {
		t.Fatalf("WriteText badReq failed: %v", err)
	}
	_, errFrame, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage badReq failed: %v", err)
	}
	var errEv map[string]any
	_ = json.Unmarshal(errFrame, &errEv)
	if errEv["type"] != "error" {
		t.Fatalf("expected error event for unknown previous_response_id, got: %s", errFrame)
	}
	errBody, _ := errEv["error"].(map[string]any)
	if errBody["code"] != "previous_response_not_found" {
		t.Errorf("error.code = %v, want previous_response_not_found", errBody["code"])
	}
}

type dynamicMockProvider struct {
	chatFn func(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) []provider.StreamEvent
}

func (d *dynamicMockProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return d.ChatWithSystem(ctx, "", messages, config)
}

func (d *dynamicMockProvider) ChatWithSystem(ctx context.Context, _ string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	events := d.chatFn(ctx, messages, config)
	ch := make(chan provider.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

func (d *dynamicMockProvider) Complete(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{Content: "ok", FinishReason: provider.FinishReasonStop}, nil
}

func (d *dynamicMockProvider) CompleteWithSystem(ctx context.Context, _ string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return d.Complete(ctx, messages, config)
}

func (d *dynamicMockProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

func (d *dynamicMockProvider) GetAvailableModels() []string {
	return []string{"claude-sonnet-4-20250514"}
}

// TestCrossProtocol_AllFormatsToUpstreamResponsesWebSocket 验证任意下游协议（Anthropic / Gemini / OpenAI / Responses）
// 通过 Relay / RelayStream 调用启用了 WebSocket 模式的 ResponsesProvider 上游时，
// 能够完成端到端跨协议请求转换、WebSocket 帧收发与目标协议响应序列化。
func TestCrossProtocol_AllFormatsToUpstreamResponsesWebSocket(t *testing.T) {
	upServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Upgrade(w, r)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req map[string]any
		_ = json.Unmarshal(payload, &req)
		if req["type"] != "response.create" {
			t.Errorf("expected response.create, got %v", req["type"])
		}

		frames := []string{
			`{"type":"response.created","response":{"id":"resp_cross_1","status":"in_progress","model":"gpt-4o"}}`,
			`{"type":"response.output_text.delta","delta":"Cross-protocol "}`,
			`{"type":"response.output_text.delta","delta":"WS OK!"}`,
			`{"type":"response.completed","response":{"id":"resp_cross_1","status":"completed","usage":{"input_tokens":8,"output_tokens":4}}}`,
		}
		for _, f := range frames {
			_ = conn.WriteText([]byte(f))
		}
	}))
	defer upServer.Close()

	wsBaseURL := "ws" + strings.TrimPrefix(upServer.URL, "http") + "/v1"
	wsProvider := responsesprovider.NewResponsesProviderWithOptions(
		responsesprovider.WithAPIKey("test-key"),
		responsesprovider.WithBaseURL(wsBaseURL),
		responsesprovider.WithWebSocketMode(true),
	)

	cases := []struct {
		name      string
		inFormat  codec.FormatType
		outFormat codec.FormatType
		body      []byte
	}{
		{"Anthropic_to_Anthropic_via_WS", codec.FormatAnthropic, codec.FormatAnthropic, anthropicBody},
		{"Gemini_to_Gemini_via_WS", codec.FormatGemini, codec.FormatGemini, geminiBody},
		{"OpenAI_to_OpenAI_via_WS", codec.FormatOpenAI, codec.FormatOpenAI, openaiBody},
		{"Anthropic_to_OpenAI_via_WS", codec.FormatAnthropic, codec.FormatOpenAI, anthropicBody},
		{"Gemini_to_Anthropic_via_WS", codec.FormatGemini, codec.FormatAnthropic, geminiBody},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_NonStream", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			out, err := Relay(ctx, wsProvider, tc.body, tc.inFormat, tc.outFormat)
			if err != nil {
				t.Fatalf("Relay(%s->%s) via WS failed: %v", tc.inFormat, tc.outFormat, err)
			}
			if !strings.Contains(string(out), "Cross-protocol WS OK!") {
				t.Errorf("unexpected response body: %s", out)
			}
		})

		t.Run(tc.name+"_Stream", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ch, err := RelayStream(ctx, wsProvider, tc.body, tc.inFormat, tc.outFormat)
			if err != nil {
				t.Fatalf("RelayStream(%s->%s) via WS failed: %v", tc.inFormat, tc.outFormat, err)
			}
			var sb strings.Builder
			for frame := range ch {
				sb.Write(frame)
			}
			if !strings.Contains(sb.String(), "Cross-protocol") || !strings.Contains(sb.String(), "WS OK!") {
				t.Errorf("unexpected stream output: %s", sb.String())
			}
		})
	}
}

// TestDownstreamResponsesWebSocket_ToUpstreamGemini3WithThoughtSignature 验证：
// 下游使用 OpenAI Responses WebSocket 协议（previous_response_id 增量多轮工具调用），
// 上游对接真实 Gemini HTTP/SSE Provider（模擬 Gemini 3 返回带 thoughtSignature 的并行 functionCall）：
// 第 2 轮客户端仅发送 previous_response_id + 两个 function_call_output，
// 桥接器不仅还原了两个并行 functionCall 并合并了单条 user functionResponse，
// 还把第 1 轮 Gemini 原生的 thoughtSignature 原封不动挂回第一个 functionCall part 上！
func TestDownstreamResponsesWebSocket_ToUpstreamGemini3WithThoughtSignature(t *testing.T) {
	var geminiRound atomic.Int32
	var round2GeminiBody map[string]any

	geminiUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := geminiRound.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			// 第 1 轮：Gemini 3 返回带 thoughtSignature 的两个并行同名工具调用
			chunk := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"gcall_1","name":"read_file","args":{"path":"a.go"}},"thoughtSignature":"gemini3_encrypted_sig_xyz"},{"functionCall":{"id":"gcall_2","name":"read_file","args":{"path":"b.go"}}}]},"finishReason":"STOP"}]}`
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			return
		}
		// 第 2 轮：捕获发往 Gemini 上游的真实请求 JSON
		_ = json.NewDecoder(r.Body).Decode(&round2GeminiBody)
		chunk := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Both files analyzed!"}]},"finishReason":"STOP"}]}`
		_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
	}))
	defer geminiUpstream.Close()

	geminiProv := geminiprovider.NewProviderWithOptions(
		geminiprovider.WithAPIKey("gemini-key"),
		geminiprovider.WithBaseURL(geminiUpstream.URL),
	)

	wsBridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = RelayResponsesWebSocket(r.Context(), w, r, geminiProv)
	}))
	defer wsBridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsConn, err := ws.Dial(ctx, "ws"+strings.TrimPrefix(wsBridge.URL, "http"), nil)
	if err != nil {
		t.Fatalf("ws.Dial failed: %v", err)
	}
	defer wsConn.Close()

	// 第 1 轮：下游 WS 发送 response.create
	if err := wsConn.WriteText([]byte(`{
		"type": "response.create",
		"stream_id": "gemini-stream",
		"model": "gemini-3-pro",
		"input": "Read a.go and b.go"
	}`)); err != nil {
		t.Fatal(err)
	}

	var respID string
	for {
		_, raw, err := wsConn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var ev map[string]any
		_ = json.Unmarshal(raw, &ev)
		if ev["type"] == "response.completed" {
			respObj, _ := ev["response"].(map[string]any)
			respID, _ = respObj["id"].(string)
			break
		}
	}
	if respID == "" {
		t.Fatal("missing response_id from turn 1")
	}

	// 第 2 轮：下游 WS 仅发送 previous_response_id + 两个工具结果
	if err := wsConn.WriteText([]byte(`{
		"type": "response.create",
		"stream_id": "gemini-stream",
		"model": "gemini-3-pro",
		"previous_response_id": "` + respID + `",
		"input": [
			{"type": "function_call_output", "call_id": "gcall_1", "output": "package a"},
			{"type": "function_call_output", "call_id": "gcall_2", "output": "package b"}
		]
	}`)); err != nil {
		t.Fatal(err)
	}

	var gotText strings.Builder
	for {
		_, raw, err := wsConn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var ev map[string]any
		_ = json.Unmarshal(raw, &ev)
		if ev["type"] == "response.output_text.delta" {
			if d, ok := ev["delta"].(string); ok {
				gotText.WriteString(d)
			}
		}
		if ev["type"] == "response.completed" {
			break
		}
	}
	if gotText.String() != "Both files analyzed!" {
		t.Fatalf("gotText = %q, want 'Both files analyzed!'", gotText.String())
	}

	// 验证发往 Gemini 3 上游的第 2 轮请求体：
	// contents[0] = user ("Read a.go and b.go")
	// contents[1] = model (2 个 functionCall，且第一个必须携带 thoughtSignature="gemini3_encrypted_sig_xyz"！)
	// contents[2] = user (合并后的 2 个 functionResponse)
	contents, ok := round2GeminiBody["contents"].([]any)
	if !ok || len(contents) != 3 {
		t.Fatalf("round2 Gemini contents = %#v, want 3 turns", round2GeminiBody["contents"])
	}
	modelTurn, _ := contents[1].(map[string]any)
	modelParts, _ := modelTurn["parts"].([]any)
	if len(modelParts) != 2 {
		t.Fatalf("modelParts len = %d, want 2", len(modelParts))
	}
	firstPart, _ := modelParts[0].(map[string]any)
	if firstPart["thoughtSignature"] != "gemini3_encrypted_sig_xyz" {
		t.Errorf("first functionCall thoughtSignature = %v, want gemini3_encrypted_sig_xyz", firstPart["thoughtSignature"])
	}
	secondPart, _ := modelParts[1].(map[string]any)
	if _, hasSig := secondPart["thoughtSignature"]; hasSig {
		t.Errorf("second parallel functionCall must not have thoughtSignature, got %v", secondPart["thoughtSignature"])
	}
}

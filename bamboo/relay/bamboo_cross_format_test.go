package relay

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/bamboo"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/gemini"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/responses"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ════════════════════════════════════════════════════════════
// bamboo 原生格式 N2N relay 矩阵测试
// ════════════════════════════════════════════════════════════
//
// 验证 bamboo ↔ {anthropic, openai, responses, gemini} 协议互转
// 在 Relay / RelayStream 端到端管道中的正确性。
// 复用 relay_test.go 中的 mockProvider 基础设施。

// ── bamboo 原生请求信封 JSON ──

var (
	// bambooBody bamboo 原生非流式请求信封
	bambooBody = []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"system":"you are helpful","config":{"model":"test-model","max_tokens":1024},"stream":false}`)

	// bambooStreamBody bamboo 原生流式请求信封
	bambooStreamBody = []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"system":"you are helpful","config":{"model":"test-model","max_tokens":1024},"stream":true}`)
)

// newMockRichProvider 创建返回 text + thinking + tool_calls 的 mock provider。
func newMockRichProvider() *mockProvider {
	return &mockProvider{
		completeResult: &provider.CompletionResult{
			Content:  "text response",
			Thinking: "thinking content",
			ToolCalls: []provider.ToolCall{
				{
					ID:   "call-123",
					Type: "function",
					Function: provider.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"SF"}`,
					},
				},
			},
			FinishReason: provider.FinishReasonToolCalls,
			Usage: provider.UsageData{
				InputTokens:  10,
				OutputTokens: 20,
			},
		},
	}
}

// newMockCacheProvider 创建返回带缓存统计的 CompletionResult 的 mock provider。
func newMockCacheProvider() *mockProvider {
	return &mockProvider{
		completeResult: &provider.CompletionResult{
			Content:      "cached response",
			FinishReason: provider.FinishReasonStop,
			Usage: provider.UsageData{
				InputTokens:              100,
				OutputTokens:             50,
				CacheCreationInputTokens: 30,
				CacheReadInputTokens:     70,
			},
		},
	}
}

// newMockErrorProvider 创建返回 BambooError 的 mock provider。
func newMockErrorProvider() *mockProvider {
	return &mockProvider{
		completeErr: pkgErrors.NewBambooError("上游", "provider boom", 500),
	}
}

// ════════════════════════════════════════════════════════════
// 非流式 Relay 测试
// ════════════════════════════════════════════════════════════

// TestBambooRelay_ToAnthropic bamboo → anthropic 非流式互转。
//
// 验证 bamboo 信封请求经 mock provider 返回 text+thinking+tool_use 后，
// 序列化为合法的 Anthropic Messages 响应 JSON。
func TestBambooRelay_ToAnthropic(t *testing.T) {
	mp := newMockRichProvider()

	out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("Relay(bamboo→anthropic) error: %v", err)
	}

	outStr := string(out)
	// Anthropic 格式应包含 "type":"message"
	if !strings.Contains(outStr, `"type":"message"`) {
		t.Errorf("expected anthropic message type, got: %s", outStr)
	}
	// 响应文本应出现
	if !strings.Contains(outStr, "text response") {
		t.Errorf("expected response text in output, got: %s", outStr)
	}
	// thinking 内容应出现
	if !strings.Contains(outStr, "thinking content") {
		t.Errorf("expected thinking content in output, got: %s", outStr)
	}
	// 工具调用应出现
	if !strings.Contains(outStr, "get_weather") {
		t.Errorf("expected tool call 'get_weather' in output, got: %s", outStr)
	}
	// 验证 JSON 可解析
	var parsed map[string]any
	if jErr := json.Unmarshal(out, &parsed); jErr != nil {
		t.Errorf("output is not valid JSON: %v", jErr)
	}
}

// TestBambooRelay_ToOpenAI bamboo → openai 非流式互转。
func TestBambooRelay_ToOpenAI(t *testing.T) {
	mp := newMockRichProvider()

	out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatOpenAI)
	if err != nil {
		t.Fatalf("Relay(bamboo→openai) error: %v", err)
	}

	outStr := string(out)
	if !strings.Contains(outStr, `"object":"chat.completion"`) {
		t.Errorf("expected chat.completion object, got: %s", outStr)
	}
	if !strings.Contains(outStr, "text response") {
		t.Errorf("expected response text, got: %s", outStr)
	}
	// 验证 JSON 可解析
	var parsed map[string]any
	if jErr := json.Unmarshal(out, &parsed); jErr != nil {
		t.Errorf("output is not valid JSON: %v", jErr)
	}
}

// TestBambooRelay_ToResponses bamboo → responses 非流式互转。
func TestBambooRelay_ToResponses(t *testing.T) {
	mp := newMockRichProvider()

	out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatResponses)
	if err != nil {
		t.Fatalf("Relay(bamboo→responses) error: %v", err)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "text response") {
		t.Errorf("expected response text, got: %s", outStr)
	}
	// 验证 JSON 可解析
	var parsed map[string]any
	if jErr := json.Unmarshal(out, &parsed); jErr != nil {
		t.Errorf("output is not valid JSON: %v", jErr)
	}
}

// TestBambooRelay_ToGemini bamboo → gemini 非流式互转。
func TestBambooRelay_ToGemini(t *testing.T) {
	mp := newMockRichProvider()

	out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatGemini)
	if err != nil {
		t.Fatalf("Relay(bamboo→gemini) error: %v", err)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "text response") {
		t.Errorf("expected response text, got: %s", outStr)
	}
	// 验证 JSON 可解析
	var parsed map[string]any
	if jErr := json.Unmarshal(out, &parsed); jErr != nil {
		t.Errorf("output is not valid JSON: %v", jErr)
	}
}

// TestBambooRelay_FromAnthropic anthropic → bamboo 非流式互转。
//
// 验证 anthropic 请求经 mock provider 后序列化为合法的 bamboo 原生响应 JSON。
func TestBambooRelay_FromAnthropic(t *testing.T) {
	mp := newMockCompleteProvider("bamboo output", 10, 20)

	out, err := Relay(context.Background(), mp, anthropicBody, codec.FormatAnthropic, codec.FormatBamboo)
	if err != nil {
		t.Fatalf("Relay(anthropic→bamboo) error: %v", err)
	}

	// 解析为 bamboo.Response 验证字段
	var resp bamboo.Response
	if jErr := json.Unmarshal(out, &resp); jErr != nil {
		t.Fatalf("output is not valid bamboo Response JSON: %v\noutput: %s", jErr, string(out))
	}

	if resp.Type != "message" {
		t.Errorf("expected type='message', got %q", resp.Type)
	}
	if resp.Role != bamboo.RoleAssistant {
		t.Errorf("expected role='assistant', got %q", resp.Role)
	}
	if resp.StopReason != bamboo.FinishReasonEndTurn {
		t.Errorf("expected stop_reason='end_turn', got %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens=10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("expected output_tokens=20, got %d", resp.Usage.OutputTokens)
	}
	// 验证内容块存在
	if len(resp.Content) == 0 {
		t.Error("expected non-empty content blocks")
	}
	// 验证文本内容
	if len(resp.Content) > 0 {
		if tb, ok := resp.Content[0].(*bamboo.TextBlock); ok {
			if tb.Text != "bamboo output" {
				t.Errorf("expected text='bamboo output', got %q", tb.Text)
			}
		} else {
			t.Errorf("expected first content block to be *TextBlock, got %T", resp.Content[0])
		}
	}
}

// ════════════════════════════════════════════════════════════
// 流式 RelayStream 测试
// ════════════════════════════════════════════════════════════

// TestBambooRelayStream_ToAnthropic bamboo → anthropic 流式互转。
//
// 验证 bamboo 信封流式请求经 mock provider 产生标准事件序列后，
// 序列化为完整的 Anthropic SSE 帧序列（message_start / content_block_delta / message_stop）。
func TestBambooRelayStream_ToAnthropic(t *testing.T) {
	mp := newMockStreamProvider([]string{"Hello", " ", "World"}, 10, 20)

	ch, err := RelayStream(context.Background(), mp, bambooStreamBody, codec.FormatBamboo, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("RelayStream(bamboo→anthropic) error: %v", err)
	}

	var output strings.Builder
	frameCount := 0
	for data := range ch {
		if len(data) > 0 {
			frameCount++
		}
		output.Write(data)
	}

	if frameCount == 0 {
		t.Fatal("expected at least one data frame in stream")
	}

	result := output.String()
	// Anthropic 流应包含完整事件序列
	if !strings.Contains(result, "message_start") {
		t.Errorf("expected 'message_start' in stream, got: %s", result)
	}
	if !strings.Contains(result, "content_block_delta") {
		t.Errorf("expected 'content_block_delta' in stream, got: %s", result)
	}
	if !strings.Contains(result, "message_stop") {
		t.Errorf("expected 'message_stop' in stream, got: %s", result)
	}
	// 验证文本内容
	if !strings.Contains(result, "Hello") {
		t.Errorf("expected 'Hello' in stream text")
	}
	if !strings.Contains(result, "World") {
		t.Errorf("expected 'World' in stream text")
	}
}

// ════════════════════════════════════════════════════════════
// 错误路径测试
// ════════════════════════════════════════════════════════════

// TestBambooRelay_ErrorToAnthropic bamboo → anthropic 错误路径。
//
// mock provider 返回错误，Relay 应通过 outCodec.SerializeError
// 返回 anthropic 格式的错误 JSON。
func TestBambooRelay_ErrorToAnthropic(t *testing.T) {
	mp := newMockErrorProvider()

	out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatAnthropic)
	// Relay 在 provider 出错时返回 errorBody + error
	if err == nil {
		t.Fatal("expected error from provider failure")
	}

	outStr := string(out)
	// 应为 anthropic 错误格式
	if !strings.Contains(outStr, `"type":"error"`) {
		t.Errorf("expected anthropic error type, got: %s", outStr)
	}
	if !strings.Contains(outStr, "provider boom") {
		t.Errorf("expected error message in output, got: %s", outStr)
	}
	// 验证 JSON 可解析
	var parsed map[string]any
	if jErr := json.Unmarshal(out, &parsed); jErr != nil {
		t.Errorf("error output is not valid JSON: %v", jErr)
	}
}

// TestBambooRelay_ErrorFromAnthropic anthropic → bamboo 错误路径。
//
// mock provider 返回错误，Relay 应通过 bamboo codec.SerializeError
// 返回 bamboo 原生格式的错误 JSON: {"type":"error","error":{...}}。
func TestBambooRelay_ErrorFromAnthropic(t *testing.T) {
	mp := newMockErrorProvider()

	out, err := Relay(context.Background(), mp, anthropicBody, codec.FormatAnthropic, codec.FormatBamboo)
	if err == nil {
		t.Fatal("expected error from provider failure")
	}

	outStr := string(out)
	// 应为 bamboo 错误格式
	if !strings.Contains(outStr, `"type":"error"`) {
		t.Errorf("expected bamboo error type, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"category"`) {
		t.Errorf("expected 'category' field in bamboo error, got: %s", outStr)
	}
	if !strings.Contains(outStr, "provider boom") {
		t.Errorf("expected error message in output, got: %s", outStr)
	}
	// 验证 JSON 可解析
	var parsed map[string]any
	if jErr := json.Unmarshal(out, &parsed); jErr != nil {
		t.Errorf("error output is not valid JSON: %v", jErr)
	}
}

// ════════════════════════════════════════════════════════════
// Usage 缓存字段透传测试
// ════════════════════════════════════════════════════════════

// TestBambooRelay_CacheUsagePassthrough bamboo → anthropic 缓存字段透传。
//
// mock provider 返回带 CacheCreationInputTokens / CacheReadInputTokens 的 CompletionResult，
// 验证 anthropic 输出包含缓存统计字段。
func TestBambooRelay_CacheUsagePassthrough(t *testing.T) {
	mp := newMockCacheProvider()

	out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("Relay(bamboo→anthropic) error: %v", err)
	}

	outStr := string(out)
	// anthropic 应输出 cache_creation_input_tokens
	if !strings.Contains(outStr, "cache_creation_input_tokens") {
		t.Errorf("expected cache_creation_input_tokens in anthropic output, got: %s", outStr)
	}
	// anthropic 应输出 cache_read_input_tokens
	if !strings.Contains(outStr, "cache_read_input_tokens") {
		t.Errorf("expected cache_read_input_tokens in anthropic output, got: %s", outStr)
	}
	// 验证缓存值正确
	if !strings.Contains(outStr, `"cache_creation_input_tokens":30`) {
		t.Errorf("expected cache_creation_input_tokens=30, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"cache_read_input_tokens":70`) {
		t.Errorf("expected cache_read_input_tokens=70, got: %s", outStr)
	}
}

// TestBambooRelay_PassthroughIntegrity bamboo → bamboo 同格式透传完整性。
//
// 验证 bamboo 信封经 relay 层同格式入出时关键字段不丢失。
func TestBambooRelay_PassthroughIntegrity(t *testing.T) {
	mp := newMockCompleteProvider("passthrough test", 10, 20)

	out, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatBamboo)
	if err != nil {
		t.Fatalf("Relay(bamboo→bamboo) error: %v", err)
	}

	// 解析为 bamboo.Response 验证字段
	var resp bamboo.Response
	if jErr := json.Unmarshal(out, &resp); jErr != nil {
		t.Fatalf("output is not valid bamboo Response JSON: %v\noutput: %s", jErr, string(out))
	}

	if resp.Type != "message" {
		t.Errorf("expected type='message', got %q", resp.Type)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens=10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("expected output_tokens=20, got %d", resp.Usage.OutputTokens)
	}
	// 验证文本内容
	if len(resp.Content) == 0 {
		t.Fatal("expected non-empty content blocks")
	}
	if tb, ok := resp.Content[0].(*bamboo.TextBlock); ok {
		if tb.Text != "passthrough test" {
			t.Errorf("expected text='passthrough test', got %q", tb.Text)
		}
	} else {
		t.Errorf("expected first content block to be *TextBlock, got %T", resp.Content[0])
	}
}

// TestBambooRelay_SystemPromptPassthrough 验证 system prompt 从 bamboo 信封透传到 provider。
func TestBambooRelay_SystemPromptPassthrough(t *testing.T) {
	mp := newMockCompleteProvider("ok", 1, 1)

	_, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("Relay error: %v", err)
	}

	if mp.lastSystem != "you are helpful" {
		t.Errorf("expected system='you are helpful', got %q", mp.lastSystem)
	}
}

// TestBambooRelay_ConfigPassthrough 验证 config 字段从 bamboo 信封透传到 provider。
func TestBambooRelay_ConfigPassthrough(t *testing.T) {
	mp := newMockCompleteProvider("ok", 1, 1)

	_, err := Relay(context.Background(), mp, bambooBody, codec.FormatBamboo, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("Relay error: %v", err)
	}

	if mp.lastConfig == nil {
		t.Fatal("expected non-nil lastConfig")
	}
	if mp.lastConfig.Model != "test-model" {
		t.Errorf("expected model='test-model', got %q", mp.lastConfig.Model)
	}
	if mp.lastConfig.MaxTokens != 1024 {
		t.Errorf("expected max_tokens=1024, got %d", mp.lastConfig.MaxTokens)
	}
}

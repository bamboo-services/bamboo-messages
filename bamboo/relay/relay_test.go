package relay

import (
	"context"
	"errors"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// ════════════════════════════════════════════════════════════
// Mock Provider 实现
// ════════════════════════════════════════════════════════════

// mockProvider 用于测试的 Provider mock 实现。
type mockProvider struct {
	// completeResult Complete 方法返回的固定结果
	completeResult *provider.CompletionResult
	// completeErr Complete 方法返回的错误
	completeErr error
	// streamEvents Chat 方法发送的流事件序列
	streamEvents []provider.StreamEvent
	// lastMessages 最后一次调用接收到的消息（用于断言）
	lastMessages []provider.Message
	// lastSystem 最后一次调用接收到的系统提示词
	lastSystem string
	// lastConfig 最后一次调用接收到的配置
	lastConfig *provider.ChatConfig
}

func (m *mockProvider) Chat(_ context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	m.lastMessages = messages
	m.lastConfig = config
	ch := make(chan provider.StreamEvent, len(m.streamEvents))
	go func() {
		defer close(ch)
		for _, e := range m.streamEvents {
			ch <- e
		}
	}()
	return ch
}

func (m *mockProvider) ChatWithSystem(_ context.Context, system string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	m.lastSystem = system
	m.lastMessages = messages
	m.lastConfig = config
	ch := make(chan provider.StreamEvent, len(m.streamEvents))
	go func() {
		defer close(ch)
		for _, e := range m.streamEvents {
			ch <- e
		}
	}()
	return ch
}

func (m *mockProvider) Complete(_ context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	m.lastMessages = messages
	m.lastConfig = config
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	return m.completeResult, nil
}

func (m *mockProvider) CompleteWithSystem(_ context.Context, system string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	m.lastSystem = system
	m.lastMessages = messages
	m.lastConfig = config
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	return m.completeResult, nil
}

func (m *mockProvider) GetProviderType() provider.ProviderType {
	return "mock"
}

func (m *mockProvider) GetAvailableModels() []string {
	return []string{"mock-model"}
}

// 确保 mockProvider 实现 Provider 接口
var _ provider.Provider = (*mockProvider)(nil)

// ════════════════════════════════════════════════════════════
// 辅助函数
// ════════════════════════════════════════════════════════════

// newMockCompleteProvider 创建返回固定文本响应的 mock provider。
func newMockCompleteProvider(text string, input, output int64) *mockProvider {
	return &mockProvider{
		completeResult: &provider.CompletionResult{
			Content:      text,
			FinishReason: provider.FinishReasonStop,
			Usage: provider.UsageData{
				InputTokens:  input,
				OutputTokens: output,
			},
		},
	}
}

// openAIRequestBody 构建简单的 OpenAI 格式请求 JSON 字符串。
func openAIRequestBody(userContent string) string {
	return `{"model":"gpt-4","messages":[{"role":"user","content":"` + userContent + `"}]}`
}

// openAIRequestBodyWithSystem 构建带 system 消息的 OpenAI 格式请求 JSON。
func openAIRequestBodyWithSystem(system, userContent string) string {
	return `{"model":"gpt-4","messages":[{"role":"system","content":"` + system + `"},{"role":"user","content":"` + userContent + `"}]}`
}

// ════════════════════════════════════════════════════════════
// 测试用例
// ════════════════════════════════════════════════════════════

// TestRelay_OpenAI2Anthropic OpenAI 格式输入 → mock provider → Anthropic 格式输出。
func TestRelay_OpenAI2Anthropic(t *testing.T) {
	mp := newMockCompleteProvider("你好世界！", 10, 20)
	body := []byte(openAIRequestBody("hello"))

	out, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("Relay() error: %v", err)
	}

	outStr := string(out)
	// Anthropic 格式应包含 "type":"message" 和 content
	if !contains(outStr, `"type":"message"`) {
		t.Errorf("expected anthropic message type in output, got: %s", outStr)
	}
	if !contains(outStr, "你好世界") {
		t.Errorf("expected response text in output, got: %s", outStr)
	}
}

// TestRelay_Anthropic2OpenAI Anthropic 格式输入 → mock provider → OpenAI 格式输出。
func TestRelay_Anthropic2OpenAI(t *testing.T) {
	mp := newMockCompleteProvider("response text", 5, 15)
	body := []byte(`{"model":"claude-sonnet-4-20250514","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	out, err := Relay(context.Background(), mp, body, codec.FormatAnthropic, codec.FormatOpenAI)
	if err != nil {
		t.Fatalf("Relay() error: %v", err)
	}

	outStr := string(out)
	// OpenAI 格式应包含 chat.completion
	if !contains(outStr, `"object":"chat.completion"`) {
		t.Errorf("expected openai chat.completion in output, got: %s", outStr)
	}
	if !contains(outStr, "response text") {
		t.Errorf("expected response text in output, got: %s", outStr)
	}
	// 验证 token 用量
	if !contains(outStr, `"prompt_tokens":5`) {
		t.Errorf("expected prompt_tokens:5 in output, got: %s", outStr)
	}
	if !contains(outStr, `"completion_tokens":15`) {
		t.Errorf("expected completion_tokens:15 in output, got: %s", outStr)
	}
}

// TestRelay_SameFormat OpenAI→OpenAI 同格式互转。
func TestRelay_SameFormat(t *testing.T) {
	mp := newMockCompleteProvider("same format", 1, 2)
	body := []byte(openAIRequestBody("test"))

	out, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatOpenAI)
	if err != nil {
		t.Fatalf("Relay() error: %v", err)
	}

	outStr := string(out)
	if !contains(outStr, `"object":"chat.completion"`) {
		t.Errorf("expected chat.completion, got: %s", outStr)
	}
}

// TestRelay_WithSystem 系统提示词透传验证。
func TestRelay_WithSystem(t *testing.T) {
	mp := newMockCompleteProvider("ok", 1, 1)
	body := []byte(openAIRequestBodyWithSystem("you are a bot", "hi"))

	_, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err != nil {
		t.Fatalf("Relay() error: %v", err)
	}

	if mp.lastSystem != "you are a bot" {
		t.Errorf("expected system prompt forwarded, got %q", mp.lastSystem)
	}
}

// TestRelay_ErrorHandling 解析错误返回 error。
func TestRelay_ErrorHandling(t *testing.T) {
	mp := newMockCompleteProvider("ok", 1, 1)

	// 无效 JSON
	body := []byte(`{invalid json`)

	_, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestRelay_InvalidInputFormat 不支持的输入格式。
func TestRelay_InvalidInputFormat(t *testing.T) {
	mp := newMockCompleteProvider("ok", 1, 1)
	body := []byte(`{}`)

	_, err := Relay(context.Background(), mp, body, "unknown_format", codec.FormatOpenAI)
	if err == nil {
		t.Fatal("expected error for unknown input format, got nil")
	}
}

// TestRelay_InvalidOutputFormat 不支持的输出格式。
func TestRelay_InvalidOutputFormat(t *testing.T) {
	mp := newMockCompleteProvider("ok", 1, 1)
	body := []byte(openAIRequestBody("hi"))

	_, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, "unknown_format")
	if err == nil {
		t.Fatal("expected error for unknown output format, got nil")
	}
}

// TestRelay_ProviderError Provider 返回错误时的处理。
func TestRelay_ProviderError(t *testing.T) {
	mp := &mockProvider{
		completeErr: errors.New("provider boom"),
	}
	body := []byte(openAIRequestBody("hi"))

	_, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic)
	if err == nil {
		t.Fatal("expected error from provider, got nil")
	}
	if !contains(err.Error(), "provider boom") {
		t.Errorf("expected 'provider boom' in error, got: %v", err)
	}
}

// TestRelay_UsageCallback OnUsage 回调触发验证。
func TestRelay_UsageCallback(t *testing.T) {
	mp := newMockCompleteProvider("hi", 42, 99)
	body := []byte(openAIRequestBody("hi"))

	var gotUsage bamboo.Usage
	called := false

	_, err := Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic,
		WithUsageCallback(func(u bamboo.Usage) {
			called = true
			gotUsage = u
		}),
	)
	if err != nil {
		t.Fatalf("Relay() error: %v", err)
	}

	if !called {
		t.Fatal("expected OnUsage callback to be called")
	}
	if gotUsage.InputTokens != 42 {
		t.Errorf("expected InputTokens=42, got %d", gotUsage.InputTokens)
	}
	if gotUsage.OutputTokens != 99 {
		t.Errorf("expected OutputTokens=99, got %d", gotUsage.OutputTokens)
	}
}

// TestRelay_ErrorCallback OnError 回调触发验证。
func TestRelay_ErrorCallback(t *testing.T) {
	mp := &mockProvider{
		completeErr: errors.New("boom"),
	}
	body := []byte(openAIRequestBody("hi"))

	var gotErr error
	called := false

	_, _ = Relay(context.Background(), mp, body, codec.FormatOpenAI, codec.FormatAnthropic,
		WithErrorCallback(func(e error) {
			called = true
			gotErr = e
		}),
	)

	if !called {
		t.Fatal("expected OnError callback to be called")
	}
	if gotErr == nil {
		t.Fatal("expected non-nil error in callback")
	}
}

// TestRelay_NilProvider nil provider 应 panic（bamboo.NewClient 的行为）。
func TestRelay_NilProvider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil provider")
		}
	}()

	body := []byte(openAIRequestBody("hi"))
	_, _ = Relay(context.Background(), nil, body, codec.FormatOpenAI, codec.FormatAnthropic)
}

// TestApplyOptions 零值配置验证。
func TestApplyOptions(t *testing.T) {
	cfg := applyOptions()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.OnUsage != nil {
		t.Error("expected nil OnUsage for zero options")
	}
	if cfg.OnError != nil {
		t.Error("expected nil OnError for zero options")
	}
}

// contains 简单字符串包含检查（避免引入 strings 包）。
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

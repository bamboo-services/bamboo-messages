package bamboo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/bamboo-services/bamboo-messages/provider/anthropic"
)

// mockProvider 用于单元测试的 Provider 模拟实现。
type mockProvider struct{}

func (m *mockProvider) Chat(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: provider.StreamTypeStart}
	close(ch)
	return ch
}

func (m *mockProvider) ChatWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: provider.StreamTypeStart}
	close(ch)
	return ch
}

func (m *mockProvider) Complete(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{
		Content:      "hello",
		FinishReason: provider.FinishReasonStop,
		Usage:        provider.UsageData{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func (m *mockProvider) CompleteWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{
		Content:      "hello with system",
		FinishReason: provider.FinishReasonStop,
		Usage:        provider.UsageData{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func (m *mockProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

func (m *mockProvider) GetAvailableModels() []string {
	return []string{"test-model"}
}

// TestNewClientNilPanic 验证传入 nil provider 时 panic。
func TestNewClientNilPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("期望 panic，但未发生")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic 值类型应为 string，实际为 %T", r)
		}
		if msg != "bamboo: provider must not be nil" {
			t.Fatalf("panic 消息不匹配: %s", msg)
		}
	}()
	NewClient(nil)
}

// TestNewClientSuccess 验证正常创建客户端返回非 nil。
func TestNewClientSuccess(t *testing.T) {
	c := NewClient(&mockProvider{})
	if c == nil {
		t.Fatal("期望返回非 nil 客户端")
	}
}

// TestCompleteBasic 验证 Complete 基本流程。
func TestCompleteBasic(t *testing.T) {
	c := NewClient(&mockProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	resp, err := c.Complete(ctx, messages, "", nil)
	if err != nil {
		t.Fatalf("Complete 返回错误: %v", err)
	}
	if resp == nil {
		t.Fatal("Response 不应为 nil")
	}
	if resp.ProviderType != "anthropic" {
		t.Fatalf("ProviderType 期望 anthropic，实际 %s", resp.ProviderType)
	}
	if len(resp.Content) == 0 {
		t.Fatal("Content 不应为空")
	}
	tb, ok := resp.Content[0].(*TextBlock)
	if !ok || tb.Text != "hello" {
		t.Fatalf("Content 不匹配: %+v", resp.Content)
	}
}

// TestCompleteWithSystem 验证带系统提示的 Complete。
func TestCompleteWithSystem(t *testing.T) {
	c := NewClient(&mockProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	resp, err := c.Complete(ctx, messages, "你是一个助手", nil)
	if err != nil {
		t.Fatalf("Complete 返回错误: %v", err)
	}
	if resp == nil {
		t.Fatal("Response 不应为 nil")
	}
	if len(resp.Content) == 0 {
		t.Fatal("Content 不应为空")
	}
	tb, ok := resp.Content[0].(*TextBlock)
	if !ok || tb.Text != "hello with system" {
		t.Fatalf("Content 不匹配: %+v", resp.Content)
	}
}

// TestChatBasic 验证 Chat 基本流程（接收 channel 事件）。
func TestChatBasic(t *testing.T) {
	c := NewClient(&mockProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	ch, err := c.Chat(ctx, messages, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	// 读取 channel 中所有事件
	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	if len(events) == 0 {
		t.Fatal("期望至少收到一个事件")
	}
}

// TestChatEmptyMessages 验证空 Content 的消息不再返回错误。
func TestChatEmptyMessages(t *testing.T) {
	c := NewClient(&mockProvider{})
	ctx := context.Background()

	// 空 Content 的消息应被允许，不返回错误
	_, err := c.Chat(ctx, []BambooMessage{{Role: RoleUser, Content: []ContentBlock{}}}, "", nil)
	if err != nil {
		t.Fatalf("空 Content 不应返回错误, got: %v", err)
	}
}

// TestNewClientWithOptions 验证通过 Functional Options 创建客户端。
func TestNewClientWithOptions(t *testing.T) {
	c := NewClientWithOptions(
		WithProvider(&mockProvider{}),
		WithDefaultModel("claude-sonnet-4-20250514"),
	)
	if c == nil {
		t.Fatal("期望返回非 nil 客户端")
	}
}

// TestNewClientWithOptionsNoProvider 验证未提供 provider 时 panic。
func TestNewClientWithOptionsNoProvider(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("期望 panic，但未发生")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic 值类型应为 string，实际为 %T", r)
		}
		if msg != "bamboo: provider must not be nil" {
			t.Fatalf("panic 消息不匹配: %s", msg)
		}
	}()
	NewClientWithOptions(WithDefaultModel("test"))
}

// mockErrorProvider 用于测试错误场景的 Provider 模拟实现。
type mockErrorProvider struct{}

func (m *mockErrorProvider) Chat(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: provider.StreamTypeError}
	close(ch)
	return ch
}

func (m *mockErrorProvider) ChatWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: provider.StreamTypeError}
	close(ch)
	return ch
}

func (m *mockErrorProvider) Complete(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return nil, errors.New("provider error")
}

func (m *mockErrorProvider) CompleteWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return nil, errors.New("provider error with system")
}

func (m *mockErrorProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

func (m *mockErrorProvider) GetAvailableModels() []string {
	return []string{"test-model"}
}

// mockRichProvider 用于测试丰富事件流的 Provider 模拟实现。
type mockRichProvider struct{}

func (m *mockRichProvider) Chat(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent, 4)
	ch <- provider.StreamEvent{Type: provider.StreamTypeStart}
	ch <- provider.StreamEvent{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("hello")}
	ch <- provider.StreamEvent{Type: provider.StreamTypeDelta, Delta: provider.NewUsageDelta(10, 5)}
	ch <- provider.StreamEvent{Type: provider.StreamTypeStop}
	close(ch)
	return ch
}

func (m *mockRichProvider) ChatWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) <-chan provider.StreamEvent {
	return m.Chat(nil, nil, nil)
}

func (m *mockRichProvider) Complete(_ context.Context, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{
		Content:      "response with tools",
		FinishReason: provider.FinishReasonStop,
		Usage:        provider.UsageData{InputTokens: 20, OutputTokens: 10},
	}, nil
}

func (m *mockRichProvider) CompleteWithSystem(_ context.Context, _ string, _ []provider.Message, _ *provider.ChatConfig) (*provider.CompletionResult, error) {
	return m.Complete(nil, nil, nil)
}

func (m *mockRichProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

func (m *mockRichProvider) GetAvailableModels() []string {
	return []string{"test-model"}
}

// ──────────────────────────────────────────────────────────────────────
// 补充单元测试
// ──────────────────────────────────────────────────────────────────────

// TestChatContextCancellation 验证上下文取消时 Chat 正确关闭 channel。
func TestChatContextCancellation(t *testing.T) {
	p := &mockProvider{}
	c := NewClient(p)

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消上下文
	cancel()

	messages := []BambooMessage{NewUserMessage("hi")}
	ch, err := c.Chat(ctx, messages, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	// channel 应该被关闭
	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	// 可能收到 0 个事件（ctx 已取消，goroutine 快速退出）或收到部分事件
	// 关键是 channel 必须被关闭（for range 能正常结束）
	_ = events
}

// TestCompleteEmptyMessages 验证 Complete 传入空 messages 时的行为。
func TestCompleteEmptyMessages(t *testing.T) {
	c := NewClient(&mockProvider{})
	ctx := context.Background()

	// 空 Content 的消息应被允许，不返回错误
	_, err := c.Complete(ctx, []BambooMessage{{Role: RoleUser, Content: []ContentBlock{}}}, "", nil)
	if err != nil {
		t.Fatalf("空 Content 不应返回错误, got: %v", err)
	}
}

// TestCompleteProviderError 验证 Complete 在 provider 返回错误时正确传播。
func TestCompleteProviderError(t *testing.T) {
	c := NewClient(&mockErrorProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	_, err := c.Complete(ctx, messages, "", nil)
	if err == nil {
		t.Fatal("期望 provider 错误被传播")
	}
	if err.Error() != "bamboo: complete failed: provider error" {
		t.Errorf("错误信息 = %q, 不匹配", err.Error())
	}
}

// TestCompleteProviderErrorWithSystem 验证带 system 的 Complete 错误传播。
func TestCompleteProviderErrorWithSystem(t *testing.T) {
	c := NewClient(&mockErrorProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	_, err := c.Complete(ctx, messages, "system prompt", nil)
	if err == nil {
		t.Fatal("期望 provider 错误被传播")
	}
	if err.Error() != "bamboo: complete failed: provider error with system" {
		t.Errorf("错误信息 = %q, 不匹配", err.Error())
	}
}

// TestChatProviderError 验证 Chat 在 provider 返回 error event 时正确传播。
func TestChatProviderError(t *testing.T) {
	c := NewClient(&mockErrorProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	ch, err := c.Chat(ctx, messages, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	if len(events) == 0 {
		t.Fatal("期望至少收到一个错误事件")
	}

	// 最后一个事件应为 error 类型
	last := events[len(events)-1]
	if last.Type != EventError {
		t.Errorf("最后事件类型 = %q, 期望 error", last.Type)
	}
}

// TestChatWithSystemBasic 验证带 system 的 Chat 基本流程。
func TestChatWithSystemBasic(t *testing.T) {
	c := NewClient(&mockProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	ch, err := c.Chat(ctx, messages, "你是一个助手", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	if len(events) == 0 {
		t.Fatal("期望至少收到一个事件")
	}
}

// TestCompleteWithConfig 验证 Complete 使用 RequestConfig 的完整流程。
func TestCompleteWithConfig(t *testing.T) {
	c := NewClient(&mockRichProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	config := &RequestConfig{
		Model:       "test-model",
		MaxTokens:   1024,
		Temperature: PtrFloat64(0.7),
		TopP:        PtrFloat64(0.9),
		Tools: []Tool{
			{
				Name:        "get_weather",
				Description: "Get weather info",
				InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"city": {"type": "string", "description": "City name"}
				},
				"required": ["city"]
			}`),
			},
		},
		StopSequences: []string{"STOP"},
		Metadata:      map[string]string{"key": "value"},
	}

	resp, err := c.Complete(ctx, messages, "", config)
	if err != nil {
		t.Fatalf("Complete 返回错误: %v", err)
	}
	if resp == nil {
		t.Fatal("Response 不应为 nil")
	}
	if resp.ProviderType != "anthropic" {
		t.Errorf("ProviderType = %q, 期望 anthropic", resp.ProviderType)
	}
}

// TestCompleteWithSystemAndConfig 验证带 system 和 config 的 Complete。
func TestCompleteWithSystemAndConfig(t *testing.T) {
	c := NewClient(&mockRichProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	config := &RequestConfig{
		Model:     "test-model",
		MaxTokens: 512,
	}

	resp, err := c.Complete(ctx, messages, "system prompt", config)
	if err != nil {
		t.Fatalf("Complete 返回错误: %v", err)
	}
	if resp == nil {
		t.Fatal("Response 不应为 nil")
	}
	if len(resp.Content) == 0 {
		t.Fatal("Content 不应为空")
	}
	tb, ok := resp.Content[0].(*TextBlock)
	if !ok || tb.Text != "response with tools" {
		t.Errorf("Content 不匹配: %+v", resp.Content)
	}
}

// TestChatRichStream 验证完整流式事件序列的转换。
func TestChatRichStream(t *testing.T) {
	c := NewClient(&mockRichProvider{})
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("hi")}
	ch, err := c.Chat(ctx, messages, "", nil)
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// 期望事件: message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop
	if len(events) < 3 {
		t.Fatalf("期望至少 3 个事件, 实际 %d", len(events))
	}

	// 首个事件应为 message_start
	if events[0].Type != EventMessageStart {
		t.Errorf("首个事件类型 = %q, 期望 message_start", events[0].Type)
	}

	// 最后一个事件应为 message_stop
	last := events[len(events)-1]
	if last.Type != EventMessageStop {
		t.Errorf("最后事件类型 = %q, 期望 message_stop", last.Type)
	}
}

// ──────────────────────────────────────────────────────────────────────
// 集成测试 — 需要 ANTHROPIC_API_KEY 环境变量
//
// 运行方式：
//
//	ANTHROPIC_API_KEY=sk-ant-xxx go test ./bamboo/... -v -run TestIntegration
// ──────────────────────────────────────────────────────────────────────

// getIntegrationClient 创建用于集成测试的 BambooClient。
// 若 ANTHROPIC_API_KEY 未设置则跳过测试。
func getIntegrationClient(t *testing.T) BambooClient {
	t.Helper()
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY 未设置，跳过集成测试")
	}
	p := anthropic.NewProvider(apiKey)
	return NewClient(p)
}

// TestIntegrationComplete 集成测试：非流式对话基本流程。
func TestIntegrationComplete(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("Say 'hello world' and nothing else.")}
	config := &RequestConfig{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 64,
	}

	resp, err := client.Complete(ctx, messages, "", config)
	if err != nil {
		t.Fatalf("Complete 失败: %v", err)
	}

	// 验证 Response 结构
	if resp.ID == "" {
		t.Error("Response.ID 不应为空")
	}
	if resp.Type != "message" {
		t.Errorf("Response.Type = %q, 期望 'message'", resp.Type)
	}
	if resp.Role != RoleAssistant {
		t.Errorf("Response.Role = %q, 期望 'assistant'", resp.Role)
	}
	if len(resp.Content) == 0 {
		t.Fatal("Response.Content 不应为空")
	}
	if resp.ProviderType != "anthropic" {
		t.Errorf("Response.ProviderType = %q, 期望 'anthropic'", resp.ProviderType)
	}
	if resp.Usage.InputTokens == 0 {
		t.Error("Usage.InputTokens 应大于 0")
	}
	if resp.RequestID == "" {
		t.Error("RequestID 不应为空")
	}
	if resp.CreatedAt == 0 {
		t.Error("CreatedAt 不应为零")
	}
}

// TestIntegrationCompleteWithSystem 集成测试：带系统提示的非流式对话。
func TestIntegrationCompleteWithSystem(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("What is 1+1? Reply with just the number.")}
	config := &RequestConfig{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 16,
	}

	resp, err := client.Complete(ctx, messages, "You are a math assistant. Be concise.", config)
	if err != nil {
		t.Fatalf("Complete 失败: %v", err)
	}
	if len(resp.Content) == 0 {
		t.Fatal("Response.Content 不应为空")
	}
}

// TestIntegrationChatStream 集成测试：流式对话完整事件序列验证。
func TestIntegrationChatStream(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	messages := []BambooMessage{NewUserMessage("Say 'ping' and nothing else.")}
	config := &RequestConfig{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 16,
	}

	eventCh, err := client.Chat(ctx, messages, "", config)
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	var events []StreamEvent
	for e := range eventCh {
		events = append(events, e)
	}

	if len(events) == 0 {
		t.Fatal("期望至少收到一个事件")
	}

	// 验证事件序列以 message_start 开头
	if events[0].Type != EventMessageStart {
		t.Errorf("首个事件类型 = %q, 期望 message_start", events[0].Type)
	}

	// 验证事件序列以 message_stop 结尾
	last := events[len(events)-1]
	if last.Type != EventMessageStop {
		t.Errorf("末尾事件类型 = %q, 期望 message_stop", last.Type)
	}
}

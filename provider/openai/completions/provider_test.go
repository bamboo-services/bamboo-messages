package completions

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 构造函数测试
// ==============================

func TestNewCompletionsProvider(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	if p == nil {
		t.Fatal("NewCompletionsProvider() returned nil")
	}
	_ = p.GetAvailableModels()
}

// ==============================
// GetProviderType 测试
// ==============================

func TestCompletionsProvider_GetProviderType(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	want := provider.ProviderOpenAICompletions
	if got := p.GetProviderType(); got != want {
		t.Errorf("GetProviderType() = %v, want %v", got, want)
	}
}

// ==============================
// GetAvailableModels 测试
// ==============================

func TestCompletionsProvider_GetAvailableModels(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	models := p.GetAvailableModels()

	if len(models) == 0 {
		t.Error("GetAvailableModels() returned empty list")
	}

	expectedModels := []string{
		openai.ChatModelGPT4o,
		openai.ChatModelGPT4oMini,
		openai.ChatModelGPT4_1,
		openai.ChatModelGPT4_1Mini,
		openai.ChatModelGPT4_1Nano,
		openai.ChatModelO3,
		openai.ChatModelO3Mini,
		openai.ChatModelO4Mini,
	}

	for _, expected := range expectedModels {
		found := false
		for _, model := range models {
			if model == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetAvailableModels() missing expected model: %s", expected)
		}
	}
}

// ==============================
// buildMessages 测试
// ==============================

func TestCompletionsProvider_buildMessages(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	tests := []struct {
		name         string
		systemPrompt string
		messages     []provider.Message
		wantLen      int
		check        func(t *testing.T, result []openai.ChatCompletionMessageParamUnion)
	}{
		{
			name:         "empty messages and no system prompt",
			systemPrompt: "",
			messages:     []provider.Message{},
			wantLen:      0,
		},
		{
			name:         "with system prompt",
			systemPrompt: "You are a helpful assistant.",
			messages:     []provider.Message{},
			wantLen:      1,
			check: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfSystem == nil {
					t.Error("expected system message at index 0")
				}
			},
		},
		{
			name:         "user message",
			systemPrompt: "",
			messages: []provider.Message{
				{Role: provider.RoleUser, Content: "Hello"},
			},
			wantLen: 1,
			check: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfUser == nil {
					t.Error("expected user message")
				}
			},
		},
		{
			name:         "assistant text only",
			systemPrompt: "",
			messages: []provider.Message{
				{Role: provider.RoleAssistant, Content: "Hi there!"},
			},
			wantLen: 1,
			check: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfAssistant == nil {
					t.Error("expected assistant message")
				}
			},
		},
		{
			name:         "assistant with tool calls",
			systemPrompt: "",
			messages: []provider.Message{
				{
					Role:    provider.RoleAssistant,
					Content: "Let me check that.",
					ToolCalls: []provider.ToolCall{
						{
							ID:   "call-123",
							Type: "function",
							Function: provider.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "Tokyo"}`,
							},
						},
					},
				},
			},
			wantLen: 1,
			check: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfAssistant == nil {
					t.Error("expected assistant message")
				}
				if len(result[0].OfAssistant.ToolCalls) != 1 {
					t.Errorf("expected 1 tool call, got %d", len(result[0].OfAssistant.ToolCalls))
				}
			},
		},
		{
			name:         "tool response message",
			systemPrompt: "",
			messages: []provider.Message{
				{
					Role:       provider.RoleTool,
					Content:    `{"temperature": 25}`,
					ToolCallID: "call-123",
				},
			},
			wantLen: 1,
			check: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfTool == nil {
					t.Error("expected tool message")
				}
			},
		},
		{
			name:         "mixed message sequence with system prompt",
			systemPrompt: "You are a weather assistant.",
			messages: []provider.Message{
				{Role: provider.RoleUser, Content: "What's the weather?"},
				{
					Role:    provider.RoleAssistant,
					Content: "",
					ToolCalls: []provider.ToolCall{
						{
							ID:   "call-789",
							Type: "function",
							Function: provider.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "Paris"}`,
							},
						},
					},
				},
				{
					Role:       provider.RoleTool,
					Content:    `{"temperature": 20}`,
					ToolCallID: "call-789",
				},
				{Role: provider.RoleAssistant, Content: "It's 20°C in Paris."},
			},
			wantLen: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.buildMessages(tt.systemPrompt, tt.messages)
			if len(result) != tt.wantLen {
				t.Errorf("buildMessages() returned %d messages, want %d", len(result), tt.wantLen)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// ==============================
// handleChunk 测试
// ==============================

func TestCompletionsProvider_handleChunk(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	tests := []struct {
		name    string
		chunk   openai.ChatCompletionChunk
		wantLen int
		check   func(t *testing.T, events []provider.StreamEvent)
	}{
		{
			name: "chunk with usage only",
			chunk: openai.ChatCompletionChunk{
				ID:      "chunk-1",
				Choices: []openai.ChatCompletionChunkChoice{},
				Usage: openai.CompletionUsage{
					TotalTokens:      150,
					PromptTokens:     100,
					CompletionTokens: 50,
				},
			},
			wantLen: 1,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeUsage {
					t.Errorf("expected usage delta, got %v", events[0].Delta.Type)
				}
			},
		},
		{
			name: "chunk with text content",
			chunk: openai.ChatCompletionChunk{
				ID: "chunk-2",
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionChunkChoiceDelta{
							Content: "Hello",
						},
						FinishReason: "",
					},
				},
				Usage: openai.CompletionUsage{},
			},
			wantLen: 1,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeTextOutput {
					t.Errorf("expected text delta, got %v", events[0].Delta.Type)
				}
			},
		},
		{
			name: "chunk with finish reason stop",
			chunk: openai.ChatCompletionChunk{
				ID: "chunk-4",
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Index:        0,
						Delta:        openai.ChatCompletionChunkChoiceDelta{},
						FinishReason: "stop",
					},
				},
				Usage: openai.CompletionUsage{},
			},
			wantLen: 1,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Type != provider.StreamTypeStop {
					t.Errorf("expected stop event, got %v", events[0].Type)
				}
			},
		},
		{
			name: "empty chunk returns nil",
			chunk: openai.ChatCompletionChunk{
				ID:      "chunk-5",
				Choices: []openai.ChatCompletionChunkChoice{},
				Usage:   openai.CompletionUsage{},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.handleChunk(tt.chunk)
			if len(result) != tt.wantLen {
				t.Errorf("handleChunk() returned %d events, want %d", len(result), tt.wantLen)
				return
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// ==============================
// mapFinishReason 测试
// ==============================

func TestCompletions_mapFinishReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   provider.FinishReason
	}{
		{"stop", "stop", provider.FinishReasonStop},
		{"length", "length", provider.FinishReasonLength},
		{"tool_calls", "tool_calls", provider.FinishReasonToolCalls},
		{"unknown", "unknown_reason", provider.FinishReasonStop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapFinishReason(tt.reason); got != tt.want {
				t.Errorf("mapFinishReason(%v) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

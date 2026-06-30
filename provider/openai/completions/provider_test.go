package completions

import (
	"testing"

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
		ModelGPT4o,
		ModelGPT4oMini,
		ModelGPT4_1,
		ModelGPT4_1Mini,
		ModelGPT4_1Nano,
		ModelO3,
		ModelO3Mini,
		ModelO4Mini,
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
		check        func(t *testing.T, result []map[string]any)
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
			check: func(t *testing.T, result []map[string]any) {
				if result[0]["role"] != "system" {
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
			check: func(t *testing.T, result []map[string]any) {
				if result[0]["role"] != "user" {
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
			check: func(t *testing.T, result []map[string]any) {
				if result[0]["role"] != "assistant" {
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
			check: func(t *testing.T, result []map[string]any) {
				if result[0]["role"] != "assistant" {
					t.Error("expected assistant message")
				}
				tc, ok := result[0]["tool_calls"].([]map[string]any)
				if !ok {
					t.Fatalf("expected tool_calls to be []map[string]any, got %T", result[0]["tool_calls"])
				}
				if len(tc) != 1 {
					t.Errorf("expected 1 tool call, got %d", len(tc))
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
			check: func(t *testing.T, result []map[string]any) {
				if result[0]["role"] != "tool" {
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
		chunk   chatCompletionChunk
		wantLen int
		check   func(t *testing.T, events []provider.StreamEvent)
	}{
		{
			name: "chunk with usage only",
			chunk: chatCompletionChunk{
				ID:      "chunk-1",
				Choices: []chatCompletionChunkChoice{},
				Usage: &chunkUsage{
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
			chunk: chatCompletionChunk{
				ID: "chunk-2",
				Choices: []chatCompletionChunkChoice{
					{
						Index: 0,
						Delta: chatCompletionDelta{
							Content: "Hello",
						},
						FinishReason: nil,
					},
				},
				Usage: &chunkUsage{},
			},
			wantLen: 2,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeBlockStart {
					t.Errorf("expected block_start delta, got %v", events[0].Delta.Type)
				}
				if events[1].Delta.Type != provider.StreamDeltaTypeTextOutput {
					t.Errorf("expected text delta, got %v", events[1].Delta.Type)
				}
			},
		},
		{
			name: "chunk with finish reason stop",
			chunk: chatCompletionChunk{
				ID: "chunk-4",
				Choices: []chatCompletionChunkChoice{
					{
						Index:        0,
						Delta:        chatCompletionDelta{},
						FinishReason: strPtr("stop"),
					},
				},
				Usage: &chunkUsage{},
			},
			wantLen: 1,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Type != provider.StreamTypeStop {
					t.Errorf("expected stop event, got %v", events[0].Type)
				}
			},
		},
		{
			name: "chunk with finish reason tool_calls",
			chunk: chatCompletionChunk{
				ID: "chunk-tc",
				Choices: []chatCompletionChunkChoice{
					{
						Index:        0,
						Delta:        chatCompletionDelta{},
						FinishReason: strPtr("tool_calls"),
					},
				},
				Usage: &chunkUsage{},
			},
			wantLen: 1,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Type != provider.StreamTypeStop {
					t.Errorf("expected stop event for tool_calls, got %v", events[0].Type)
				}
			},
		},
		{
			name: "chunk with finish reason length",
			chunk: chatCompletionChunk{
				ID: "chunk-len",
				Choices: []chatCompletionChunkChoice{
					{
						Index:        0,
						Delta:        chatCompletionDelta{},
						FinishReason: strPtr("length"),
					},
				},
				Usage: &chunkUsage{},
			},
			wantLen: 1,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Type != provider.StreamTypeStop {
					t.Errorf("expected stop event for length, got %v", events[0].Type)
				}
			},
		},
		{
			name: "empty chunk returns nil",
			chunk: chatCompletionChunk{
				ID:      "chunk-5",
				Choices: []chatCompletionChunkChoice{},
				Usage:   &chunkUsage{},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			textBlockStarted := false
			thinkingBlockStarted := false
			stopSent := false
			result := p.handleChunk(tt.chunk, &textBlockStarted, &thinkingBlockStarted, &stopSent)
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

// strPtr 辅助函数，返回字符串指针。
func strPtr(s string) *string {
	return &s
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

// ==============================
// Options 模式测试
// ==============================

func TestNewCompletionsProviderWithOptions(t *testing.T) {
	t.Run("仅 WithAPIKey", func(t *testing.T) {
		p := NewCompletionsProviderWithOptions(WithAPIKey("test-key"))
		if p == nil {
			t.Fatal("returned nil")
		}
	})

	t.Run("WithAPIKey + WithBaseURL", func(t *testing.T) {
		p := NewCompletionsProviderWithOptions(
			WithAPIKey("test-key"),
			WithBaseURL("https://ollama.local/v1"),
		)
		if p == nil {
			t.Fatal("with BaseURL returned nil")
		}
		_ = p.GetAvailableModels()
	})

	t.Run("完整选项", func(t *testing.T) {
		p := NewCompletionsProviderWithOptions(
			WithAPIKey("test-key"),
			WithBaseURL("https://ollama.local/v1"),
			WithHeader("X-Custom", "value"),
		)
		if p == nil {
			t.Fatal("with full options returned nil")
		}
		_ = p.GetAvailableModels()
	})
}

func TestNewCompletionsProviderWithOptions_EmptyOptions(t *testing.T) {
	p := NewCompletionsProviderWithOptions()
	if p == nil {
		t.Fatal("with no args returned nil")
	}
}

// ==============================
// buildResponseFormat 测试
// ==============================

// TestBuildResponseFormat_MapInput 验证 map[string]any 输入的 ResponseFormat 转换。
func TestBuildResponseFormat_MapInput(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantNil   bool
		wantType  string
	}{
		{
			name:     "json_object map",
			input:    map[string]any{"type": "json_object"},
			wantType: "json_object",
		},
		{
			name:     "text map",
			input:    map[string]any{"type": "text"},
			wantType: "text",
		},
		{
			name:    "unknown type map",
			input:   map[string]any{"type": "unknown"},
			wantNil: true,
		},
		{
			name:    "map missing type key",
			input:   map[string]any{"foo": "bar"},
			wantNil: true,
		},
		{
			name:    "map with non-string type",
			input:   map[string]any{"type": 123},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResponseFormat(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("期望 nil，但得到 %v", result)
				}
				return
			}
			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("期望 map[string]any，得到 %T", result)
			}
			if m["type"] != tt.wantType {
				t.Errorf("type = %v, 期望 %v", m["type"], tt.wantType)
			}
		})
	}
}

// TestBuildResponseFormat_StringInput 验证字符串输入的 ResponseFormat 转换。
func TestBuildResponseFormat_StringInput(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantNil  bool
		wantType string
	}{
		{
			name:     "text string",
			input:    "text",
			wantType: "text",
		},
		{
			name:     "json_object string",
			input:    "json_object",
			wantType: "json_object",
		},
		{
			name:    "unknown string",
			input:   "unknown",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResponseFormat(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Error("期望 nil")
				}
				return
			}
			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("期望 map[string]any，得到 %T", result)
			}
			if m["type"] != tt.wantType {
				t.Errorf("type = %v, 期望 %v", m["type"], tt.wantType)
			}
		})
	}
}

// ==============================
// buildStop 测试
// ==============================

func TestBuildStop(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantNil bool
		wantLen int
	}{
		{name: "空列表", input: []string{}, wantNil: true},
		{name: "nil 列表", input: nil, wantNil: true},
		{name: "单个停止词", input: []string{"STOP"}, wantLen: 1},
		{name: "多个停止词", input: []string{"STOP", "END", "DONE"}, wantLen: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStop(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Error("空输入应返回 nil")
				}
				return
			}
			arr, ok := result.([]string)
			if !ok {
				t.Fatalf("期望 []string，得到 %T", result)
			}
			if len(arr) != tt.wantLen {
				t.Errorf("停止词数量 = %d, 期望 %d", len(arr), tt.wantLen)
			}
		})
	}
}

// ==============================
// buildTools 测试
// ==============================

func TestBuildTools(t *testing.T) {
	tests := []struct {
		name    string
		input   []provider.Tool
		wantNil bool
		wantLen int
	}{
		{name: "空列表", input: []provider.Tool{}, wantNil: true},
		{name: "nil 列表", input: nil, wantNil: true},
		{
			name: "单个 function 工具",
			input: []provider.Tool{
				{
					Type: "function",
					Function: provider.FunctionDef{
						Name:        "get_weather",
						Description: "获取天气信息",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"location": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "多个 function 工具",
			input: []provider.Tool{
				{Type: "function", Function: provider.FunctionDef{Name: "get_weather", Parameters: map[string]any{"type": "object"}}},
				{Type: "function", Function: provider.FunctionDef{Name: "search", Parameters: map[string]any{"type": "object"}}},
			},
			wantLen: 2,
		},
		{
			name: "过滤非 function 类型",
			input: []provider.Tool{
				{Type: "other_type", Function: provider.FunctionDef{Name: "ignored"}},
				{Type: "function", Function: provider.FunctionDef{Name: "get_weather"}},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTools(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Error("空输入应返回 nil")
				}
				return
			}
			if len(result) != tt.wantLen {
				t.Errorf("工具数量 = %d, 期望 %d", len(result), tt.wantLen)
			}
		})
	}
}

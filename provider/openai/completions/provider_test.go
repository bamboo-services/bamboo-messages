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

// TestCompletionsProvider_buildMessages_ToolFollowsAssistant 验证场景 C：
// user 文本消息夹在 assistant(tool_calls) 与 tool 响应之间时，
// tool 消息被插到紧跟 assistant 的位置（Chat Completions 邻接约束）。
func TestCompletionsProvider_buildMessages_ToolFollowsAssistant(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	msgs := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "tc1", Type: "function", Function: provider.FunctionCall{Name: "get_weather"}},
			},
		},
		{Role: provider.RoleUser, Content: "谢谢"},
		{Role: provider.RoleTool, Content: "晴", ToolCallID: "tc1"},
	}

	result := p.buildMessages("", msgs)
	if len(result) != 3 {
		t.Fatalf("期望 3 条消息, 实际 %d", len(result))
	}
	// assistant(tc1) → tool(tc1) → user("谢谢")
	if result[0]["role"] != "assistant" {
		t.Errorf("result[0].role = %q, 期望 assistant", result[0]["role"])
	}
	if result[1]["role"] != "tool" {
		t.Errorf("result[1].role = %q, 期望 tool", result[1]["role"])
	}
	if result[1]["tool_call_id"] != "tc1" {
		t.Errorf("result[1].tool_call_id = %v, 期望 tc1", result[1]["tool_call_id"])
	}
	if result[2]["role"] != "user" {
		t.Errorf("result[2].role = %q, 期望 user", result[2]["role"])
	}
	if result[2]["content"] != "谢谢" {
		t.Errorf("result[2].content = %v, 期望 谢谢", result[2]["content"])
	}
}

// TestCompletionsProvider_buildMessages_ParallelToolOrderPreserved 验证并行 tool_call 的响应保持输入顺序。
func TestCompletionsProvider_buildMessages_ParallelToolOrderPreserved(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	msgs := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "tc1", Type: "function", Function: provider.FunctionCall{Name: "f1"}},
				{ID: "tc2", Type: "function", Function: provider.FunctionCall{Name: "f2"}},
			},
		},
		{Role: provider.RoleTool, Content: "r1", ToolCallID: "tc1"},
		{Role: provider.RoleTool, Content: "r2", ToolCallID: "tc2"},
	}

	result := p.buildMessages("", msgs)
	if len(result) != 3 {
		t.Fatalf("期望 3 条消息, 实际 %d", len(result))
	}
	if result[1]["tool_call_id"] != "tc1" {
		t.Errorf("result[1].tool_call_id = %v, 期望 tc1", result[1]["tool_call_id"])
	}
	if result[2]["tool_call_id"] != "tc2" {
		t.Errorf("result[2].tool_call_id = %v, 期望 tc2", result[2]["tool_call_id"])
	}
}

// TestCompletionsProvider_buildMessages_MultiTurnReordering 验证多轮工具调用时每轮 tool 都紧跟其 assistant。
func TestCompletionsProvider_buildMessages_MultiTurnReordering(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	msgs := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "tc1", Type: "function", Function: provider.FunctionCall{Name: "f1"}},
			},
		},
		{Role: provider.RoleTool, Content: "r1", ToolCallID: "tc1"},
		{Role: provider.RoleUser, Content: "继续"},
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "tc2", Type: "function", Function: provider.FunctionCall{Name: "f2"}},
			},
		},
		{Role: provider.RoleUser, Content: "夹心文本"},
		{Role: provider.RoleTool, Content: "r2", ToolCallID: "tc2"},
	}

	result := p.buildMessages("", msgs)
	// 期望顺序：assistant(tc1) → tool(tc1) → user("继续") → assistant(tc2) → tool(tc2) → user("夹心文本")
	if len(result) != 6 {
		t.Fatalf("期望 6 条消息, 实际 %d", len(result))
	}
	if result[1]["role"] != "tool" || result[1]["tool_call_id"] != "tc1" {
		t.Errorf("result[1] 期望 tool(tc1), 实际 role=%v", result[1]["role"])
	}
	if result[2]["role"] != "user" || result[2]["content"] != "继续" {
		t.Errorf("result[2] 期望 user(继续), 实际 role=%v", result[2]["role"])
	}
	if result[4]["role"] != "tool" || result[4]["tool_call_id"] != "tc2" {
		t.Errorf("result[4] 期望 tool(tc2), 实际 role=%v", result[4]["role"])
	}
	if result[5]["role"] != "user" || result[5]["content"] != "夹心文本" {
		t.Errorf("result[5] 期望 user(夹心文本), 实际 role=%v", result[5]["role"])
	}
}

// TestCompletionsProvider_buildMessages_KeepsExistingOrder 验证正常配对序列顺序不变（防回归）。
func TestCompletionsProvider_buildMessages_KeepsExistingOrder(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "What's the weather?"},
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "call-789", Type: "function", Function: provider.FunctionCall{Name: "get_weather"}},
			},
		},
		{Role: provider.RoleTool, Content: `{"temperature": 20}`, ToolCallID: "call-789"},
		{Role: provider.RoleAssistant, Content: "It's 20°C in Paris."},
	}

	result := p.buildMessages("", msgs)
	if len(result) != 4 {
		t.Fatalf("期望 4 条消息, 实际 %d", len(result))
	}
	wantRoles := []string{"user", "assistant", "tool", "assistant"}
	for i, want := range wantRoles {
		if result[i]["role"] != want {
			t.Errorf("result[%d].role = %v, 期望 %s", i, result[i]["role"], want)
		}
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
			result := p.handleChunk(tt.chunk, &textBlockStarted, &thinkingBlockStarted, &stopSent, nil)
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
		name     string
		input    any
		wantNil  bool
		wantType string
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

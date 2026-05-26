package responses

import (
	"context"
	"encoding/json"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// unmarshalResponseEvent 将 JSON 字符串反序列化为 OpenAI Responses 流事件 Union 类型。
func unmarshalResponseEvent(t *testing.T, rawJSON string) responses.ResponseStreamEventUnion {
	t.Helper()
	var event responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
		t.Fatalf("failed to unmarshal event JSON: %v", err)
	}
	return event
}

// ==============================
// 构造函数测试
// ==============================

func TestNewResponsesProvider(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	if p == nil {
		t.Fatal("NewResponsesProvider() returned nil")
	}
	_ = p.GetAvailableModels()
}

// ==============================
// GetProviderType 测试
// ==============================

func TestResponsesProvider_GetProviderType(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	want := provider.ProviderOpenAIResponses
	if got := p.GetProviderType(); got != want {
		t.Errorf("GetProviderType() = %v, want %v", got, want)
	}
}

// ==============================
// GetAvailableModels 测试
// ==============================

func TestResponsesProvider_GetAvailableModels(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	models := p.GetAvailableModels()

	if len(models) == 0 {
		t.Error("GetAvailableModels() returned empty list")
	}

	expectedModels := []string{
		openaisdk.ChatModelGPT4o,
		openaisdk.ChatModelGPT4oMini,
		openaisdk.ChatModelGPT4_1,
		openaisdk.ChatModelGPT4_1Mini,
		openaisdk.ChatModelGPT4_1Nano,
		openaisdk.ChatModelO3,
		openaisdk.ChatModelO3Mini,
		openaisdk.ChatModelO4Mini,
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
// buildInput 测试
// ==============================

func TestResponsesProvider_buildInput(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	tests := []struct {
		name         string
		systemPrompt string
		messages     []provider.Message
		wantItems    int
		check        func(t *testing.T, input responses.ResponseNewParamsInputUnion)
	}{
		{
			name:         "empty messages and no system prompt",
			systemPrompt: "",
			messages:     []provider.Message{},
			wantItems:    0,
		},
		{
			name:         "with system prompt",
			systemPrompt: "You are a helpful assistant.",
			messages:     []provider.Message{},
			wantItems:    1,
			check: func(t *testing.T, input responses.ResponseNewParamsInputUnion) {
				if len(input.OfInputItemList) != 1 {
					t.Error("expected system message in input")
				}
			},
		},
		{
			name:         "user message",
			systemPrompt: "",
			messages: []provider.Message{
				{Role: provider.RoleUser, Content: "Hello"},
			},
			wantItems: 1,
			check: func(t *testing.T, input responses.ResponseNewParamsInputUnion) {
				if len(input.OfInputItemList) != 1 {
					t.Error("expected user message in input")
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
			wantItems: 1,
			check: func(t *testing.T, input responses.ResponseNewParamsInputUnion) {
				if len(input.OfInputItemList) != 1 {
					t.Error("expected function call output in input")
				}
			},
		},
		{
			name:         "mixed message sequence",
			systemPrompt: "You are a weather assistant.",
			messages: []provider.Message{
				{Role: provider.RoleUser, Content: "What's the weather?"},
				{Role: provider.RoleAssistant, Content: "It's sunny!"},
			},
			wantItems: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.buildInput(tt.systemPrompt, tt.messages)
			if len(result.OfInputItemList) != tt.wantItems {
				t.Errorf("buildInput() returned %d items, want %d", len(result.OfInputItemList), tt.wantItems)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// ==============================
// handleStreamEvent 测试
// ==============================

func TestResponsesProvider_handleStreamEvent(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	tests := []struct {
		name     string
		rawJSON  string
		wantLen  int
		wantType provider.StreamType
		check    func(t *testing.T, events []provider.StreamEvent)
	}{
		{
			name:     "response.created returns nil",
			rawJSON:  `{"type":"response.created","response":{"id":"resp_01","object":"response","created_at":1743000000,"status":"in_progress","model":"gpt-4o","output":[]}}`,
			wantLen:  0,
			wantType: "",
		},
		{
			name:     "response.output_item.added with function_call",
			rawJSON:  `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_01ABC","call_id":"call_abc123","name":"get_weather","arguments":""}}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeToolCall {
					t.Errorf("expected tool_call delta, got %v", events[0].Delta.Type)
				}
			},
		},
		{
			name:     "response.output_text.delta",
			rawJSON:  `{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello world"}`,
			wantLen:  2,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeBlockStart {
					t.Errorf("expected block_start delta first, got %v", events[0].Delta.Type)
				}
				if events[1].Delta.Type != provider.StreamDeltaTypeTextOutput {
					t.Errorf("expected text_output delta second, got %v", events[1].Delta.Type)
				}
			},
		},
		{
			name:     "response.completed with usage",
			rawJSON:  `{"type":"response.completed","response":{"id":"resp_01","object":"response","created_at":1743000000,"status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150}}}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeUsage {
					t.Errorf("expected usage delta, got %v", events[0].Delta.Type)
				}
			},
		},
		{
			name:     "response.failed with error",
			rawJSON:  `{"type":"response.failed","response":{"id":"resp_01","object":"response","created_at":1743000000,"status":"failed","model":"gpt-4o","output":[],"error":{"code":"server_error","message":"Internal server error"}}}`,
			wantLen:  1,
			wantType: provider.StreamTypeError,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Err == nil {
					t.Error("expected error in event")
				}
			},
		},
		{
			name:     "response.incomplete returns stop",
			rawJSON:  `{"type":"response.incomplete","response":{"id":"resp_01","object":"response","created_at":1743000000,"status":"incomplete","model":"gpt-4o","output":[]}}`,
			wantLen:  1,
			wantType: provider.StreamTypeStop,
		},
		{
			name:     "unknown event type returns nil",
			rawJSON:  `{"type":"unknown_type"}`,
			wantLen:  0,
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := unmarshalResponseEvent(t, tt.rawJSON)
			textBlockStarted := false
			result := p.handleStreamEvent(context.Background(), event, &textBlockStarted)
			if len(result) != tt.wantLen {
				t.Errorf("handleStreamEvent() returned %d events, want %d", len(result), tt.wantLen)
				return
			}
			if tt.wantLen > 0 && tt.wantType != "" {
				if result[0].Type != tt.wantType {
					t.Errorf("handleStreamEvent() event type = %v, want %v", result[0].Type, tt.wantType)
				}
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// ==============================
// Options 模式测试
// ==============================

func TestNewResponsesProviderWithOptions(t *testing.T) {
	t.Run("仅 WithAPIKey", func(t *testing.T) {
		p := NewResponsesProviderWithOptions(WithAPIKey("test-key"))
		if p == nil {
			t.Fatal("returned nil")
		}
	})

	t.Run("WithAPIKey + WithBaseURL", func(t *testing.T) {
		p := NewResponsesProviderWithOptions(
			WithAPIKey("test-key"),
			WithBaseURL("https://gateway.example.com/v1"),
		)
		if p == nil {
			t.Fatal("with BaseURL returned nil")
		}
		_ = p.GetAvailableModels()
	})

	t.Run("完整选项", func(t *testing.T) {
		p := NewResponsesProviderWithOptions(
			WithAPIKey("test-key"),
			WithBaseURL("https://gateway.example.com/v1"),
			WithHeader("X-Custom", "value"),
		)
		if p == nil {
			t.Fatal("with full options returned nil")
		}
		_ = p.GetAvailableModels()
	})
}



func TestNewResponsesProviderWithOptions_EmptyOptions(t *testing.T) {
	p := NewResponsesProviderWithOptions()
	if p == nil {
		t.Fatal("with no args returned nil")
	}
}

// ==============================
// buildAssistantItem 测试
// ==============================

func TestResponsesProvider_buildAssistantItem(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	tests := []struct {
		name      string
		msg       provider.Message
		wantCount int
		check     func(t *testing.T, items []responses.ResponseInputItemUnionParam)
	}{
		{
			name: "text-only assistant message",
			msg: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "Hello, how can I help you?",
			},
			wantCount: 1,
			check: func(t *testing.T, items []responses.ResponseInputItemUnionParam) {
				if items[0].OfMessage == nil {
					t.Error("expected message item")
				}
				if items[0].OfMessage.Role != responses.EasyInputMessageRoleAssistant {
					t.Error("expected assistant role")
				}
			},
		},
		{
			name: "tool-calls-only assistant message (single)",
			msg: provider.Message{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{
						ID: "call_123",
						Function: provider.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"city":"Beijing"}`,
						},
					},
				},
			},
			wantCount: 1,
			check: func(t *testing.T, items []responses.ResponseInputItemUnionParam) {
				if items[0].OfFunctionCall == nil {
					t.Error("expected function call item")
				}
				if items[0].OfFunctionCall.Name != "get_weather" {
					t.Errorf("expected function name 'get_weather', got '%s'", items[0].OfFunctionCall.Name)
				}
			},
		},
		{
			name: "tool-calls-only assistant message (multiple)",
			msg: provider.Message{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{
						ID: "call_123",
						Function: provider.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"city":"Beijing"}`,
						},
					},
					{
						ID: "call_456",
						Function: provider.FunctionCall{
							Name:      "get_time",
							Arguments: `{"timezone":"UTC"}`,
						},
					},
				},
			},
			wantCount: 2,
			check: func(t *testing.T, items []responses.ResponseInputItemUnionParam) {
				for i, item := range items {
					if item.OfFunctionCall == nil {
						t.Errorf("item %d: expected function call item", i)
					}
				}
			},
		},
		{
			name: "mixed text + tool calls",
			msg: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "I'll help you get the weather information.",
				ToolCalls: []provider.ToolCall{
					{
						ID: "call_123",
						Function: provider.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"city":"Shanghai"}`,
						},
					},
					{
						ID: "call_456",
						Function: provider.FunctionCall{
							Name:      "get_time",
							Arguments: `{"timezone":"Asia/Shanghai"}`,
						},
					},
				},
			},
			wantCount: 3,
			check: func(t *testing.T, items []responses.ResponseInputItemUnionParam) {
				// First item should be text message
				if items[0].OfMessage == nil {
					t.Error("first item should be message")
				}
				if items[0].OfMessage.Role != responses.EasyInputMessageRoleAssistant {
					t.Error("first item should have assistant role")
				}

				// Remaining items should be function calls
				for i := 1; i < len(items); i++ {
					if items[i].OfFunctionCall == nil {
						t.Errorf("item %d: expected function call", i)
					}
				}
			},
		},
		{
			name: "empty assistant message (no text, no tool calls)",
			msg: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "",
				ToolCalls: []provider.ToolCall{},
			},
			wantCount: 0,
		},
		{
			name: "empty tool calls slice",
			msg: provider.Message{
				Role:       provider.RoleAssistant,
				Content:    "Hello",
				ToolCalls:  []provider.ToolCall{},
			},
			wantCount: 1,
			check: func(t *testing.T, items []responses.ResponseInputItemUnionParam) {
				if items[0].OfMessage == nil {
					t.Error("expected message item")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.buildAssistantItem(tt.msg)
			if len(result) != tt.wantCount {
				t.Errorf("buildAssistantItem() returned %d items, want %d", len(result), tt.wantCount)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

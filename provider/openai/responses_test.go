package openai

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
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
			name:         "assistant text only",
			systemPrompt: "",
			messages: []provider.Message{
				{Role: provider.RoleAssistant, Content: "Hi there!"},
			},
			wantItems: 1,
			check: func(t *testing.T, input responses.ResponseNewParamsInputUnion) {
				if len(input.OfInputItemList) != 1 {
					t.Error("expected assistant message in input")
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
				if data, ok := events[0].Delta.Data.(provider.ToolCallData); !ok || data.ID != "fc_01ABC" || data.Name != "get_weather" {
					t.Errorf("expected ToolCallData{id: fc_01ABC, name: get_weather}, got %v", events[0].Delta.Data)
				}
			},
		},
		{
			name:     "response.output_item.added with message type returns nil",
			rawJSON:  `{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_01","role":"assistant","content":[],"status":"in_progress"}}`,
			wantLen:  0,
			wantType: "",
		},
		{
			name:     "response.output_text.delta",
			rawJSON:  `{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello world"}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeTextOutput {
					t.Errorf("expected text_output delta, got %v", events[0].Delta.Type)
				}
				if data, ok := events[0].Delta.Data.(provider.TextData); !ok || string(data) != "Hello world" {
					t.Errorf("expected TextData 'Hello world', got %v", events[0].Delta.Data)
				}
			},
		},
		{
			name:     "response.reasoning_text.delta",
			rawJSON:  `{"type":"response.reasoning_text.delta","output_index":0,"content_index":0,"delta":"Let me think..."}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeThinking {
					t.Errorf("expected thinking delta, got %v", events[0].Delta.Type)
				}
				if data, ok := events[0].Delta.Data.(provider.ThinkingData); !ok || string(data) != "Let me think..." {
					t.Errorf("expected ThinkingData 'Let me think...', got %v", events[0].Delta.Data)
				}
			},
		},
		{
			name:     "response.function_call_arguments.delta",
			rawJSON:  `{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_01","delta":"{\"city\": \"Tokyo\""}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeToolCallDelta {
					t.Errorf("expected tool_call_delta, got %v", events[0].Delta.Type)
				}
			},
		},
		{
			name:     "response.function_call_arguments.done returns nil",
			rawJSON:  `{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_01","arguments":"{\"city\": \"Tokyo\"}"}`,
			wantLen:  0,
			wantType: "",
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
				if data, ok := events[0].Delta.Data.(provider.UsageData); !ok || data.InputTokens != 100 || data.OutputTokens != 50 {
					t.Errorf("expected UsageData{100, 50}, got %v", events[0].Delta.Data)
				}
			},
		},
		{
			name:     "response.completed without usage returns nil",
			rawJSON:  `{"type":"response.completed","response":{"id":"resp_01","object":"response","created_at":1743000000,"status":"completed","model":"gpt-4o","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
			wantLen:  0,
			wantType: "",
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
			result := p.handleStreamEvent(event)
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
// buildAssistantItem 测试
// ==============================

func TestResponsesProvider_buildAssistantItem(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	tests := []struct {
		name    string
		msg     provider.Message
		wantNil bool
		check   func(t *testing.T, item responses.ResponseInputItemUnionParam)
	}{
		{
			name: "text only",
			msg: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "Hello!",
			},
			wantNil: false,
			check: func(t *testing.T, item responses.ResponseInputItemUnionParam) {
				if item.OfMessage == nil {
					t.Error("expected message in item")
				}
			},
		},
		{
			name: "tool call only",
			msg: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "",
				ToolCalls: []provider.ToolCall{
					{
						ID:   "call-123",
						Type: "function",
						Function: provider.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"city": "Tokyo"}`,
						},
					},
				},
			},
			wantNil: false,
			check: func(t *testing.T, item responses.ResponseInputItemUnionParam) {
				if item.OfFunctionCall == nil {
					t.Error("expected function call in item")
				}
			},
		},
		{
			name: "text and tool calls",
			msg: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "Let me check.",
				ToolCalls: []provider.ToolCall{
					{
						ID:   "call-456",
						Type: "function",
						Function: provider.FunctionCall{
							Name:      "search",
							Arguments: `{"q": "test"}`,
						},
					},
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.buildAssistantItem(tt.msg)
			if tt.wantNil && result.OfMessage != nil && result.OfFunctionCall != nil {
				t.Error("buildAssistantItem() expected nil result")
			}
			if !tt.wantNil && tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

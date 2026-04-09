package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// unmarshalEvent 将 JSON 字符串反序列化为 Anthropic 流事件 Union 类型。
func unmarshalEvent(t *testing.T, rawJSON string) anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	var event anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
		t.Fatalf("failed to unmarshal event JSON: %v", err)
	}
	return event
}

// ==============================
// 构造函数测试
// ==============================

func TestNewProvider(t *testing.T) {
	p := NewProvider("test-api-key")
	if p == nil {
		t.Fatal("NewProvider() returned nil")
	}
	_ = p.GetAvailableModels()
}

// ==============================
// GetProviderType 测试
// ==============================

func TestProvider_GetProviderType(t *testing.T) {
	p := NewProvider("test-api-key")
	want := provider.ProviderAnthropic
	if got := p.GetProviderType(); got != want {
		t.Errorf("GetProviderType() = %v, want %v", got, want)
	}
}

// ==============================
// GetAvailableModels 测试
// ==============================

func TestProvider_GetAvailableModels(t *testing.T) {
	p := NewProvider("test-api-key")
	models := p.GetAvailableModels()

	if len(models) == 0 {
		t.Error("GetAvailableModels() returned empty list")
	}

	expectedModels := []string{
		"claude-sonnet-4-20250514",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
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

func TestProvider_buildMessages(t *testing.T) {
	p := NewProvider("test-api-key")

	tests := []struct {
		name     string
		messages []provider.Message
		wantLen  int
		check    func(t *testing.T, result []anthropic.BetaMessageParam)
	}{
		{
			name:     "empty messages",
			messages: []provider.Message{},
			wantLen:  0,
		},
		{
			name: "user message",
			messages: []provider.Message{
				{Role: provider.RoleUser, Content: "Hello"},
			},
			wantLen: 1,
			check: func(t *testing.T, result []anthropic.BetaMessageParam) {
				if result[0].Role != anthropic.BetaMessageParamRoleUser {
					t.Errorf("expected role user, got %v", result[0].Role)
				}
			},
		},
		{
			name: "assistant text only",
			messages: []provider.Message{
				{Role: provider.RoleAssistant, Content: "Hi there!"},
			},
			wantLen: 1,
			check: func(t *testing.T, result []anthropic.BetaMessageParam) {
				if result[0].Role != anthropic.BetaMessageParamRoleAssistant {
					t.Errorf("expected role assistant, got %v", result[0].Role)
				}
			},
		},
		{
			name: "assistant with tool calls",
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
			check: func(t *testing.T, result []anthropic.BetaMessageParam) {
				if result[0].Role != anthropic.BetaMessageParamRoleAssistant {
					t.Errorf("expected role assistant, got %v", result[0].Role)
				}
				if len(result[0].Content) < 2 {
					t.Errorf("expected at least 2 content blocks, got %d", len(result[0].Content))
				}
			},
		},
		{
			name: "assistant tool calls only no text",
			messages: []provider.Message{
				{
					Role:    provider.RoleAssistant,
					Content: "",
					ToolCalls: []provider.ToolCall{
						{
							ID:   "call-456",
							Type: "function",
							Function: provider.FunctionCall{
								Name:      "search",
								Arguments: `{"query": "test"}`,
							},
						},
					},
				},
			},
			wantLen: 1,
			check: func(t *testing.T, result []anthropic.BetaMessageParam) {
				if len(result[0].Content) != 1 {
					t.Errorf("expected 1 content block (tool only), got %d", len(result[0].Content))
				}
			},
		},
		{
			name: "tool response message",
			messages: []provider.Message{
				{
					Role:       provider.RoleTool,
					Content:    `{"temperature": 25}`,
					ToolCallID: "call-123",
				},
			},
			wantLen: 1,
			check: func(t *testing.T, result []anthropic.BetaMessageParam) {
				if result[0].Role != anthropic.BetaMessageParamRoleUser {
					t.Errorf("expected role user for tool result, got %v", result[0].Role)
				}
			},
		},
		{
			name: "mixed message sequence",
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
			wantLen: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.buildMessages(tt.messages)
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
// handleStreamEvent 测试
// ==============================

func TestProvider_handleStreamEvent(t *testing.T) {
	p := NewProvider("test-api-key")

	tests := []struct {
		name     string
		rawJSON  string
		wantLen  int
		wantType provider.StreamType
		check    func(t *testing.T, events []provider.StreamEvent)
	}{
		{
			name:     "message_start returns nil",
			rawJSON:  `{"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
			wantLen:  0,
			wantType: "",
		},
		{
			name:     "content_block_start with text returns nil",
			rawJSON:  `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			wantLen:  0,
			wantType: "",
		},
		{
			name:     "content_block_start with thinking",
			rawJSON:  `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"Let me think..."}}`,
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
			name:     "content_block_start with tool_use",
			rawJSON:  `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01ABC","name":"get_weather"}}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeToolCall {
					t.Errorf("expected tool_call delta, got %v", events[0].Delta.Type)
				}
				if data, ok := events[0].Delta.Data.(provider.ToolCallData); !ok || data.ID != "toolu_01ABC" || data.Name != "get_weather" {
					t.Errorf("expected ToolCallData{id: toolu_01ABC, name: get_weather}, got %v", events[0].Delta.Data)
				}
			},
		},
		{
			name:     "content_block_delta with text_delta",
			rawJSON:  `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello world"}}`,
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
			name:     "content_block_delta with thinking_delta",
			rawJSON:  `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm..."}}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeThinking {
					t.Errorf("expected thinking delta, got %v", events[0].Delta.Type)
				}
			},
		},
		{
			name:     "content_block_delta with input_json_delta",
			rawJSON:  `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeToolCallDelta {
					t.Errorf("expected tool_call_delta, got %v", events[0].Delta.Type)
				}
				if data, ok := events[0].Delta.Data.(provider.ToolCallDeltaData); !ok || string(data) != `{"city":` {
					t.Errorf("expected ToolCallDeltaData '{\"city\":', got %v", events[0].Delta.Data)
				}
			},
		},
		{
			name:     "content_block_stop returns nil",
			rawJSON:  `{"type":"content_block_stop","index":0}`,
			wantLen:  0,
			wantType: "",
		},
		{
			name:     "message_delta with usage",
			rawJSON:  `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":42}}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeUsage {
					t.Errorf("expected usage delta, got %v", events[0].Delta.Type)
				}
			},
		},
		{
			name:     "message_delta without usage returns nil",
			rawJSON:  `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":0}}`,
			wantLen:  0,
			wantType: "",
		},
		{
			name:     "message_stop returns stop",
			rawJSON:  `{"type":"message_stop"}`,
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
			event := unmarshalEvent(t, tt.rawJSON)
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
// mapFinishReason 测试
// ==============================

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		name   string
		reason anthropic.BetaStopReason
		want   provider.FinishReason
	}{
		{"end_turn", anthropic.BetaStopReasonEndTurn, provider.FinishReasonStop},
		{"max_tokens", anthropic.BetaStopReasonMaxTokens, provider.FinishReasonLength},
		{"tool_use", anthropic.BetaStopReasonToolUse, provider.FinishReasonToolCalls},
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

func TestNewProviderWithOptions(t *testing.T) {
	t.Run("仅 WithAPIKey", func(t *testing.T) {
		p := NewProviderWithOptions(WithAPIKey("test-key"))
		if p == nil {
			t.Fatal("NewProviderWithOptions() returned nil")
		}
		_ = p.GetAvailableModels()
	})

	t.Run("WithAPIKey + WithBaseURL", func(t *testing.T) {
		p := NewProviderWithOptions(
			WithAPIKey("test-key"),
			WithBaseURL("https://custom.example.com"),
		)
		if p == nil {
			t.Fatal("NewProviderWithOptions with BaseURL returned nil")
		}
		_ = p.GetAvailableModels()
	})

	t.Run("完整选项", func(t *testing.T) {
		p := NewProviderWithOptions(
			WithAPIKey("test-key"),
			WithBaseURL("https://custom.example.com"),
			WithHeader("X-Custom", "test-value"),
		)
		if p == nil {
			t.Fatal("NewProviderWithOptions with full options returned nil")
		}
		_ = p.GetAvailableModels()
	})
}

func TestNewProvider_BackwardCompatible(t *testing.T) {
	p := NewProvider("test-api-key")
	if p == nil {
		t.Fatal("NewProvider(string) returned nil")
	}
	if got := p.GetProviderType(); got != provider.ProviderAnthropic {
		t.Errorf("GetProviderType() = %v, want %v", got, provider.ProviderAnthropic)
	}
}

func TestNewProviderWithOptions_EmptyOptions(t *testing.T) {
	p := NewProviderWithOptions()
	if p == nil {
		t.Fatal("NewProviderWithOptions() with no args returned nil")
	}
}

func TestWithHeader_MultipleHeaders(t *testing.T) {
	p := NewProviderWithOptions(
		WithAPIKey("test-key"),
		WithHeader("X-Header-1", "value-1"),
		WithHeader("X-Header-2", "value-2"),
	)
	if p == nil {
		t.Fatal("failed with multiple headers")
	}
	_ = p.GetAvailableModels()
}

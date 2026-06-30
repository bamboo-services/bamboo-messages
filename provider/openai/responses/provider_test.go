package responses

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// unmarshalResponseEvent 将 JSON 字符串反序列化为本地 responseStreamEvent 类型。
//
// 去 SDK 化后不再使用 responses.ResponseStreamEventUnion，
// 直接解析到 types.go 中定义的 responseStreamEvent 结构体。
func unmarshalResponseEvent(t *testing.T, rawJSON string) responseStreamEvent {
	t.Helper()
	var event responseStreamEvent
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
// buildInput 测试
// ==============================

func TestResponsesProvider_buildInput(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	tests := []struct {
		name      string
		messages  []provider.Message
		wantItems int
	}{
		{
			name:      "empty messages",
			messages:  []provider.Message{},
			wantItems: 0,
		},
		{
			name: "user message",
			messages: []provider.Message{
				{Role: provider.RoleUser, Content: "Hello"},
			},
			wantItems: 1,
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
			wantItems: 1,
		},
		{
			name: "mixed message sequence",
			messages: []provider.Message{
				{Role: provider.RoleUser, Content: "What's the weather?"},
				{Role: provider.RoleAssistant, Content: "It's sunny!"},
			},
			wantItems: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.buildInput(tt.messages)
			if len(result) != tt.wantItems {
				t.Errorf("buildInput() returned %d items, want %d", len(result), tt.wantItems)
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
			name:     "response.created returns metadata delta with response ID",
			rawJSON:  `{"type":"response.created","response":{"id":"resp_01","object":"response","created_at":1743000000,"status":"in_progress","model":"gpt-4o","output":[]}}`,
			wantLen:  1,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				if events[0].Delta.Type != provider.StreamDeltaTypeMetadata {
					t.Errorf("expected metadata delta, got %v", events[0].Delta.Type)
				}
				data, ok := events[0].Delta.Data.(provider.MetadataData)
				if !ok {
					t.Fatalf("expected MetadataData, got %T", events[0].Delta.Data)
				}
				if data.ResponseID != "resp_01" {
					t.Errorf("ResponseID = %q, want resp_01", data.ResponseID)
				}
			},
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
			rawJSON:  `{"type":"response.output_text.delta","output_index":0,"content_index":0,"text":"Hello world"}`,
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
			wantLen:  2,
			wantType: provider.StreamTypeDelta,
			check: func(t *testing.T, events []provider.StreamEvent) {
				// 第一个事件为 UsageDelta
				if events[0].Delta.Type != provider.StreamDeltaTypeUsage {
					t.Errorf("expected usage delta, got %v", events[0].Delta.Type)
				}
				// 第二个事件为 StreamTypeStop
				if events[1].Type != provider.StreamTypeStop {
					t.Errorf("expected stop event, got %v", events[1].Type)
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
			thinkingBlockStarted := false
			result := p.handleStreamEvent(context.Background(), event, &textBlockStarted, &thinkingBlockStarted)
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
		check     func(t *testing.T, items []map[string]any)
	}{
		{
			name: "text-only assistant message",
			msg: provider.Message{
				Role:    provider.RoleAssistant,
				Content: "Hello, how can I help you?",
			},
			wantCount: 1,
			check: func(t *testing.T, items []map[string]any) {
				if items[0]["role"] != "assistant" {
					t.Errorf("expected assistant role, got %v", items[0]["role"])
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
			check: func(t *testing.T, items []map[string]any) {
				if items[0]["type"] != "function_call" {
					t.Errorf("expected function_call type, got %v", items[0]["type"])
				}
				if items[0]["name"] != "get_weather" {
					t.Errorf("expected function name 'get_weather', got '%v'", items[0]["name"])
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
			check: func(t *testing.T, items []map[string]any) {
				for i, item := range items {
					if item["type"] != "function_call" {
						t.Errorf("item %d: expected function_call type", i)
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
			check: func(t *testing.T, items []map[string]any) {
				// First item should be text message
				if items[0]["role"] != "assistant" {
					t.Error("first item should be message with assistant role")
				}
				// Remaining items should be function calls
				for i := 1; i < len(items); i++ {
					if items[i]["type"] != "function_call" {
						t.Errorf("item %d: expected function_call", i)
					}
				}
			},
		},
		{
			name: "empty assistant message (no text, no tool calls)",
			msg: provider.Message{
				Role:      provider.RoleAssistant,
				Content:   "",
				ToolCalls: []provider.ToolCall{},
			},
			wantCount: 0,
		},
		{
			name: "empty tool calls slice",
			msg: provider.Message{
				Role:      provider.RoleAssistant,
				Content:   "Hello",
				ToolCalls: []provider.ToolCall{},
			},
			wantCount: 1,
			check: func(t *testing.T, items []map[string]any) {
				if items[0]["role"] != "assistant" {
					t.Error("expected message item with assistant role")
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

// ==============================
// buildParams 参数映射测试
// ==============================

// testBuildParams 辅助函数，用空输入构建参数 map。
func testBuildParams(p *ResponsesProvider, config *provider.ChatConfig) map[string]any {
	return p.buildParams("gpt-4o", "", nil, config, false)
}

// TestBuildParams_StopMapping 验证 Stop 参数正确映射到 params["stop"]。
func TestBuildParams_StopMapping(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	config := &provider.ChatConfig{
		Stop: []string{"STOP", "END"},
	}

	params := testBuildParams(p, config)

	stopVal, ok := params["stop"]
	if !ok {
		t.Fatal("params 中应包含 'stop' 字段")
	}
	stopSlice, ok := stopVal.([]string)
	if !ok {
		t.Fatalf("'stop' 应为 []string 类型, 实际为 %T", stopVal)
	}
	if len(stopSlice) != 2 || stopSlice[0] != "STOP" || stopSlice[1] != "END" {
		t.Errorf("stop = %v, want [STOP END]", stopSlice)
	}
}

// TestBuildParams_User 验证 UserID 参数正确映射到 params["user"]。
func TestBuildParams_User(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	config := &provider.ChatConfig{
		UserID: "test-user",
	}

	params := testBuildParams(p, config)

	userVal, ok := params["user"]
	if !ok {
		t.Fatal("params 中应包含 'user' 字段")
	}
	if userVal != "test-user" {
		t.Errorf("params['user'] = %v, want %q", userVal, "test-user")
	}
}

// TestBuildParams_Store 验证 Store 参数从 ProviderExtra 正确映射到 params["store"]。
func TestBuildParams_Store(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{
			"store": true,
		},
	}

	params := testBuildParams(p, config)

	storeVal, ok := params["store"]
	if !ok {
		t.Fatal("params 中应包含 'store' 字段")
	}
	if storeVal != true {
		t.Errorf("params['store'] = %v, want true", storeVal)
	}
}

// TestBuildParams_Modalities 验证 Modalities 参数从 ProviderExtra 正确传递。
func TestBuildParams_Modalities(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	modalities := []string{"text", "audio"}
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{
			"modalities": modalities,
		},
	}

	params := testBuildParams(p, config)

	m, ok := params["modalities"]
	if !ok {
		t.Fatal("params 中应包含 'modalities' 字段")
	}
	arr, ok := m.([]string)
	if !ok {
		t.Fatalf("'modalities' 应为 []string 类型, 实际为 %T", m)
	}
	if len(arr) != 2 {
		t.Errorf("modalities 长度 = %d, want 2", len(arr))
	}
}

// TestBuildParams_Truncation 验证 Truncation 参数从 ProviderExtra 正确映射。
func TestBuildParams_Truncation(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{
			"truncation": "auto",
		},
	}

	params := testBuildParams(p, config)

	truncationVal, ok := params["truncation"]
	if !ok {
		t.Fatal("params 中应包含 'truncation' 字段")
	}
	if truncationVal != "auto" {
		t.Errorf("params['truncation'] = %v, want %q", truncationVal, "auto")
	}
}

// TestBuildParams_PreviousResponseID 验证 PreviousResponseID 参数从 ProviderExtra 正确映射。
func TestBuildParams_PreviousResponseID(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{
			"previous_response_id": "resp-123",
		},
	}

	params := testBuildParams(p, config)

	prevID, ok := params["previous_response_id"]
	if !ok {
		t.Fatal("params 中应包含 'previous_response_id' 字段")
	}
	if prevID != "resp-123" {
		t.Errorf("params['previous_response_id'] = %v, want %q", prevID, "resp-123")
	}
}

// TestBuildParams_InstructionsNativeField 验证 instructions 从 ProviderExtra 回退。
func TestBuildParams_InstructionsNativeField(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	config := &provider.ChatConfig{
		ProviderExtra: map[string]any{
			"instructions":         "Use native instructions.",
			"previous_response_id": "resp-123",
		},
	}

	params := testBuildParams(p, config)

	ins, ok := params["instructions"]
	if !ok {
		t.Fatal("params 中应包含 'instructions' 字段")
	}
	if ins != "Use native instructions." {
		t.Errorf("params['instructions'] = %v", ins)
	}
	prevID, ok := params["previous_response_id"]
	if !ok || prevID != "resp-123" {
		t.Fatalf("params['previous_response_id'] = %v", prevID)
	}
}

// TestBuildParams_ParallelToolCallsNativeField 验证 ParallelToolCalls 映射。
func TestBuildParams_ParallelToolCallsNativeField(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	params := testBuildParams(p, &provider.ChatConfig{ParallelToolCalls: true})

	ptc, ok := params["parallel_tool_calls"]
	if !ok {
		t.Fatal("params 中应包含 'parallel_tool_calls' 字段")
	}
	if ptc != true {
		t.Errorf("params['parallel_tool_calls'] = %v, want true", ptc)
	}
}

// TestBuildParams_Metadata 验证 Metadata 参数正确映射。
func TestBuildParams_Metadata(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	config := &provider.ChatConfig{
		Metadata: map[string]string{
			"key": "val",
		},
	}

	params := testBuildParams(p, config)

	meta, ok := params["metadata"]
	if !ok {
		t.Fatal("params 中应包含 'metadata' 字段")
	}
	metaMap, ok := meta.(map[string]any)
	if !ok {
		t.Fatalf("'metadata' 应为 map[string]any 类型, 实际为 %T", meta)
	}
	if metaMap["key"] != "val" {
		t.Errorf("params['metadata']['key'] = %v, want %q", metaMap["key"], "val")
	}
}

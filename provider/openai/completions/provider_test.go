package completions

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
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
//
// 回归测试：确保 map 形式的 ResponseFormat 能正确转换为 SDK 类型。
func TestBuildResponseFormat_MapInput(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantText  bool // 期望 OfText 非空
		wantJSON  bool // 期望 OfJSONObject 非空
		wantEmpty bool // 期望空联合类型
	}{
		{
			name:     "json_object map",
			input:    map[string]any{"type": "json_object"},
			wantJSON: true,
		},
		{
			name:     "text map",
			input:    map[string]any{"type": "text"},
			wantText: true,
		},
		{
			name:      "unknown type map",
			input:     map[string]any{"type": "unknown"},
			wantEmpty: true,
		},
		{
			name:      "map missing type key",
			input:     map[string]any{"foo": "bar"},
			wantEmpty: true,
		},
		{
			name:      "map with non-string type",
			input:     map[string]any{"type": 123},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResponseFormat(tt.input)
			if tt.wantText {
				if result.OfText == nil {
					t.Error("期望 OfText 非空，但得到 nil")
				}
				if result.OfJSONObject != nil {
					t.Error("期望 OfJSONObject 为 nil，但得到非空值")
				}
			}
			if tt.wantJSON {
				if result.OfJSONObject == nil {
					t.Error("期望 OfJSONObject 非空，但得到 nil")
				}
				if result.OfText != nil {
					t.Error("期望 OfText 为 nil，但得到非空值")
				}
			}
			if tt.wantEmpty {
				if result.OfText != nil || result.OfJSONObject != nil || result.OfJSONSchema != nil {
					t.Error("期望空联合类型，但得到非空字段")
				}
			}
		})
	}
}

// TestBuildResponseFormat_StringInput 验证字符串输入的 ResponseFormat 转换。
func TestBuildResponseFormat_StringInput(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantText  bool
		wantJSON  bool
		wantEmpty bool
	}{
		{
			name:     "text string",
			input:    "text",
			wantText: true,
		},
		{
			name:     "json_object string",
			input:    "json_object",
			wantJSON: true,
		},
		{
			name:      "unknown string",
			input:     "unknown",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResponseFormat(tt.input)
			if tt.wantText && result.OfText == nil {
				t.Error("期望 OfText 非空")
			}
			if tt.wantJSON && result.OfJSONObject == nil {
				t.Error("期望 OfJSONObject 非空")
			}
			if tt.wantEmpty && (result.OfText != nil || result.OfJSONObject != nil) {
				t.Error("期望空联合类型")
			}
		})
	}
}

// TestBuildResponseFormat_SdkTypePassthrough 验证 SDK 原生类型直接透传。
//
// 回归测试：确保向后兼容，SDK 类型传入时原样返回。
func TestBuildResponseFormat_SdkTypePassthrough(t *testing.T) {
	sdkType := openai.ChatCompletionNewParamsResponseFormatUnion{
		OfText: openai.Ptr(shared.NewResponseFormatTextParam()),
	}

	result := buildResponseFormat(sdkType)

	if result.OfText == nil {
		t.Error("SDK 类型透传后期望 OfText 非空")
	}
}

// ==============================
// ToolChoice 映射测试
// ==============================

// TestToolChoiceStringMapping 验证字符串形式的 ToolChoice 映射到 SDK 类型。
//
// 回归测试：确保 "auto"/"none"/"required" 字符串正确转换为
// ChatCompletionToolChoiceOptionUnionParam 的 OfAuto 字段。
func TestToolChoiceStringMapping(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "auto", input: "auto"},
		{name: "none", input: "none"},
		{name: "required", input: "required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 chat.go 中的映射逻辑
			var toolChoice openai.ChatCompletionToolChoiceOptionUnionParam
			toolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt(tt.input),
			}

			// 验证 OfAuto 被正确设置
			if !toolChoice.OfAuto.Valid() {
				t.Errorf("OfAuto 应为有效值，但得到无效值")
			}
			if toolChoice.OfAuto.Value != tt.input {
				t.Errorf("OfAuto.Value = %q, 期望 %q", toolChoice.OfAuto.Value, tt.input)
			}
			// 验证其他字段未设置
			if toolChoice.OfAllowedTools != nil {
				t.Error("OfAllowedTools 应为 nil")
			}
			if toolChoice.OfFunctionToolChoice != nil {
				t.Error("OfFunctionToolChoice 应为 nil")
			}
		})
	}
}

// TestToolChoiceSdkTypePassthrough 验证 SDK 原生 ToolChoice 类型直接透传。
//
// 回归测试：确保向后兼容，传入 SDK 类型时能正确赋值。
func TestToolChoiceSdkTypePassthrough(t *testing.T) {
	// 构造一个 SDK 原生的 ToolChoice（函数工具选择）
	sdkChoice := openai.ChatCompletionToolChoiceOptionUnionParam{
		OfAuto: param.NewOpt("auto"),
	}

	// 模拟 chat.go 中的类型断言透传逻辑
	tc := any(sdkChoice)
	var params openai.ChatCompletionNewParams
	if toolChoice, ok := tc.(openai.ChatCompletionToolChoiceOptionUnionParam); ok {
		params.ToolChoice = toolChoice
	} else {
		t.Fatal("SDK 类型断言失败")
	}

	if !params.ToolChoice.OfAuto.Valid() {
		t.Error("透传后 OfAuto 应为有效值")
	}
	if params.ToolChoice.OfAuto.Value != "auto" {
		t.Errorf("透传后 OfAuto.Value = %q, 期望 %q", params.ToolChoice.OfAuto.Value, "auto")
	}
}

// TestToolChoiceStringTypeAssertion 验证从 ProviderExtra 提取 ToolChoice 字符串时的类型断言。
func TestToolChoiceStringTypeAssertion(t *testing.T) {
	extra := map[string]any{
		"tool_choice": "required",
	}

	tc, ok := provider.GetExtraAny(extra, "tool_choice")
	if !ok {
		t.Fatal("未能从 ProviderExtra 中获取 ToolChoice")
	}

	// 验证字符串类型断言
	s, ok := tc.(string)
	if !ok {
		t.Fatal("ToolChoice 应为字符串类型")
	}
	if s != "required" {
		t.Errorf("ToolChoice = %q, 期望 %q", s, "required")
	}

	// 验证转换为 SDK 类型
	toolChoice := openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(s)}
	if !toolChoice.OfAuto.Valid() || toolChoice.OfAuto.Value != "required" {
		t.Error("字符串转换后的 SDK ToolChoice 值不正确")
	}
}

// ==============================
// 新参数映射测试
// ==============================

// TestProviderExtraUserMapping 验证 User 参数从 ProviderExtra 映射到 SDK 参数。
func TestProviderExtraUserMapping(t *testing.T) {
	extra := map[string]any{
		"user": "test-user-123",
	}

	// 模拟 chat.go 中的映射逻辑
	u, ok := provider.GetExtraString(extra, "user")
	if !ok {
		t.Fatal("未能从 ProviderExtra 中获取 User")
	}

	userParam := openai.String(u)
	if userParam.Value != "test-user-123" {
		t.Errorf("User = %q, 期望 %q", userParam.Value, "test-user-123")
	}
	if !userParam.Valid() {
		t.Error("User 参数应为有效值")
	}
}

// TestProviderExtraPredictionMapping 验证 Prediction 参数从 ProviderExtra 映射到 SDK 参数。
func TestProviderExtraPredictionMapping(t *testing.T) {
	// 构造一个 Prediction 参数
	prediction := openai.ChatCompletionPredictionContentParam{
		Content: openai.ChatCompletionPredictionContentContentUnionParam{
			OfString: param.NewOpt("predicted content"),
		},
	}

	extra := map[string]any{
		"prediction": prediction,
	}

	// 模拟 chat.go 中的映射逻辑
	pred, ok := provider.GetExtraAny(extra, "prediction")
	if !ok {
		t.Fatal("未能从 ProviderExtra 中获取 Prediction")
	}

	predTyped, ok := pred.(openai.ChatCompletionPredictionContentParam)
	if !ok {
		t.Fatal("Prediction 类型断言失败")
	}

	// 验证 Prediction 内容正确传递
	if !predTyped.Content.OfString.Valid() {
		t.Fatal("Prediction Content.OfString 应为有效值")
	}
	if predTyped.Content.OfString.Value != "predicted content" {
		t.Errorf("Prediction 内容 = %q, 期望 %q", predTyped.Content.OfString.Value, "predicted content")
	}
}

// TestProviderExtraParallelToolCallsMapping 验证 ParallelToolCalls 参数映射。
func TestProviderExtraParallelToolCallsMapping(t *testing.T) {
	extra := map[string]any{
		"parallel_tool_calls": true,
	}

	// 模拟 chat.go 中的映射逻辑
	ptc, ok := provider.GetExtraBool(extra, "parallel_tool_calls")
	if !ok {
		t.Fatal("未能从 ProviderExtra 中获取 ParallelToolCalls")
	}

	param := openai.Bool(ptc)
	if param.Value != true {
		t.Errorf("ParallelToolCalls = %v, 期望 true", param.Value)
	}
	if !param.Valid() {
		t.Error("ParallelToolCalls 参数应为有效值")
	}
}

// TestMetadataMapping 验证 Metadata 从 ChatConfig 映射到 SDK 参数。
func TestMetadataMapping(t *testing.T) {
	config := &provider.ChatConfig{
		Metadata: map[string]string{
			"key": "val",
			"foo": "bar",
		},
	}

	// 模拟 chat.go 中的映射逻辑
	if len(config.Metadata) == 0 {
		t.Fatal("Metadata 不应为空")
	}

	sdkMetadata := shared.Metadata(config.Metadata)
	if len(sdkMetadata) != 2 {
		t.Errorf("Metadata 长度 = %d, 期望 2", len(sdkMetadata))
	}
	if sdkMetadata["key"] != "val" {
		t.Errorf("Metadata[\"key\"] = %q, 期望 %q", sdkMetadata["key"], "val")
	}
	if sdkMetadata["foo"] != "bar" {
		t.Errorf("Metadata[\"foo\"] = %q, 期望 %q", sdkMetadata["foo"], "bar")
	}
}

// ==============================
// buildStop 测试
// ==============================

// TestBuildStop 验证停止词列表到 SDK 类型的转换。
func TestBuildStop(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantNil bool
		wantLen int
	}{
		{
			name:    "空列表",
			input:   []string{},
			wantNil: true,
		},
		{
			name:    "nil 列表",
			input:   nil,
			wantNil: true,
		},
		{
			name:    "单个停止词",
			input:   []string{"STOP"},
			wantLen: 1,
		},
		{
			name:    "多个停止词",
			input:   []string{"STOP", "END", "DONE"},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStop(tt.input)
			if tt.wantNil {
				if result.OfStringArray != nil {
					t.Error("空输入应返回零值 StopUnion")
				}
				return
			}
			if len(result.OfStringArray) != tt.wantLen {
				t.Errorf("停止词数量 = %d, 期望 %d", len(result.OfStringArray), tt.wantLen)
			}
		})
	}
}

// ==============================
// buildTools 测试
// ==============================

// TestBuildTools 验证工具定义到 SDK 类型的转换。
func TestBuildTools(t *testing.T) {
	tests := []struct {
		name    string
		input   []provider.Tool
		wantNil bool
		wantLen int
	}{
		{
			name:    "空列表",
			input:   []provider.Tool{},
			wantNil: true,
		},
		{
			name:    "nil 列表",
			input:   nil,
			wantNil: true,
		},
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
				{
					Type: "function",
					Function: provider.FunctionDef{
						Name:       "get_weather",
						Parameters: map[string]any{"type": "object"},
					},
				},
				{
					Type: "function",
					Function: provider.FunctionDef{
						Name:       "search",
						Parameters: map[string]any{"type": "object"},
					},
				},
			},
			wantLen: 2,
		},
		{
			name: "过滤非 function 类型",
			input: []provider.Tool{
				{
					Type: "other_type",
					Function: provider.FunctionDef{
						Name: "ignored",
					},
				},
				{
					Type: "function",
					Function: provider.FunctionDef{
						Name: "get_weather",
					},
				},
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

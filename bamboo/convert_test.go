package bamboo

import (
	"encoding/json"
	"strings"
	"testing"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ---- messagesToProvider 测试 ----

func TestConvertTextMessage(t *testing.T) {
	msgs := []BambooMessage{NewUserMessage("hello")}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != "hello" {
		t.Errorf("Content = %q, want %q", result[0].Content, "hello")
	}
	if result[0].Role != provider.RoleUser {
		t.Errorf("Role = %q, want %q", result[0].Role, provider.RoleUser)
	}
}

func TestConvertMultiBlockMessage(t *testing.T) {
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewTextBlock("hello"),
			NewThinkingBlock("let me think", "sig123"),
			NewTextBlock(" world"),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// thinking 被保留到 ThinkingContent，两个 text 拼接
	if result[0].Content != "hello world" {
		t.Errorf("Content = %q, want %q", result[0].Content, "hello world")
	}
	if result[0].ThinkingContent != "let me think" {
		t.Errorf("ThinkingContent = %q, want %q", result[0].ThinkingContent, "let me think")
	}
	if result[0].ThinkingSignature != "sig123" {
		t.Errorf("ThinkingSignature = %q, want %q", result[0].ThinkingSignature, "sig123")
	}
	if result[0].Role != provider.RoleAssistant {
		t.Errorf("Role = %q, want %q", result[0].Role, provider.RoleAssistant)
	}
}

func TestConvertToolUseMessage(t *testing.T) {
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewTextBlock("calling tool"),
			NewToolUseBlock("call_123", "get_weather", map[string]any{"city": "Tokyo"}),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != "calling tool" {
		t.Errorf("Content = %q, want %q", result[0].Content, "calling tool")
	}
	if len(result[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[0].ToolCalls))
	}
	tc := result[0].ToolCalls[0]
	if tc.ID != "call_123" {
		t.Errorf("ToolCall.ID = %q, want %q", tc.ID, "call_123")
	}
	if tc.Type != "function" {
		t.Errorf("ToolCall.Type = %q, want %q", tc.Type, "function")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("Function.Name = %q, want %q", tc.Function.Name, "get_weather")
	}
	// Arguments 应为 JSON 字符串
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("Arguments is not valid JSON: %v", err)
	}
	if args["city"] != "Tokyo" {
		t.Errorf("Arguments.city = %v, want Tokyo", args["city"])
	}
}

func TestConvertToolResultMessage(t *testing.T) {
	msgs := []BambooMessage{
		NewUserMessageBlocks(
			NewToolResultBlock("call_123", "sunny, 25°C", false),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message (tool_result split), got %d", len(result))
	}
	if result[0].Role != provider.RoleTool {
		t.Errorf("Role = %q, want %q", result[0].Role, provider.RoleTool)
	}
	if result[0].Content != "sunny, 25°C" {
		t.Errorf("Content = %q, want %q", result[0].Content, "sunny, 25°C")
	}
	if result[0].ToolCallID != "call_123" {
		t.Errorf("ToolCallID = %q, want %q", result[0].ToolCallID, "call_123")
	}
}

func TestConvertUnsupportedImage(t *testing.T) {
	msgs := []BambooMessage{
		NewUserMessageBlocks(
			NewTextBlock("look at this"),
			NewImageBlock(ContentSource{Type: "base64", MediaType: "image/png", Data: "abc123"}),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != "look at this" {
		t.Errorf("Content = %q, want %q", result[0].Content, "look at this")
	}
}

func TestConvertDocument(t *testing.T) {
	msgs := []BambooMessage{
		NewUserMessageBlocks(
			NewDocumentBlock(ContentSource{Type: "base64", MediaType: "application/pdf", Data: "abc123"}),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message with document, got %d", len(result))
	}
	if len(result[0].ContentBlocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result[0].ContentBlocks))
	}
	doc, ok := result[0].ContentBlocks[0].(provider.DocumentContentBlock)
	if !ok {
		t.Fatal("expected DocumentContentBlock")
	}
	if doc.Source.Type != "base64" {
		t.Errorf("Source.Type = %q, want base64", doc.Source.Type)
	}
}

func TestConvertEmptyContent(t *testing.T) {
	msgs := []BambooMessage{
		{Role: RoleUser, Content: []ContentBlock{}},
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息, 实际 %d", len(result))
	}
	if result[0].Content != "" {
		t.Errorf("Content = %q, 期望空字符串", result[0].Content)
	}
}

// TestConvertEmptyAssistantContent 验证 assistant 空内容时自动补 "-"。
func TestConvertEmptyAssistantContent(t *testing.T) {
	msgs := []BambooMessage{
		{Role: RoleAssistant, Content: []ContentBlock{}},
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息, 实际 %d", len(result))
	}
	if result[0].Content != "-" {
		t.Errorf("Content = %q, 期望 \"-\"", result[0].Content)
	}
}

// ---- configToProvider 测试 ----

func TestConvertConfig(t *testing.T) {
	temp := 0.7
	topP := 0.9
	cfg := &RequestConfig{
		Model:         "claude-sonnet-4-20250514",
		MaxTokens:     1024,
		Temperature:   &temp,
		TopP:          &topP,
		StopSequences: []string{"STOP"},
		Tools: []Tool{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"city": {"type": "string", "description": "City name"}
				},
				"required": ["city"]
			}`),
			},
		},
		Metadata: map[string]string{"key": "value"},
	}

	result := configToProvider(cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", result.Model, "claude-sonnet-4-20250514")
	}
	if result.MaxTokens != 1024 {
		t.Errorf("MaxTokens = %d, want 1024", result.MaxTokens)
	}
	if result.Temperature == nil || *result.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", result.Temperature)
	}
	if result.TopP == nil || *result.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", result.TopP)
	}
	if len(result.Stop) != 1 || result.Stop[0] != "STOP" {
		t.Errorf("Stop = %v, want [STOP]", result.Stop)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(result.Tools))
	}
	if result.Tools[0].Function.Name != "get_weather" {
		t.Errorf("Tool name = %q, want get_weather", result.Tools[0].Function.Name)
	}
	if result.Metadata["key"] != "value" {
		t.Errorf("Metadata = %v, want key=value", result.Metadata)
	}
}

func TestConvertConfigNil(t *testing.T) {
	result := configToProvider(nil)
	if result != nil {
		t.Errorf("expected nil for nil config, got %v", result)
	}
}

func TestConvertConfigNilOptionals(t *testing.T) {
	cfg := &RequestConfig{
		Model:     "test-model",
		MaxTokens: 512,
	}
	result := configToProvider(cfg)
	if result.Temperature != nil {
		t.Errorf("Temperature = %v, want nil", result.Temperature)
	}
	if result.TopP != nil {
		t.Errorf("TopP = %v, want nil", result.TopP)
	}
	if result.Stop != nil {
		t.Errorf("Stop = %v, want nil", result.Stop)
	}
	if result.Tools != nil {
		t.Errorf("Tools = %v, want nil", result.Tools)
	}
}

// ---- toolsToProvider 测试 ----

func TestConvertTools(t *testing.T) {
	tools := []Tool{
		{
			Name:        "search",
			Description: "Search the web",
			InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Search query"},
				"limit": {"type": "number", "description": "Max results"}
			},
			"required": ["query"]
		}`),
		},
	}

	result := toolsToProvider(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}

	tool := result[0]
	if tool.Type != "function" {
		t.Errorf("Tool.Type = %q, want function", tool.Type)
	}
	if tool.Function.Name != "search" {
		t.Errorf("Function.Name = %q, want search", tool.Function.Name)
	}
	if tool.Function.Description != "Search the web" {
		t.Errorf("Function.Description = %q, want Search the web", tool.Function.Description)
	}

	params := tool.Function.Parameters
	if params["type"] != "object" {
		t.Errorf("Parameters.type = %v, want object", params["type"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not map[string]any")
	}
	queryProp, ok := props["query"].(map[string]any)
	if !ok {
		t.Fatal("query property is not map[string]any")
	}
	if queryProp["type"] != "string" {
		t.Errorf("query.type = %v, want string", queryProp["type"])
	}
	reqAny, ok := params["required"].([]any)
	if !ok {
		t.Fatalf("required is not []any, got %T", params["required"])
	}
	if len(reqAny) != 1 || reqAny[0] != "query" {
		t.Errorf("required = %v, want [query]", reqAny)
	}
}

func TestConvertToolsEmpty(t *testing.T) {
	result := toolsToProvider(nil)
	if result != nil {
		t.Errorf("expected nil for nil tools, got %v", result)
	}
	result = toolsToProvider([]Tool{})
	if result != nil {
		t.Errorf("expected nil for empty tools, got %v", result)
	}
}

// ---- resultToResponse 测试 ----

func TestConvertResult(t *testing.T) {
	result := &provider.CompletionResult{
		Content:      "Hello!",
		FinishReason: provider.FinishReasonStop,
		Usage:        provider.UsageData{InputTokens: 10, OutputTokens: 20},
	}

	resp := resultToResponse(result, "anthropic")
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Type != "message" {
		t.Errorf("Type = %q, want message", resp.Type)
	}
	if resp.Role != RoleAssistant {
		t.Errorf("Role = %q, want assistant", resp.Role)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].BlockType() != ContentBlockText {
		t.Errorf("Content[0].BlockType() = %q, want text", resp.Content[0].BlockType())
	}
	tb, ok := resp.Content[0].(*TextBlock)
	if !ok {
		t.Fatal("Content[0] 类型断言为 *TextBlock 失败")
	}
	if tb.Text != "Hello!" {
		t.Errorf("Content[0].Text = %q, want Hello!", tb.Text)
	}
	if resp.StopReason != FinishReasonEndTurn {
		t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d, want 10", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("Usage.OutputTokens = %d, want 20", resp.Usage.OutputTokens)
	}
	if resp.ID == "" {
		t.Error("ID should not be empty")
	}
	if resp.RequestID == "" {
		t.Error("RequestID should not be empty")
	}
	if resp.CreatedAt == 0 {
		t.Error("CreatedAt should not be zero")
	}
	if resp.ProviderType != "anthropic" {
		t.Errorf("ProviderType = %q, want anthropic", resp.ProviderType)
	}
}

func TestConvertResultWithToolCalls(t *testing.T) {
	result := &provider.CompletionResult{
		Content:      "",
		FinishReason: provider.FinishReasonToolCalls,
		ToolCalls: []provider.ToolCall{
			{
				ID:   "call_456",
				Type: "function",
				Function: provider.FunctionCall{
					Name:      "calc",
					Arguments: `{"expr": "1+1"}`,
				},
			},
		},
		Usage: provider.UsageData{InputTokens: 5, OutputTokens: 15},
	}

	resp := resultToResponse(result, "openai-completions")
	if resp.StopReason != FinishReasonToolUse {
		t.Errorf("StopReason = %q, want tool_use", resp.StopReason)
	}
	// 1 tool_use block, no text block
	if len(resp.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(resp.Content))
	}
	if resp.Content[0].BlockType() != ContentBlockToolUse {
		t.Errorf("Content[0].BlockType() = %q, want tool_use", resp.Content[0].BlockType())
	}
	tb, ok := resp.Content[0].(*ToolUseBlock)
	if !ok {
		t.Fatal("Content[0] 类型断言为 *ToolUseBlock 失败")
	}
	if tb.ID != "call_456" {
		t.Errorf("Content[0].ID = %q, want call_456", tb.ID)
	}
	if tb.Name != "calc" {
		t.Errorf("Content[0].Name = %q, want calc", tb.Name)
	}
}

func TestConvertResultNil(t *testing.T) {
	resp := resultToResponse(nil, "anthropic")
	if resp != nil {
		t.Errorf("expected nil for nil result, got %v", resp)
	}
}

// ---- FinishReason 映射测试 ----

func TestConvertFinishReason(t *testing.T) {
	tests := []struct {
		input    provider.FinishReason
		expected FinishReason
	}{
		{provider.FinishReasonStop, FinishReasonEndTurn},
		{provider.FinishReasonLength, FinishReasonMaxTokens},
		{provider.FinishReasonToolCalls, FinishReasonToolUse},
		{provider.FinishReason("unknown"), FinishReasonEndTurn}, // default
	}

	for _, tt := range tests {
		got := mapFinishReason(tt.input)
		if got != tt.expected {
			t.Errorf("mapFinishReason(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ---- 流事件转换测试 ----

func TestConvertStreamStart(t *testing.T) {
	sc := NewStreamConverter()
	events := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventMessageStart {
		t.Errorf("events[0].Type = %q, want message_start", events[0].Type)
	}
	if events[0].Message == nil {
		t.Fatal("events[0].Message is nil")
	}
	if events[0].Message.Role != RoleAssistant {
		t.Errorf("Message.Role = %q, want assistant", events[0].Message.Role)
	}
}

func TestConvertStreamTextDelta(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart}) // 初始化

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta("Hello"),
	})

	// 防御性: 首次 text_delta 自动补发 content_block_start + content_block_delta
	if len(events) != 2 {
		t.Fatalf("expected 2 events (auto block_start + delta), got %d", len(events))
	}
	if events[0].Type != EventContentBlockStart {
		t.Errorf("events[0].Type = %q, want content_block_start", events[0].Type)
	}
	if events[1].Type != EventContentBlockDelta {
		t.Errorf("events[1].Type = %q, want content_block_delta", events[1].Type)
	}
	if events[1].Index != 0 {
		t.Errorf("events[1].Index = %d, want 0", events[1].Index)
	}

	delta, ok := events[1].Delta.(*StreamDelta)
	if !ok {
		t.Fatal("Delta is not *StreamDelta")
	}
	if delta.Type != DeltaTextDelta {
		t.Errorf("Delta.Type = %q, want text_delta", delta.Type)
	}
	if delta.Text != "Hello" {
		t.Errorf("Delta.Text = %q, want Hello", delta.Text)
	}
}

func TestConvertStreamThinkingDelta(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewThinkingDelta("hmm"),
	})

	// 防御性 BlockStart：无前置 BlockStart 时自动补发 content_block_start
	if len(events) != 2 {
		t.Fatalf("expected 2 events (defensive BlockStart + ThinkingDelta), got %d", len(events))
	}
	if events[0].Type != EventContentBlockStart {
		t.Errorf("events[0].Type = %q, want content_block_start", events[0].Type)
	}
	if events[0].ContentBlock == nil || events[0].ContentBlock.BlockType() != ContentBlockThinking {
		t.Errorf("events[0].ContentBlock.BlockType() 不匹配，期望 thinking")
	}
	delta, ok := events[1].Delta.(*StreamDelta)
	if !ok {
		t.Fatal("events[1].Delta is not *StreamDelta")
	}
	if delta.Type != DeltaThinkingDelta {
		t.Errorf("Delta.Type = %q, want thinking_delta", delta.Type)
	}
	if delta.Thinking != "hmm" {
		t.Errorf("Delta.Thinking = %q, want hmm", delta.Thinking)
	}
}

func TestConvertStreamToolCall(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDelta("call_1", "search"),
	})

	// tool-only 流不能凭空补一个 content_block_stop。
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventContentBlockStart {
		t.Errorf("events[0].Type = %q, want content_block_start", events[0].Type)
	}
	if events[0].Index != 0 {
		t.Errorf("events[0].Index = %d, want 0", events[0].Index)
	}
	if events[0].ContentBlock.BlockType() != ContentBlockToolUse {
		t.Errorf("ContentBlock.BlockType() = %q, want tool_use", events[0].ContentBlock.BlockType())
	}
	tb, ok := events[0].ContentBlock.(*ToolUseBlock)
	if !ok {
		t.Fatal("ContentBlock 类型断言为 *ToolUseBlock 失败")
	}
	if tb.ID != "call_1" {
		t.Errorf("ContentBlock.ID = %q, want call_1", tb.ID)
	}
}

func TestConvertStreamToolCallDelta(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})
	sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDelta("call_1", "search"),
	})

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaData(`{"q":"test"}`),
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	delta, ok := events[0].Delta.(*StreamDelta)
	if !ok {
		t.Fatal("Delta is not *StreamDelta")
	}
	if delta.Type != DeltaInputJSON {
		t.Errorf("Delta.Type = %q, want input_json_delta", delta.Type)
	}
	if delta.PartialJSON != `{"q":"test"}` {
		t.Errorf("Delta.PartialJSON = %q, want {\"q\":\"test\"}", delta.PartialJSON)
	}
}

func TestConvertStreamParallelToolCallDeltasUseProviderIndex(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	firstStart := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaWithIndex("call_1", "first_tool", 0),
	})
	secondStart := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaWithIndex("call_2", "second_tool", 1),
	})
	firstDelta := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaDataWithIndex(`{"first":`, 0),
	})
	secondDelta := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaDataWithIndex(`{"second":`, 1),
	})

	if len(firstStart) != 1 || firstStart[0].Index != 0 {
		t.Fatalf("first tool start = %#v, want one block at index 0", firstStart)
	}
	if len(secondStart) != 1 || secondStart[0].Index != 1 {
		t.Fatalf("second tool start = %#v, want one block at index 1", secondStart)
	}
	if len(firstDelta) != 1 || firstDelta[0].Index != 0 {
		t.Fatalf("first tool delta = %#v, want index 0", firstDelta)
	}
	if len(secondDelta) != 1 || secondDelta[0].Index != 1 {
		t.Fatalf("second tool delta = %#v, want index 1", secondDelta)
	}
}

func TestConvertStreamUsage(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDelta(100, 50),
	})

	// Usage 被立即发射为 MessageDelta 事件（回归修复后行为）
	if len(events) != 1 {
		t.Fatalf("expected 1 event (immediate usage emit), got %d", len(events))
	}
	if events[0].Type != EventMessageDelta {
		t.Errorf("events[0].Type = %q, want message_delta", events[0].Type)
	}
	if events[0].Usage == nil {
		t.Fatal("events[0].Usage is nil")
	}
	if events[0].Usage.InputTokens != 100 {
		t.Errorf("Usage.InputTokens = %d, want 100", events[0].Usage.InputTokens)
	}
}

func TestConvertStreamStop(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop})
	events := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventMessageDelta {
		t.Errorf("events[0].Type = %q, want message_delta", events[0].Type)
	}
	if events[1].Type != EventMessageStop {
		t.Errorf("events[1].Type = %q, want message_stop", events[1].Type)
	}

	// 验证 MessageDelta 的 StopReason
	msgDelta, ok := events[0].Delta.(*MessageDelta)
	if !ok {
		t.Fatal("Delta is not *MessageDelta")
	}
	if msgDelta.StopReason != FinishReasonEndTurn {
		t.Errorf("StopReason = %q, want end_turn", msgDelta.StopReason)
	}
}

func TestConvertStreamDone(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	// Done 在没有 Stop 时应触发 handleStop（兜底）
	events := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})
	if len(events) == 0 {
		t.Fatal("expected events for done (fallback stop), got 0")
	}
}

func TestConvertStreamError(t *testing.T) {
	sc := NewStreamConverter()

	events := sc.Convert(provider.StreamEvent{
		Type: provider.StreamTypeError,
		Err:  nil,
	})
	// Err 为 nil 时应返回 unknown error
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventError {
		t.Errorf("Type = %q, want error", events[0].Type)
	}
	if events[0].Error == nil {
		t.Fatal("Error is nil")
	}
	if events[0].Error.Message != "unknown error" {
		t.Errorf("Error.Message = %q, want unknown error", events[0].Error.Message)
	}
}

func TestConvertStreamErrorWithMessage(t *testing.T) {
	sc := NewStreamConverter()
	testErr := xError.NewError(nil, nil, "connection reset", false)
	events := sc.Convert(provider.StreamEvent{
		Type: provider.StreamTypeError,
		Err:  testErr,
	})

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventError {
		t.Errorf("Type = %q, want error", events[0].Type)
	}
	if events[0].Error == nil {
		t.Fatal("Error is nil")
	}
	if !strings.Contains(events[0].Error.Message, "connection reset") {
		t.Errorf("Error.Message = %q, want containing 'connection reset'", events[0].Error.Message)
	}
}

func TestConvertStreamFullLifecycle(t *testing.T) {
	sc := NewStreamConverter()

	var allEvents []StreamEvent

	// Start
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})...)

	// Text deltas
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta("Hi"),
	})...)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta(" there"),
	})...)

	// Usage (立即发射 MessageDelta 事件)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDelta(50, 10),
	})...)

	// Stop (deferred, produces no events)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop})...)

	// Done triggers the deferred stop sequence
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})...)

	// message_start, content_block_start, delta, delta,
	// message_delta(usage立即发射), content_block_stop, message_delta, message_stop
	expectedTypes := []StreamEventType{
		EventMessageStart,
		EventContentBlockStart,
		EventContentBlockDelta,
		EventContentBlockDelta,
		EventMessageDelta,
		EventContentBlockStop,
		EventMessageDelta,
		EventMessageStop,
	}

	if len(allEvents) != len(expectedTypes) {
		t.Fatalf("expected %d events, got %d", len(expectedTypes), len(allEvents))
	}

	for i, et := range expectedTypes {
		if allEvents[i].Type != et {
			t.Errorf("events[%d].Type = %q, want %q", i, allEvents[i].Type, et)
		}
	}
}

// ---- 边界情况测试 ----

func TestConvertMultipleToolResults(t *testing.T) {
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewToolUseBlock("call_1", "tool_a", nil),
			NewToolUseBlock("call_2", "tool_b", nil),
		),
	}
	assistantMsgs, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assistantMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(assistantMsgs))
	}
	if len(assistantMsgs[0].ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(assistantMsgs[0].ToolCalls))
	}

	// Now test tool_result messages
	msgs2 := []BambooMessage{
		NewUserMessageBlocks(
			NewToolResultBlock("call_1", "result A", false),
			NewToolResultBlock("call_2", "result B", false),
		),
	}
	resultMsgs, err := messagesToProvider(msgs2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resultMsgs) != 2 {
		t.Fatalf("expected 2 messages (split tool_results), got %d", len(resultMsgs))
	}
	if resultMsgs[0].ToolCallID != "call_1" {
		t.Errorf("resultMsgs[0].ToolCallID = %q, want call_1", resultMsgs[0].ToolCallID)
	}
	if resultMsgs[1].ToolCallID != "call_2" {
		t.Errorf("resultMsgs[1].ToolCallID = %q, want call_2", resultMsgs[1].ToolCallID)
	}
}

func TestConvertMixedAssistantToolUseAndResult(t *testing.T) {
	// 先是 assistant 的 tool_use，然后是 user 的 tool_result
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewTextBlock("let me check"),
			NewToolUseBlock("call_1", "search", map[string]any{"q": "go"}),
		),
		NewUserMessageBlocks(
			NewToolResultBlock("call_1", "found results", false),
		),
	}

	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	// 第一条: assistant with text + tool_calls
	if result[0].Role != provider.RoleAssistant {
		t.Errorf("result[0].Role = %q, want assistant", result[0].Role)
	}
	if result[0].Content != "let me check" {
		t.Errorf("result[0].Content = %q", result[0].Content)
	}
	if len(result[0].ToolCalls) != 1 {
		t.Errorf("result[0].ToolCalls len = %d, want 1", len(result[0].ToolCalls))
	}
	// 第二条: tool result
	if result[1].Role != provider.RoleTool {
		t.Errorf("result[1].Role = %q, want tool", result[1].Role)
	}
}

func TestConvertToolWithEnumAndItems(t *testing.T) {
	tools := []Tool{
		{
			Name:        "search",
			Description: "Search",
			InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"sort": {"type": "string", "description": "Sort order", "enum": ["asc", "desc"]},
				"tags": {"type": "array", "description": "Tags", "items": {"type": "string"}}
			}
		}`),
		},
	}

	result := toolsToProvider(tools)
	props := result[0].Function.Parameters["properties"].(map[string]any)

	sortProp := props["sort"].(map[string]any)
	if sortProp["type"] != "string" {
		t.Errorf("sort.type = %v", sortProp["type"])
	}
	enumVals, ok := sortProp["enum"].([]any)
	if !ok || len(enumVals) != 2 {
		t.Errorf("sort.enum = %v, want [asc desc]", sortProp["enum"])
	}

	tagsProp := props["tags"].(map[string]any)
	if tagsProp["type"] != "array" {
		t.Errorf("tags.type = %v", tagsProp["type"])
	}
	itemsMap, ok := tagsProp["items"].(map[string]any)
	if !ok || itemsMap["type"] != "string" {
		t.Errorf("tags.items = %v", tagsProp["items"])
	}
}

// ──────────────────────────────────────────────────────────────────────
// 补充覆盖率测试
// ──────────────────────────────────────────────────────────────────────

// TestProviderRoleUnknown 验证未知 role 返回默认值。
func TestProviderRoleUnknown(t *testing.T) {
	tests := []struct {
		name     string
		role     MessageRole
		expected provider.MessageRole
	}{
		{"user", RoleUser, provider.RoleUser},
		{"assistant", RoleAssistant, provider.RoleAssistant},
		{"unknown_system", "system", provider.RoleUser},
		{"empty", "", provider.RoleUser},
		{"random", "custom_role", provider.RoleUser},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerRole(tt.role)
			if got != tt.expected {
				t.Errorf("providerRole(%q) = %q, 期望 %q", tt.role, got, tt.expected)
			}
		})
	}
}

// TestResultToResponseWithProviderType 验证不同 providerType 正确设置。
func TestResultToResponseWithProviderType(t *testing.T) {
	result := &provider.CompletionResult{
		Content:      "test",
		FinishReason: provider.FinishReasonStop,
		Usage:        provider.UsageData{InputTokens: 5, OutputTokens: 3},
	}

	tests := []struct {
		name         string
		providerType string
	}{
		{"anthropic", "anthropic"},
		{"openai-completions", "openai-completions"},
		{"openai-responses", "openai-responses"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := resultToResponse(result, tt.providerType)
			if resp.ProviderType != tt.providerType {
				t.Errorf("ProviderType = %q, 期望 %q", resp.ProviderType, tt.providerType)
			}
		})
	}
}

// TestResultToResponseEmptyContent 验证空 Content + 空 ToolCalls 返回空切片。
func TestResultToResponseEmptyContent(t *testing.T) {
	result := &provider.CompletionResult{
		Content:      "",
		FinishReason: provider.FinishReasonStop,
		Usage:        provider.UsageData{},
	}

	resp := resultToResponse(result, "anthropic")
	if resp == nil {
		t.Fatal("期望非 nil 响应")
	}
	if len(resp.Content) != 0 {
		t.Errorf("Content len = %d, 期望 0", len(resp.Content))
	}
}

// TestConvertStreamDefaultDeltaType 验证未知 delta 类型返回空。
func TestConvertStreamDefaultDeltaType(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.StreamDelta[any]{Type: "unknown_type", Data: "something"},
	})

	if len(events) != 0 {
		t.Errorf("期望 0 个事件, 实际 %d", len(events))
	}
}

// TestConvertDefaultEventType 验证未知 StreamType 返回空。
func TestConvertDefaultEventType(t *testing.T) {
	sc := NewStreamConverter()
	events := sc.Convert(provider.StreamEvent{Type: "custom_event"})
	if len(events) != 0 {
		t.Errorf("期望 0 个事件, 实际 %d", len(events))
	}
}

// TestMessagesToProvider_ThinkingBlock 验证 thinking block 被跳过。
func TestMessagesToProvider_ThinkingBlock(t *testing.T) {
	msgs := []BambooMessage{
		NewUserMessageBlocks(
			NewThinkingBlock("deep thoughts", "sig_abc"),
			NewTextBlock("real message"),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息, 实际 %d", len(result))
	}
	// thinking 被保留到 ThinkingContent，text 正常保留
	if result[0].Content != "real message" {
		t.Errorf("Content = %q, 期望 %q", result[0].Content, "real message")
	}
	if result[0].ThinkingContent != "deep thoughts" {
		t.Errorf("ThinkingContent = %q, 期望 %q", result[0].ThinkingContent, "deep thoughts")
	}
	if result[0].ThinkingSignature != "sig_abc" {
		t.Errorf("ThinkingSignature = %q, 期望 %q", result[0].ThinkingSignature, "sig_abc")
	}
}

// TestMessagesToProvider_MixedBlocks 验证混合 text + tool_use blocks 的转换。
func TestMessagesToProvider_MixedBlocks(t *testing.T) {
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewTextBlock("let me search"),
			NewToolUseBlock("call_1", "search", map[string]any{"q": "golang"}),
			NewTextBlock(" and also "),
			NewToolUseBlock("call_2", "calculator", map[string]any{"expr": "1+1"}),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息, 实际 %d", len(result))
	}
	// 两个 text 拼接
	if result[0].Content != "let me search and also " {
		t.Errorf("Content = %q, 期望 %q", result[0].Content, "let me search and also ")
	}
	// 两个 tool calls
	if len(result[0].ToolCalls) != 2 {
		t.Fatalf("期望 2 个 tool calls, 实际 %d", len(result[0].ToolCalls))
	}
	if result[0].ToolCalls[0].Function.Name != "search" {
		t.Errorf("ToolCalls[0].Name = %q, 期望 search", result[0].ToolCalls[0].Function.Name)
	}
	if result[0].ToolCalls[1].Function.Name != "calculator" {
		t.Errorf("ToolCalls[1].Name = %q, 期望 calculator", result[0].ToolCalls[1].Function.Name)
	}
}

// TestMessagesToProvider_NilToolUseInput 验证 tool_use block 的 Input 为 nil 时使用空 JSON。
func TestMessagesToProvider_NilToolUseInput(t *testing.T) {
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewToolUseBlock("call_nil", "no_args_tool", nil),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息, 实际 %d", len(result))
	}
	if len(result[0].ToolCalls) != 1 {
		t.Fatalf("期望 1 个 tool call, 实际 %d", len(result[0].ToolCalls))
	}
	// nil input 应被序列化为 `{}`
	if result[0].ToolCalls[0].Function.Arguments != `{}` {
		t.Errorf("Arguments = %q, 期望 %q", result[0].ToolCalls[0].Function.Arguments, `{}`)
	}
}

// TestMessagesToProvider_OnlyToolUseBlocks 验证仅有 tool_use blocks（无 text）时的转换。
func TestMessagesToProvider_OnlyToolUseBlocks(t *testing.T) {
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewToolUseBlock("call_1", "tool_a", nil),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息, 实际 %d", len(result))
	}
	// 没有文本，但 tool_calls 存在，消息应被保留
	if result[0].Content != "" {
		t.Errorf("Content = %q, 期望空字符串", result[0].Content)
	}
	if len(result[0].ToolCalls) != 1 {
		t.Errorf("期望 1 个 tool call, 实际 %d", len(result[0].ToolCalls))
	}
}

// TestMessagesToProvider_OnlyThinkingBlocks 验证仅有 thinking blocks（无 text、无 tool_calls）时不生成消息。
func TestMessagesToProvider_OnlyThinkingBlocks(t *testing.T) {
	msgs := []BambooMessage{
		NewAssistantMessageBlocks(
			NewThinkingBlock("hmm", "sig"),
		),
	}
	result, err := messagesToProvider(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// thinking 被保留到 ThinkingContent，即使没有 text 也生成消息
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息, 实际 %d", len(result))
	}
	if result[0].ThinkingContent != "hmm" {
		t.Errorf("ThinkingContent = %q, 期望 %q", result[0].ThinkingContent, "hmm")
	}
	if result[0].ThinkingSignature != "sig" {
		t.Errorf("ThinkingSignature = %q, 期望 %q", result[0].ThinkingSignature, "sig")
	}
	if result[0].Content != "" {
		t.Errorf("Content = %q, 期望空字符串", result[0].Content)
	}
}

// TestNewToolUseBlock_MarshalError 验证 NewToolUseBlock 序列化失败时回退为空 JSON。
func TestNewToolUseBlock_MarshalError(t *testing.T) {
	// 传入无法序列化的值（如 channel）
	block := NewToolUseBlock("id", "name", make(chan int))
	tb, ok := block.(*ToolUseBlock)
	if !ok {
		t.Fatal("类型断言为 *ToolUseBlock 失败")
	}
	if string(tb.Input) != `{}` {
		t.Errorf("Input = %q, 期望 %q", string(tb.Input), `{}`)
	}
}

// TestBuildParameters_EmptySchema 验证空 schema 构建参数。
func TestBuildParameters_EmptySchema(t *testing.T) {
	params := buildParameters(json.RawMessage(`{"type":"object"}`))
	if params["type"] != "object" {
		t.Errorf("type = %v, 期望 object", params["type"])
	}
	if _, ok := params["properties"]; ok {
		t.Error("不应包含 properties")
	}
	if _, ok := params["required"]; ok {
		t.Error("不应包含 required")
	}
}

// TestConvertStreamTypeDone 验证 StreamTypeDone 在未经过 Start 时直接返回空事件。
func TestConvertStreamTypeDone(t *testing.T) {
	sc := NewStreamConverter()
	events := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})
	if len(events) != 0 {
		t.Errorf("期望 0 个事件, 实际 %d", len(events))
	}
}

// TestConvertStreamStopWithoutUsage 验证 stop 事件无 usage 时使用默认值。
func TestConvertStreamStopWithoutUsage(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})
	// 不发送 usage delta，直接 stop

	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop})
	events := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})
	if len(events) != 2 {
		t.Fatalf("期望 2 个事件, 实际 %d", len(events))
	}

	msgDeltaEvent := events[0]
	if msgDeltaEvent.Usage == nil {
		t.Fatal("Usage 不应为 nil")
	}
	// 应使用默认零值
	if msgDeltaEvent.Usage.InputTokens != 0 {
		t.Errorf("InputTokens = %d, 期望 0", msgDeltaEvent.Usage.InputTokens)
	}
	if msgDeltaEvent.Usage.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, 期望 0", msgDeltaEvent.Usage.OutputTokens)
	}
}

// ──────────────────────────────────────────────────────────────────────
// Task 12: configToProvider ThinkingConfig / ProviderExtra 测试
// ──────────────────────────────────────────────────────────────────────

// TestConfigToProvider_ThinkingConfig 验证 ThinkingConfig 正确透传到 provider.ChatConfig。
func TestConfigToProvider_ThinkingConfig(t *testing.T) {
	cfg := &RequestConfig{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 4096,
		ThinkingConfig: &ThinkingConfig{
			Effort: "high",
		},
	}

	result := configToProvider(cfg)
	if result == nil {
		t.Fatal("期望非 nil 结果")
	}
	if result.ThinkingConfig == nil {
		t.Fatal("ThinkingConfig 不应为 nil")
	}
	if result.ThinkingConfig.Effort != "high" {
		t.Errorf("Effort = %q, 期望 %q", result.ThinkingConfig.Effort, "high")
	}
}

// TestConfigToProvider_ProviderExtra 验证 ProviderExtra 正确透传到 provider.ChatConfig。
func TestConfigToProvider_ProviderExtra(t *testing.T) {
	cfg := &RequestConfig{
		Model:     "test-model",
		MaxTokens: 2048,
		ProviderExtra: map[string]any{
			"custom_key": "custom_value",
			"top_k":      50.0,
		},
	}

	result := configToProvider(cfg)
	if result == nil {
		t.Fatal("期望非 nil 结果")
	}
	if result.ProviderExtra == nil {
		t.Fatal("ProviderExtra 不应为 nil")
	}
	if result.ProviderExtra["custom_key"] != "custom_value" {
		t.Errorf("custom_key = %v, 期望 custom_value", result.ProviderExtra["custom_key"])
	}
	if result.ProviderExtra["top_k"] != 50.0 {
		t.Errorf("top_k = %v, 期望 50.0", result.ProviderExtra["top_k"])
	}
}

// ──────────────────────────────────────────────────────────────────────
// Task 12: StreamConverter 场景测试
// ──────────────────────────────────────────────────────────────────────

// TestStreamConverter_TextOnly 验证纯文本流完整生命周期: start → block_start(text) → text_delta → stop。
func TestStreamConverter_TextOnly(t *testing.T) {
	sc := NewStreamConverter()
	var allEvents []StreamEvent

	// start
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})...)

	// block_start(text) — 使用 NewBlockStartDelta
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewBlockStartDelta("text"),
	})...)

	// text_delta
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta("hello"),
	})...)

	// usage
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDelta(10, 5),
	})...)

	// stop (deferred)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop})...)
	// done triggers stop sequence
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})...)

	expectedTypes := []StreamEventType{
		EventMessageStart,
		EventContentBlockStart,
		EventContentBlockDelta,
		EventMessageDelta,
		EventContentBlockStop,
		EventMessageDelta,
		EventMessageStop,
	}

	if len(allEvents) != len(expectedTypes) {
		t.Fatalf("期望 %d 个事件, 实际 %d", len(expectedTypes), len(allEvents))
	}
	for i, et := range expectedTypes {
		if allEvents[i].Type != et {
			t.Errorf("events[%d].Type = %q, 期望 %q", i, allEvents[i].Type, et)
		}
	}

	// 验证 usage 透传（stop 的 message_delta 在 events[5]）
	msgDeltaEvent := allEvents[5]
	if msgDeltaEvent.Usage == nil {
		t.Fatal("message_delta Usage 不应为 nil")
	}
	if msgDeltaEvent.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, 期望 10", msgDeltaEvent.Usage.InputTokens)
	}
	if msgDeltaEvent.Usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, 期望 5", msgDeltaEvent.Usage.OutputTokens)
	}
}

// TestStreamConverter_ThinkingAndText 验证思考+文本流: start → block_start(thinking) → thinking_delta → block_start(text) → text_delta → stop。
func TestStreamConverter_ThinkingAndText(t *testing.T) {
	sc := NewStreamConverter()
	var allEvents []StreamEvent

	// start
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})...)

	// block_start(thinking)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewBlockStartDelta("thinking"),
	})...)

	// thinking_delta
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewThinkingDelta("let me think..."),
	})...)

	// block_start(text) — 新的内容块
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewBlockStartDelta("text"),
	})...)

	// text_delta
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta("the answer is 42"),
	})...)

	// stop (deferred) + done triggers stop sequence
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop})...)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})...)

	expectedTypes := []StreamEventType{
		EventMessageStart,
		EventContentBlockStart,
		EventContentBlockDelta,
		EventContentBlockStop,
		EventContentBlockStart,
		EventContentBlockDelta,
		EventContentBlockStop,
		EventMessageDelta,
		EventMessageStop,
	}

	if len(allEvents) != len(expectedTypes) {
		t.Fatalf("期望 %d 个事件, 实际 %d", len(expectedTypes), len(allEvents))
	}
	for i, et := range expectedTypes {
		if allEvents[i].Type != et {
			t.Errorf("events[%d].Type = %q, 期望 %q", i, allEvents[i].Type, et)
		}
	}

	// 验证 thinking delta 内容
	thinkingEvent := allEvents[2]
	delta, ok := thinkingEvent.Delta.(*StreamDelta)
	if !ok {
		t.Fatal("thinking event Delta 类型不匹配")
	}
	if delta.Type != DeltaThinkingDelta {
		t.Errorf("Delta.Type = %q, 期望 %q", delta.Type, DeltaThinkingDelta)
	}
	if delta.Thinking != "let me think..." {
		t.Errorf("Delta.Thinking = %q, 期望 %q", delta.Thinking, "let me think...")
	}

	// 验证 text delta 内容
	textEvent := allEvents[5]
	textDelta, ok := textEvent.Delta.(*StreamDelta)
	if !ok {
		t.Fatal("text event Delta 类型不匹配")
	}
	if textDelta.Text != "the answer is 42" {
		t.Errorf("Delta.Text = %q, 期望 %q", textDelta.Text, "the answer is 42")
	}
}

// TestStreamConverter_ToolCall 验证工具调用流: start → text_delta → tool_call → tool_call_delta → stop。
func TestStreamConverter_ToolCall(t *testing.T) {
	sc := NewStreamConverter()
	var allEvents []StreamEvent

	// start
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})...)

	// text_delta (no block_start, relies on auto-emit)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta("calling tool..."),
	})...)

	// tool_call → [content_block_stop, content_block_start(tool_use)]
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDelta("call_abc", "search"),
	})...)

	// tool_call_delta
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewToolCallDeltaData(`{"query":"golang"}`),
	})...)

	// stop (deferred) + done triggers stop sequence
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop})...)
	allEvents = append(allEvents, sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})...)

	expectedTypes := []StreamEventType{
		EventMessageStart,
		EventContentBlockStart,
		EventContentBlockDelta,
		EventContentBlockStop,
		EventContentBlockStart,
		EventContentBlockDelta,
		EventContentBlockStop,
		EventMessageDelta,
		EventMessageStop,
	}

	if len(allEvents) != len(expectedTypes) {
		t.Fatalf("期望 %d 个事件, 实际 %d", len(expectedTypes), len(allEvents))
	}
	for i, et := range expectedTypes {
		if allEvents[i].Type != et {
			t.Errorf("events[%d].Type = %q, 期望 %q", i, allEvents[i].Type, et)
		}
	}

	// 验证 tool_use block 的 ID 和 Name
	toolStartEvent := allEvents[4]
	if toolStartEvent.ContentBlock == nil {
		t.Fatal("tool_use ContentBlock 不应为 nil")
	}
	if toolStartEvent.ContentBlock.BlockType() != ContentBlockToolUse {
		t.Errorf("ContentBlock.BlockType() = %q, 期望 %q", toolStartEvent.ContentBlock.BlockType(), ContentBlockToolUse)
	}
	tb, ok := toolStartEvent.ContentBlock.(*ToolUseBlock)
	if !ok {
		t.Fatal("ContentBlock 类型断言为 *ToolUseBlock 失败")
	}
	if tb.ID != "call_abc" {
		t.Errorf("ContentBlock.ID = %q, 期望 %q", tb.ID, "call_abc")
	}
	if tb.Name != "search" {
		t.Errorf("ContentBlock.Name = %q, 期望 %q", tb.Name, "search")
	}

	// 验证 tool_call_delta 的 PartialJSON
	toolDeltaEvent := allEvents[5]
	toolDelta, ok := toolDeltaEvent.Delta.(*StreamDelta)
	if !ok {
		t.Fatal("tool_call_delta Delta 类型不匹配")
	}
	if toolDelta.Type != DeltaInputJSON {
		t.Errorf("Delta.Type = %q, 期望 %q", toolDelta.Type, DeltaInputJSON)
	}
	if toolDelta.PartialJSON != `{"query":"golang"}` {
		t.Errorf("Delta.PartialJSON = %q, 期望 %q", toolDelta.PartialJSON, `{"query":"golang"}`)
	}
}

// ──────────────────────────────────────────────────────────────────────

// TestConvertStreamWithUsageInStop 验证 stop 事件携带之前存储的 usage。
func TestConvertStreamWithUsageInStop(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	// 先发送 usage delta
	sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDelta(100, 200),
	})

	// 发送文本
	sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewTextDelta("test"),
	})

	// stop 延迟到 done 才输出终止事件序列
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop})
	events := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})

	// events: [content_block_stop, message_delta, message_stop]
	msgDeltaEvent := events[1]
	if msgDeltaEvent.Usage == nil {
		t.Fatal("message_delta event Usage is nil")
	}
	if msgDeltaEvent.Usage.InputTokens != 100 {
		t.Errorf("Usage.InputTokens = %d, want 100", msgDeltaEvent.Usage.InputTokens)
	}
	if msgDeltaEvent.Usage.OutputTokens != 200 {
		t.Errorf("Usage.OutputTokens = %d, want 200", msgDeltaEvent.Usage.OutputTokens)
	}
}

// TestStreamConverter_UsageImmediateEmit 验证 Usage Delta 被立即转为 MessageDelta 事件。
//
// 背景：Usage 事件（StreamDeltaTypeUsage）到达时，StreamConverter 应立即返回
// 一个 EventMessageDelta 事件并携带 Usage，确保流中断时 usage 不丢失。
// 这是 usage 即时发射的回归测试。
func TestStreamConverter_UsageImmediateEmit(t *testing.T) {
	sc := NewStreamConverter()

	// 先发 start 建立 message_start
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	// 发 BlockStart(text)
	sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewBlockStartDelta("text"),
	})

	// 发 Usage Delta — 应立即返回 MessageDelta + Usage
	events := sc.Convert(provider.StreamEvent{
		Type: provider.StreamTypeDelta,
		Delta: provider.NewUsageDeltaWithCache(
			42, 88, 5, 10,
		),
	})

	if len(events) == 0 {
		t.Fatal("Usage delta should produce at least one event")
	}

	first := events[0]
	if first.Type != EventMessageDelta {
		t.Errorf("first event Type = %v, want %v", first.Type, EventMessageDelta)
	}
	if first.Usage == nil {
		t.Fatal("first event Usage is nil — usage should be emitted immediately")
	}
	if first.Usage.InputTokens != 42 {
		t.Errorf("Usage.InputTokens = %d, want 42", first.Usage.InputTokens)
	}
	if first.Usage.OutputTokens != 88 {
		t.Errorf("Usage.OutputTokens = %d, want 88", first.Usage.OutputTokens)
	}
	if first.Usage.CacheCreationInputTokens != 5 {
		t.Errorf("Usage.CacheCreationInputTokens = %d, want 5", first.Usage.CacheCreationInputTokens)
	}
	if first.Usage.CacheReadInputTokens != 10 {
		t.Errorf("Usage.CacheReadInputTokens = %d, want 10", first.Usage.CacheReadInputTokens)
	}
}

// TestStreamConverter_StopDeferredToDone 验证 StreamTypeStop 不立即输出终止事件，
// 而是延迟到 StreamTypeDone 统一输出。这保证了无论上游发多少个 stop，
// 客户端只收到一次终止事件序列。
func TestStreamConverter_StopDeferredToDone(t *testing.T) {
	sc := NewStreamConverter()

	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDelta, Delta: provider.NewTextDelta("hi")})

	stopEvents := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonStop})
	if len(stopEvents) != 0 {
		t.Fatalf("StreamTypeStop should produce 0 events (deferred to Done), got %d", len(stopEvents))
	}

	doneEvents := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})
	if len(doneEvents) == 0 {
		t.Fatal("StreamTypeDone should produce stop event sequence")
	}

	var hasMsgDelta, hasMsgStop bool
	for _, ev := range doneEvents {
		if ev.Type == EventMessageDelta {
			hasMsgDelta = true
		}
		if ev.Type == EventMessageStop {
			hasMsgStop = true
		}
	}
	if !hasMsgDelta {
		t.Error("missing EventMessageDelta in Done output")
	}
	if !hasMsgStop {
		t.Error("missing EventMessageStop in Done output")
	}
}

// TestStreamConverter_FinishReasonPriority 验证当上游发送多个 StreamTypeStop 时，
// finishReason 优先级策略确保 tool_use 不被 stop 覆盖。
//
// 复现 agent loop 中断场景：上游先发 finish_reason=stop，后发 finish_reason=tool_calls。
// 修复前：客户端收到 finish_reason=stop → agent loop 终止。
// 修复后：tool_use 优先级高于 stop → 客户端收到 finish_reason=tool_use → agent loop 继续。
func TestStreamConverter_FinishReasonPriority(t *testing.T) {
	sc := NewStreamConverter()
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDelta, Delta: provider.NewToolCallDelta("call_1", "edit")})

	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonStop})
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStop, FinishReason: provider.FinishReasonToolCalls})

	doneEvents := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})

	var stopReason FinishReason
	for _, ev := range doneEvents {
		if ev.Type == EventMessageDelta {
			if md, ok := ev.Delta.(*MessageDelta); ok {
				stopReason = md.StopReason
			}
		}
	}

	if stopReason != FinishReasonToolUse {
		t.Fatalf("StopReason = %q, want %q (tool_use must win over stop)", stopReason, FinishReasonToolUse)
	}
}

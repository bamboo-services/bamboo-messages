package bamboo

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// === buildMessages 测试 ===

func TestBuildMessages_TextContent(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "你好"},
		{Role: provider.RoleAssistant, Content: "你好！有什么可以帮你的？"},
	}
	result := buildMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected role user, got %s", result[0].Role)
	}
	if len(result[0].Content) != 1 || result[0].Content[0].Type != "text" {
		t.Fatalf("expected single text block, got %+v", result[0].Content)
	}
	if result[0].Content[0].Text != "你好" {
		t.Errorf("expected text '你好', got %s", result[0].Content[0].Text)
	}
	if result[1].Role != "assistant" {
		t.Errorf("expected role assistant, got %s", result[1].Role)
	}
}

func TestBuildMessages_ImageContentBlock(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: provider.RoleUser,
			ContentBlocks: []provider.ContentBlock{
				provider.ImageContentBlock{
					Source: provider.ImageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      "iVBORw0KGgo=",
					},
				},
			},
		},
	}
	result := buildMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result[0].Content))
	}
	block := result[0].Content[0]
	if block.Type != "image" {
		t.Fatalf("expected type image, got %s", block.Type)
	}
	if block.Source == nil {
		t.Fatal("expected non-nil source")
	}
	if block.Source.Type != "base64" {
		t.Errorf("expected source type base64, got %s", block.Source.Type)
	}
	if block.Source.MediaType != "image/png" {
		t.Errorf("expected media_type image/png, got %s", block.Source.MediaType)
	}
	if block.Source.Data != "iVBORw0KGgo=" {
		t.Errorf("expected data iVBORw0KGgo=, got %s", block.Source.Data)
	}
}

func TestBuildMessages_DocumentContentBlock(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: provider.RoleUser,
			ContentBlocks: []provider.ContentBlock{
				provider.DocumentContentBlock{
					Source: provider.DocumentSource{
						Type: "url",
						URL:  "https://example.com/doc.pdf",
					},
				},
			},
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result[0].Content))
	}
	block := result[0].Content[0]
	if block.Type != "document" {
		t.Fatalf("expected type document, got %s", block.Type)
	}
	if block.Source == nil {
		t.Fatal("expected non-nil source")
	}
	if block.Source.URL != "https://example.com/doc.pdf" {
		t.Errorf("expected url https://example.com/doc.pdf, got %s", block.Source.URL)
	}
}

func TestBuildMessages_TextContentBlock(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: provider.RoleUser,
			ContentBlocks: []provider.ContentBlock{
				provider.TextContentBlock{Text: "from content block"},
			},
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result[0].Content))
	}
	if result[0].Content[0].Type != "text" {
		t.Fatalf("expected type text, got %s", result[0].Content[0].Type)
	}
	if result[0].Content[0].Text != "from content block" {
		t.Errorf("expected 'from content block', got %s", result[0].Content[0].Text)
	}
}

func TestBuildMessages_ContentStringAndBlocksCoexist(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:    provider.RoleUser,
			Content: "描述这张图",
			ContentBlocks: []provider.ContentBlock{
				provider.ImageContentBlock{
					Source: provider.ImageSource{Type: "url", URL: "https://example.com/img.png"},
				},
			},
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 content blocks (text + image), got %d", len(result[0].Content))
	}
	if result[0].Content[0].Type != "text" || result[0].Content[0].Text != "描述这张图" {
		t.Errorf("first block should be text, got %+v", result[0].Content[0])
	}
	if result[0].Content[1].Type != "image" {
		t.Errorf("second block should be image, got %+v", result[0].Content[1])
	}
}

func TestBuildMessages_RoleToolMergedIntoUser(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "调用工具"},
		{
			Role:      provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{{ID: "call_1", Type: "function", Function: provider.FunctionCall{Name: "get_weather", Arguments: `{"city":"北京"}`}}},
		},
		{
			Role:       provider.RoleTool,
			Content:    "晴天 25°C",
			ToolCallID: "call_1",
		},
	}
	result := buildMessages(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages (user, assistant, tool_result-merged-user), got %d", len(result))
	}
	// 第三条消息应为 user 角色，包含 tool_result 块
	if result[2].Role != "user" {
		t.Fatalf("expected role user for tool result, got %s", result[2].Role)
	}
	if len(result[2].Content) != 1 || result[2].Content[0].Type != "tool_result" {
		t.Fatalf("expected single tool_result block, got %+v", result[2].Content)
	}
	if result[2].Content[0].ToolUseID != "call_1" {
		t.Errorf("expected tool_use_id call_1, got %s", result[2].Content[0].ToolUseID)
	}
	if result[2].Content[0].Content != "晴天 25°C" {
		t.Errorf("expected content '晴天 25°C', got %s", result[2].Content[0].Content)
	}
}

func TestBuildMessages_MultipleRoleToolMergedIntoSameUser(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:       provider.RoleTool,
			Content:    "结果1",
			ToolCallID: "call_1",
		},
		{
			Role:       provider.RoleTool,
			Content:    "结果2",
			ToolCallID: "call_2",
		},
	}
	result := buildMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 merged user message, got %d", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 tool_result blocks, got %d", len(result[0].Content))
	}
	if result[0].Content[0].ToolUseID != "call_1" {
		t.Errorf("expected first tool_use_id call_1, got %s", result[0].Content[0].ToolUseID)
	}
	if result[0].Content[1].ToolUseID != "call_2" {
		t.Errorf("expected second tool_use_id call_2, got %s", result[0].Content[1].ToolUseID)
	}
}

func TestBuildMessages_RoleToolNotMergedWhenPreviousIsAssistant(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, Content: "好的"},
		{
			Role:       provider.RoleTool,
			Content:    "结果",
			ToolCallID: "call_1",
		},
	}
	result := buildMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (assistant + new user), got %d", len(result))
	}
	if result[1].Role != "user" {
		t.Fatalf("expected role user, got %s", result[1].Role)
	}
	if result[1].Content[0].Type != "tool_result" {
		t.Errorf("expected tool_result block, got %s", result[1].Content[0].Type)
	}
}

func TestBuildMessages_EmptyContent(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: ""},
	}
	result := buildMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content block (empty text fallback), got %d", len(result[0].Content))
	}
	if result[0].Content[0].Type != "text" {
		t.Errorf("expected type text, got %s", result[0].Content[0].Type)
	}
	if result[0].Content[0].Text != "" {
		t.Errorf("expected empty text, got %s", result[0].Content[0].Text)
	}
}

func TestBuildMessages_CacheControl(t *testing.T) {
	cc := provider.NewEphemeralCacheControl()
	msgs := []provider.Message{
		{
			Role:                  provider.RoleUser,
			Content:               "cached content",
			CacheControl:          cc,
			CacheControlBlockType: "text",
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result[0].Content))
	}
	block := result[0].Content[0]
	if block.Type != "text" {
		t.Fatalf("expected type text, got %s", block.Type)
	}
	if len(block.CacheControl) == 0 {
		t.Fatal("expected non-empty cache_control")
	}
	var parsed provider.CacheControl
	if err := json.Unmarshal(block.CacheControl, &parsed); err != nil {
		t.Fatalf("failed to unmarshal cache_control: %v", err)
	}
	if parsed.Type != "ephemeral" {
		t.Errorf("expected type ephemeral, got %s", parsed.Type)
	}
}

func TestBuildMessages_CacheControlByBlockType(t *testing.T) {
	cc := provider.NewEphemeralCacheControl()
	msgs := []provider.Message{
		{
			Role:                  provider.RoleAssistant,
			Content:               "text content",
			ThinkingContent:       "thinking content",
			ThinkingSignature:     "sig123",
			CacheControl:          cc,
			CacheControlBlockType: "thinking",
		},
	}
	result := buildMessages(msgs)
	blocks := result[0].Content
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (thinking + text), got %d", len(blocks))
	}
	// cache_control 应设置在 thinking 块上（而非 text）
	if blocks[0].Type != "thinking" {
		t.Fatalf("expected first block thinking, got %s", blocks[0].Type)
	}
	if len(blocks[0].CacheControl) == 0 {
		t.Error("expected cache_control on thinking block")
	}
	if blocks[1].Type != "text" {
		t.Fatalf("expected second block text, got %s", blocks[1].Type)
	}
	if len(blocks[1].CacheControl) != 0 {
		t.Error("expected NO cache_control on text block")
	}
}

func TestBuildMessages_ToolCalls(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "call_1", Type: "function", Function: provider.FunctionCall{Name: "get_weather", Arguments: `{"city":"北京"}`}},
				{ID: "call_2", Type: "function", Function: provider.FunctionCall{Name: "get_time", Arguments: `{}`}},
			},
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 tool_use blocks, got %d", len(result[0].Content))
	}
	for i, block := range result[0].Content {
		if block.Type != "tool_use" {
			t.Errorf("block %d: expected type tool_use, got %s", i, block.Type)
		}
	}
	if result[0].Content[0].ID != "call_1" {
		t.Errorf("expected id call_1, got %s", result[0].Content[0].ID)
	}
	if result[0].Content[0].Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", result[0].Content[0].Name)
	}
	if string(result[0].Content[0].Input) != `{"city":"北京"}` {
		t.Errorf("expected input {\"city\":\"北京\"}, got %s", string(result[0].Content[0].Input))
	}
	if string(result[0].Content[1].Input) != `{}` {
		t.Errorf("expected input {}, got %s", string(result[0].Content[1].Input))
	}
}

func TestBuildMessages_ToolCallsEmptyArgumentsDefaultsToEmptyObject(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "call_1", Type: "function", Function: provider.FunctionCall{Name: "no_args"}},
			},
		},
	}
	result := buildMessages(msgs)
	if string(result[0].Content[0].Input) != `{}` {
		t.Errorf("expected default empty object {}, got %s", string(result[0].Content[0].Input))
	}
}

func TestBuildMessages_ThinkingContent(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:              provider.RoleAssistant,
			ThinkingContent:   "让我想想...",
			ThinkingSignature: "signature_abc",
			Content:           "答案是42",
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 blocks (thinking + text), got %d", len(result[0].Content))
	}
	if result[0].Content[0].Type != "thinking" {
		t.Fatalf("expected first block thinking, got %s", result[0].Content[0].Type)
	}
	if result[0].Content[0].Thinking != "让我想想..." {
		t.Errorf("expected thinking content, got %s", result[0].Content[0].Thinking)
	}
	if result[0].Content[0].Signature != "signature_abc" {
		t.Errorf("expected signature, got %s", result[0].Content[0].Signature)
	}
	if result[0].Content[1].Type != "text" {
		t.Fatalf("expected second block text, got %s", result[0].Content[1].Type)
	}
	if result[0].Content[1].Text != "答案是42" {
		t.Errorf("expected text '答案是42', got %s", result[0].Content[1].Text)
	}
}

func TestBuildMessages_ThinkingOnlySignature(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:              provider.RoleAssistant,
			ThinkingSignature: "sig_only",
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(result[0].Content))
	}
	if result[0].Content[0].Type != "thinking" {
		t.Fatalf("expected thinking block, got %s", result[0].Content[0].Type)
	}
	if result[0].Content[0].Signature != "sig_only" {
		t.Errorf("expected signature sig_only, got %s", result[0].Content[0].Signature)
	}
}

func TestBuildMessages_RedactedThinkingData(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:                 provider.RoleAssistant,
			RedactedThinkingData: "encrypted_data_here",
			Content:              "正常回复",
		},
	}
	result := buildMessages(msgs)
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 blocks (redacted_thinking + text), got %d", len(result[0].Content))
	}
	if result[0].Content[0].Type != "redacted_thinking" {
		t.Fatalf("expected first block redacted_thinking, got %s", result[0].Content[0].Type)
	}
	if result[0].Content[0].Data != "encrypted_data_here" {
		t.Errorf("expected data encrypted_data_here, got %s", result[0].Content[0].Data)
	}
}

func TestBuildMessages_ReasoningID(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:        provider.RoleAssistant,
			Content:     "response",
			ReasoningID: "rs_abc123",
		},
	}
	result := buildMessages(msgs)
	if result[0].ReasoningID != "rs_abc123" {
		t.Errorf("expected reasoning_id rs_abc123, got %s", result[0].ReasoningID)
	}
}

func TestBuildMessages_ReasoningIDOmitempty(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
	}
	result := buildMessages(msgs)
	if result[0].ReasoningID != "" {
		t.Errorf("expected empty reasoning_id, got %s", result[0].ReasoningID)
	}
	// 验证 JSON 序列化时 reasoning_id 被省略
	data, _ := json.Marshal(result[0])
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["reasoning_id"]; ok {
		t.Error("expected reasoning_id to be omitted from JSON when empty")
	}
}

func TestBuildMessages_ToolResultWithToolName(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:       provider.RoleTool,
			Content:    "result",
			ToolCallID: "call_1",
			ToolName:   "get_weather",
		},
	}
	result := buildMessages(msgs)
	block := result[0].Content[0]
	if block.ToolName != "get_weather" {
		t.Errorf("expected tool_name get_weather, got %s", block.ToolName)
	}
}

func TestBuildMessages_ToolResultWithError(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:       provider.RoleTool,
			Content:    "tool failed",
			ToolCallID: "call_1",
			IsError:    true,
		},
	}
	result := buildMessages(msgs)
	block := result[0].Content[0]
	if !block.IsError {
		t.Error("expected is_error true")
	}
}

func TestBuildMessages_RoleSystemDegradesToUser(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system prompt as message"},
	}
	result := buildMessages(msgs)
	if result[0].Role != "user" {
		t.Errorf("expected system degraded to user, got %s", result[0].Role)
	}
}

func TestBuildMessages_NilMessages(t *testing.T) {
	result := buildMessages(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 messages for nil input, got %d", len(result))
	}
}

func TestBuildMessages_JSONRoundTrip(t *testing.T) {
	msgs := []provider.Message{
		{
			Role:              provider.RoleAssistant,
			ThinkingContent:   "思考",
			ThinkingSignature: "sig",
			Content:           "回复",
			ToolCalls: []provider.ToolCall{
				{ID: "c1", Type: "function", Function: provider.FunctionCall{Name: "fn", Arguments: `{"k":"v"}`}},
			},
		},
	}
	result := buildMessages(msgs)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}
	// 验证 JSON 结构合法
	var parsed []wireMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 message after round trip, got %d", len(parsed))
	}
}

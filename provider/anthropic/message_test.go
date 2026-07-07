package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// buildMessages 测试
// ==============================

// TestBuildMessages_ToolUseInputNotDoubleEncoded 验证 tool_use 的 input 字段不会被双重编码。
//
// buildMessages 将 tc.Function.Arguments 转为 json.RawMessage，
// json.Marshal 时直接输出原始字节，不会产生双重编码。
func TestBuildMessages_ToolUseInputNotDoubleEncoded(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{
					ID: "call_123",
					Function: provider.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"北京"}`,
					},
				},
			},
		},
	}

	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	msg := result[0]
	if msg["role"] != "assistant" {
		t.Fatalf("expected assistant role, got %v", msg["role"])
	}

	content, ok := msg["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", msg["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}

	block := content[0]
	if block["type"] != "tool_use" {
		t.Fatalf("expected tool_use block, got type=%v", block["type"])
	}
	if block["id"] != "call_123" {
		t.Errorf("expected tool_use ID 'call_123', got %v", block["id"])
	}
	if block["name"] != "get_weather" {
		t.Errorf("expected tool_use name 'get_weather', got %v", block["name"])
	}

	// 验证 input 是合法 JSON 对象，不是双重编码字符串
	inputRaw, ok := block["input"].(json.RawMessage)
	if !ok {
		t.Fatalf("expected input to be json.RawMessage, got %T", block["input"])
	}

	var parsed map[string]any
	if err := json.Unmarshal(inputRaw, &parsed); err != nil {
		t.Fatalf("tool_use input is not valid JSON object: %v\nraw: %s", err, string(inputRaw))
	}

	if parsed["city"] != "北京" {
		t.Errorf("expected city=北京, got %v", parsed["city"])
	}
}

// TestBuildMessages_EmptyToolUseArguments 验证空 arguments 不会 panic，且 input 为空对象。
func TestBuildMessages_EmptyToolUseArguments(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{
					ID: "call_empty",
					Function: provider.FunctionCall{
						Name:      "noop",
						Arguments: "",
					},
				},
			},
		},
	}

	// 不应 panic
	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[0]["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}

	block := content[0]
	if block["type"] != "tool_use" {
		t.Fatalf("expected tool_use block, got type=%v", block["type"])
	}

	// input 应为空对象 map[string]any{}
	input, ok := block["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected input to be map[string]any, got %T", block["input"])
	}
	if len(input) != 0 {
		t.Errorf("expected empty object, got %v", input)
	}
}

// TestBuildMessages_UserTextMessage 验证 user 角色纯文本消息构建。
func TestBuildMessages_UserTextMessage(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "Hello world"},
	}

	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0]["role"] != "user" {
		t.Errorf("expected role 'user', got %v", result[0]["role"])
	}

	content, ok := result[0]["content"].([]map[string]any)
	if !ok || len(content) != 1 {
		t.Fatalf("expected 1 content block, got %v", result[0]["content"])
	}
	if content[0]["type"] != "text" {
		t.Errorf("expected text block, got %v", content[0]["type"])
	}
	if content[0]["text"] != "Hello world" {
		t.Errorf("expected 'Hello world', got %v", content[0]["text"])
	}
}

// TestBuildMessages_ToolResultMerged 验证连续 tool_result 消息合并到同一 user 消息。
func TestBuildMessages_ToolResultMerged(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "What's the weather?"},
		{
			Role:       provider.RoleTool,
			Content:    `{"temp": 25}`,
			ToolCallID: "call-1",
		},
		{
			Role:       provider.RoleTool,
			Content:    `{"temp": 20}`,
			ToolCallID: "call-2",
		},
	}

	result := p.buildMessages(messages)

	// user + merged tool results = 2 messages
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (user + merged tool results), got %d", len(result))
	}

	// 第二条消息应包含 2 个 tool_result blocks
	content, ok := result[1]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[1]["content"])
	}
	if len(content) != 2 {
		t.Errorf("expected 2 tool_result blocks in merged message, got %d", len(content))
	}
}

// TestBuildMessages_ToolResultWithName 验证 RoleTool 带 ToolName 时 tool_result 包含 name 字段。
func TestBuildMessages_ToolResultWithName(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role:       provider.RoleTool,
			Content:    `{"temp": 25}`,
			ToolCallID: "call-1",
			ToolName:   "get_weather",
		},
	}

	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[0]["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}

	block := content[0]
	if block["type"] != "tool_result" {
		t.Fatalf("expected tool_result block, got type=%v", block["type"])
	}
	if block["tool_use_id"] != "call-1" {
		t.Errorf("expected tool_use_id 'call-1', got %v", block["tool_use_id"])
	}
	if block["name"] != "get_weather" {
		t.Errorf("expected name 'get_weather', got %v", block["name"])
	}
}

// TestBuildMessages_ToolResultWithoutName 验证 RoleTool 无 ToolName 时 tool_result 不包含 name 键。
func TestBuildMessages_ToolResultWithoutName(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role:       provider.RoleTool,
			Content:    `{"temp": 25}`,
			ToolCallID: "call-1",
		},
	}

	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[0]["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}

	block := content[0]
	if block["type"] != "tool_result" {
		t.Fatalf("expected tool_result block, got type=%v", block["type"])
	}
	if _, exists := block["name"]; exists {
		t.Errorf("expected 'name' key not to exist when ToolName is empty")
	}
}

// TestBuildMessages_AssistantWithThinking 验证 thinking block 保留在 assistant 消息中。
func TestBuildMessages_AssistantWithThinking(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role:              provider.RoleAssistant,
			Content:           "The answer is 42.",
			ThinkingContent:   "Let me think about this...",
			ThinkingSignature: "sig_abc123",
		},
	}

	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[0]["content"])
	}
	if len(content) < 2 {
		t.Fatalf("expected at least 2 blocks (thinking + text), got %d", len(content))
	}

	// 第一个 block 应为 thinking
	if content[0]["type"] != "thinking" {
		t.Errorf("expected first block to be thinking, got %v", content[0]["type"])
	}
	if content[0]["thinking"] != "Let me think about this..." {
		t.Errorf("expected thinking content, got %v", content[0]["thinking"])
	}
	if content[0]["signature"] != "sig_abc123" {
		t.Errorf("expected signature, got %v", content[0]["signature"])
	}
}

// TestBuildMessages_AssistantWithRedactedThinking 验证 RedactedThinkingData 产生 redacted_thinking block。
func TestBuildMessages_AssistantWithRedactedThinking(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role:                 provider.RoleAssistant,
			Content:              "The answer is 42.",
			RedactedThinkingData: "rt_encrypted_data_xyz",
		},
	}

	result := p.buildMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[0]["content"])
	}

	// 应包含 redacted_thinking block 和 text block
	var foundRedacted bool
	for _, block := range content {
		if block["type"] == "redacted_thinking" {
			foundRedacted = true
			if block["data"] != "rt_encrypted_data_xyz" {
				t.Errorf("redacted_thinking data = %v, want 'rt_encrypted_data_xyz'", block["data"])
			}
		}
	}
	if !foundRedacted {
		t.Error("expected redacted_thinking block in content, not found")
	}
}

// TestBuildMessages_AssistantWithThinkingAndRedactedThinking 验证 thinking + redacted_thinking + text 的顺序。
func TestBuildMessages_AssistantWithThinkingAndRedactedThinking(t *testing.T) {
	p := NewProvider("test-api-key")

	messages := []provider.Message{
		{
			Role:                 provider.RoleAssistant,
			Content:              "Answer.",
			ThinkingContent:      "Let me think...",
			ThinkingSignature:    "sig_abc",
			RedactedThinkingData: "rt_data",
		},
	}

	result := p.buildMessages(messages)

	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[0]["content"])
	}
	if len(content) < 3 {
		t.Fatalf("expected at least 3 blocks (thinking + redacted + text), got %d", len(content))
	}

	// 顺序: thinking → redacted_thinking → text
	if content[0]["type"] != "thinking" {
		t.Errorf("block[0] type = %v, want 'thinking'", content[0]["type"])
	}
	if content[1]["type"] != "redacted_thinking" {
		t.Errorf("block[1] type = %v, want 'redacted_thinking'", content[1]["type"])
	}
	if content[2]["type"] != "text" {
		t.Errorf("block[2] type = %v, want 'text'", content[2]["type"])
	}
}

// ---- applyMsgCacheControl blockType 定位测试 ----

func TestApplyMsgCacheControl_ThinkingBlockType(t *testing.T) {
	blocks := []map[string]any{
		{"type": "thinking", "thinking": "...", "signature": "sig"},
		{"type": "text", "text": "response"},
	}
	cc := provider.NewEphemeralCacheControl()
	applyMsgCacheControl(blocks, cc, "thinking")

	if _, ok := blocks[0]["cache_control"]; !ok {
		t.Error("expected cache_control on thinking block (index 0)")
	}
	if _, ok := blocks[1]["cache_control"]; ok {
		t.Error("expected NO cache_control on text block (index 1)")
	}
}

func TestApplyMsgCacheControl_TextBlockType(t *testing.T) {
	blocks := []map[string]any{
		{"type": "text", "text": "hello"},
		{"type": "image", "source": map[string]any{"type": "url"}},
	}
	cc := provider.NewEphemeralCacheControl()
	applyMsgCacheControl(blocks, cc, "text")

	if _, ok := blocks[0]["cache_control"]; !ok {
		t.Error("expected cache_control on text block (index 0)")
	}
	if _, ok := blocks[1]["cache_control"]; ok {
		t.Error("expected NO cache_control on image block (index 1)")
	}
}

func TestApplyMsgCacheControl_ToolUseBlockType(t *testing.T) {
	blocks := []map[string]any{
		{"type": "thinking", "thinking": "...", "signature": "sig"},
		{"type": "text", "text": "response"},
		{"type": "tool_use", "id": "call_1", "name": "get_weather"},
		{"type": "tool_use", "id": "call_2", "name": "get_time"},
	}
	cc := provider.NewEphemeralCacheControl()
	applyMsgCacheControl(blocks, cc, "tool_use")

	if _, ok := blocks[0]["cache_control"]; ok {
		t.Error("expected NO cache_control on thinking block")
	}
	if _, ok := blocks[1]["cache_control"]; ok {
		t.Error("expected NO cache_control on text block")
	}
	if _, ok := blocks[2]["cache_control"]; ok {
		t.Error("expected NO cache_control on first tool_use block")
	}
	if _, ok := blocks[3]["cache_control"]; !ok {
		t.Error("expected cache_control on last tool_use block (index 3)")
	}
}

func TestApplyMsgCacheControl_FallbackToLastBlock(t *testing.T) {
	blocks := []map[string]any{
		{"type": "text", "text": "hello"},
		{"type": "image", "source": map[string]any{"type": "url"}},
	}
	cc := provider.NewEphemeralCacheControl()

	// blockType 为空 → 回退到最后一个 block
	applyMsgCacheControl(blocks, cc, "")

	if _, ok := blocks[1]["cache_control"]; !ok {
		t.Error("expected cache_control on last block (fallback)")
	}
}

func TestApplyMsgCacheControl_BlockTypeNotFound_Fallback(t *testing.T) {
	blocks := []map[string]any{
		{"type": "text", "text": "hello"},
		{"type": "text", "text": "world"},
	}
	cc := provider.NewEphemeralCacheControl()

	// blockType="thinking" 但没有 thinking block → 回退到最后一个
	applyMsgCacheControl(blocks, cc, "thinking")

	if _, ok := blocks[1]["cache_control"]; !ok {
		t.Error("expected cache_control on last block (fallback when blockType not found)")
	}
}

func TestBuildMessages_AssistantCacheControlOnThinking(t *testing.T) {
	p := &Provider{}
	msgs := []provider.Message{
		{
			Role:                 provider.RoleAssistant,
			Content:              "response",
			ThinkingContent:      "long thinking",
			ThinkingSignature:    "sig123",
			CacheControl:         provider.NewEphemeralCacheControl(),
			CacheControlBlockType: "thinking",
		},
	}

	result := p.buildMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any")
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(content))
	}
	if content[0]["type"] != "thinking" {
		t.Errorf("block[0] type = %v, want 'thinking'", content[0]["type"])
	}
	if _, ok := content[0]["cache_control"]; !ok {
		t.Error("expected cache_control on thinking block")
	}
	if content[1]["type"] != "text" {
		t.Errorf("block[1] type = %v, want 'text'", content[1]["type"])
	}
	if _, ok := content[1]["cache_control"]; ok {
		t.Error("expected NO cache_control on text block")
	}
}

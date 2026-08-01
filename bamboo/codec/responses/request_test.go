package responses

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

func TestParseRequest_InputString(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hello"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", req.Config.Model, "gpt-4o")
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != bamboo.RoleUser {
		t.Errorf("Role = %q, want %q", req.Messages[0].Role, bamboo.RoleUser)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", req.Messages[0].Content[0])
	}
	if tb.Text != "Hello" {
		t.Errorf("Text = %q, want %q", tb.Text, "Hello")
	}
}

func TestParseRequest_Instructions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"instructions": "You are helpful.",
		"input": "Hi"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.System != "You are helpful." {
		t.Errorf("System = %q", req.System)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra should preserve native instructions")
	}
	if req.Config.ProviderExtra["instructions"] != "You are helpful." {
		t.Errorf("ProviderExtra[instructions] = %v", req.Config.ProviderExtra["instructions"])
	}
}

func TestParseRequest_InputArrayUserMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello world"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d", len(req.Messages))
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleUser {
		t.Errorf("Role = %q", msg.Role)
	}
	tb, ok := msg.Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", msg.Content[0])
	}
	if tb.Text != "Hello world" {
		t.Errorf("Text = %q", tb.Text)
	}
}

func TestParseRequest_InputArrayAssistantMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Hi there"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleAssistant {
		t.Errorf("Role = %q", msg.Role)
	}
	tb, ok := msg.Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock")
	}
	if tb.Text != "Hi there" {
		t.Errorf("Text = %q", tb.Text)
	}
}

func TestParseRequest_InputArraySystemMessage(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"instructions": "Base instructions.",
		"input": [
			{"type": "message", "role": "system", "content": [{"type": "input_text", "text": "Extra system"}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hi"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1 (system excluded)", len(req.Messages))
	}
	// system 应合并到 System 字段
	if req.System != "Base instructions.\n\nExtra system" {
		t.Errorf("System = %q", req.System)
	}
}

func TestParseRequest_FunctionCall(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call", "id": "fc_1", "call_id": "call_abc", "name": "get_weather", "arguments": "{\"city\":\"SF\"}"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleAssistant {
		t.Errorf("Role = %q", msg.Role)
	}
	toolUse, ok := msg.Content[0].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", msg.Content[0])
	}
	if toolUse.ID != "call_abc" {
		t.Errorf("ID = %q, want %q", toolUse.ID, "call_abc")
	}
	if toolUse.Name != "get_weather" {
		t.Errorf("Name = %q", toolUse.Name)
	}
}

func TestParseRequest_FunctionCallOutput(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call_output", "call_id": "call_abc", "output": "Sunny, 72F"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleUser {
		t.Errorf("Role = %q", msg.Role)
	}
	trBlock, ok := msg.Content[0].(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", msg.Content[0])
	}
	if trBlock.ToolUseID != "call_abc" {
		t.Errorf("ToolUseID = %q", trBlock.ToolUseID)
	}
	if trBlock.Content != "Sunny, 72F" {
		t.Errorf("Content = %q", trBlock.Content)
	}
}

func TestParseRequest_Reasoning(t *testing.T) {
	body := []byte(`{
		"model": "o3",
		"input": [
			{"type": "reasoning", "content": [{"type": "reasoning_text", "text": "Let me think..."}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "What is 2+2?"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	// 第一条消息是 assistant + ThinkingBlock
	if req.Messages[0].Role != bamboo.RoleAssistant {
		t.Errorf("Messages[0].Role = %q", req.Messages[0].Role)
	}
	thinking, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinking.Thinking != "Let me think..." {
		t.Errorf("Thinking = %q", thinking.Thinking)
	}
}

// TestParseRequest_ReasoningContentPreferred 验证 content（原始思考全文）
// 优先于 summary（有损摘要）被解析。
func TestParseRequest_ReasoningContentPreferred(t *testing.T) {
	body := []byte(`{
		"model": "o3",
		"input": [
			{"type": "reasoning", "summary": [{"type": "summary_text", "text": "摘要"}], "content": [{"type": "reasoning_text", "text": "完整原始思考"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	thinking, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinking.Thinking != "完整原始思考" {
		t.Errorf("Thinking = %q, want content text (preferred over summary)", thinking.Thinking)
	}
}

// TestParseRequest_ReasoningEncryptedContent 验证 encrypted_content 透传为
// ThinkingBlock.Signature，且无明文文本时仍生成消息（保留加密推理链）。
func TestParseRequest_ReasoningEncryptedContent(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5",
		"input": [
			{"type": "reasoning", "summary": [], "encrypted_content": "gAAAAAB_encrypted"},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "continue"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	thinking, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinking.Thinking != "" {
		t.Errorf("Thinking = %q, want empty", thinking.Thinking)
	}
	if thinking.Signature != "gAAAAAB_encrypted" {
		t.Errorf("Signature = %q, want encrypted_content passthrough", thinking.Signature)
	}
}

// TestParseRequest_AssistantTurnMerging 验证连续 assistant 侧条目
// （reasoning / message / function_call）合并为单条 assistant 消息。
//
// Chat Completions 语义下单轮 assistant 消息同时携带 reasoning_content +
// content + tool_calls；拆分为多条消息会触发 DeepSeek 等思考模式强校验
// 上游的 "reasoning_content must be passed back" 错误。
func TestParseRequest_AssistantTurnMerging(t *testing.T) {
	body := []byte(`{
		"model": "deepseek-v4-pro",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "查看目录"}]},
			{"type": "reasoning", "id": "rs_1", "summary": [{"type": "summary_text", "text": "需要调用工具"}]},
			{"type": "message", "role": "assistant", "status": "completed", "content": [{"type": "output_text", "text": "我来看看。"}]},
			{"type": "function_call", "call_id": "call_1", "name": "shell", "arguments": "{\"cmd\":\"ls\"}"},
			{"type": "function_call", "call_id": "call_2", "name": "shell", "arguments": "{\"cmd\":\"pwd\"}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "file1.txt"},
			{"type": "function_call_output", "call_id": "call_2", "output": "/root"},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "继续"}]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// 期望：user, assistant(合并轮), tool, tool, user — 共 5 条
	if len(req.Messages) != 5 {
		t.Fatalf("Messages len = %d, want 5", len(req.Messages))
	}

	// 合并后的 assistant 消息：thinking + text + 2 个 tool_use 同属一条
	turn := req.Messages[1]
	if turn.Role != bamboo.RoleAssistant {
		t.Fatalf("Messages[1].Role = %q, want assistant", turn.Role)
	}
	if len(turn.Content) != 4 {
		t.Fatalf("assistant turn blocks = %d, want 4 (thinking + text + 2 tool_use)", len(turn.Content))
	}
	if _, ok := turn.Content[0].(*bamboo.ThinkingBlock); !ok {
		t.Errorf("block[0] = %T, want *ThinkingBlock", turn.Content[0])
	}
	if text, ok := turn.Content[1].(*bamboo.TextBlock); !ok || text.Text != "我来看看。" {
		t.Errorf("block[1] = %T, want *TextBlock with merged text", turn.Content[1])
	}
	if tu, ok := turn.Content[2].(*bamboo.ToolUseBlock); !ok || tu.ID != "call_1" {
		t.Errorf("block[2] = %T, want *ToolUseBlock(call_1)", turn.Content[2])
	}
	if tu, ok := turn.Content[3].(*bamboo.ToolUseBlock); !ok || tu.ID != "call_2" {
		t.Errorf("block[3] = %T, want *ToolUseBlock(call_2) — 并行工具调用应同属一条消息", turn.Content[3])
	}
	// reasoning item 的 id 应保留为 ReasoningID
	if turn.ReasoningID != "rs_1" {
		t.Errorf("ReasoningID = %q, want %q", turn.ReasoningID, "rs_1")
	}

	// function_call_output 结束 assistant 轮次，各自成为 user(tool_result) 消息
	if req.Messages[2].Role != bamboo.RoleUser || req.Messages[3].Role != bamboo.RoleUser {
		t.Errorf("tool results roles = %q/%q, want user/user", req.Messages[2].Role, req.Messages[3].Role)
	}
	if req.Messages[4].Role != bamboo.RoleUser {
		t.Errorf("Messages[4].Role = %q, want user", req.Messages[4].Role)
	}
}

func TestParseRequest_Tools(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Weather?",
		"tools": [{
			"type": "function",
			"name": "get_weather",
			"description": "Get weather",
			"parameters": {
				"type": "object",
				"properties": {
					"city": {"type": "string", "description": "City name"}
				},
				"required": ["city"]
			}
		}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Config.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(req.Config.Tools))
	}
	tool := req.Config.Tools[0]
	if tool.Name != "get_weather" {
		t.Errorf("Name = %q", tool.Name)
	}
	if tool.Description != "Get weather" {
		t.Errorf("Description = %q", tool.Description)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("解析 InputSchema 失败: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("Schema Type = %v", schema["type"])
	}
}

func TestParseRequest_ReasoningConfig(t *testing.T) {
	body := []byte(`{
		"model": "o3",
		"input": "Think",
		"reasoning": {"effort": "high"}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ThinkingConfig == nil {
		t.Fatal("ThinkingConfig is nil")
	}
	if req.Config.ThinkingConfig.Effort != "high" {
		t.Errorf("Effort = %q, want %q", req.Config.ThinkingConfig.Effort, "high")
	}
}

func TestParseRequest_MaxOutputTokens(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"max_output_tokens": 4096
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", req.Config.MaxTokens)
	}
}

func TestParseRequest_TextFormat(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"text": {"format": {"type": "json_object"}}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ResponseFormat != "json_object" {
		t.Errorf("ResponseFormat = %q", req.Config.ResponseFormat)
	}
}

func TestParseRequest_ProviderExtra(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"previous_response_id": "resp_prev",
		"store": true,
		"truncation": "auto"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra is nil")
	}
	if v, ok := req.Config.ProviderExtra["previous_response_id"].(string); !ok || v != "resp_prev" {
		t.Errorf("previous_response_id = %v", req.Config.ProviderExtra["previous_response_id"])
	}
	if v, ok := req.Config.ProviderExtra["store"].(bool); !ok || !v {
		t.Errorf("store = %v", req.Config.ProviderExtra["store"])
	}
	if v, ok := req.Config.ProviderExtra["truncation"].(string); !ok || v != "auto" {
		t.Errorf("truncation = %v", req.Config.ProviderExtra["truncation"])
	}
}

func TestParseRequest_ToolChoiceString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"auto"`, "auto"},
		{`"none"`, "none"},
		{`"required"`, "required"},
		{`{"type":"function","name":"my_tool"}`, "forced"},
	}

	for _, tt := range tests {
		choice, err := parseToolChoice(json.RawMessage(tt.input))
		if err != nil {
			t.Errorf("parseToolChoice(%s) error = %v", tt.input, err)
		}
		if choice != tt.expected {
			t.Errorf("parseToolChoice(%s) = %q, want %q", tt.input, choice, tt.expected)
		}
	}
}

func TestParseRequest_Stream(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": "Hi",
		"stream": true
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if !req.IsStream {
		t.Errorf("IsStream = false, want true")
	}
}

func TestParseRequest_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)

	_, err := parseRequest(body)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	var bambooErr *pkgErrors.BambooError
	if !errors.As(err, &bambooErr) {
		t.Errorf("expected *pkgErrors.BambooError, got %T", err)
	}
	if bambooErr == nil {
		t.Fatalf("expected BambooError to be non-nil")
	}
	if bambooErr.Category != "下游" {
		t.Errorf("Category = %q, want %q", bambooErr.Category, "下游")
	}
	if bambooErr.Message == "" {
		t.Errorf("Message should not be empty")
	}
	if bambooErr.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0", bambooErr.StatusCode)
	}
}

// ── 非标准字段类型容错测试 ──

// TestParseRequest_FunctionCallArgumentsAsObject 验证 arguments 为 JSON object
// （非标准格式，部分客户端如 Codex 截图工具链会这样序列化）时正常解析。
func TestParseRequest_FunctionCallArgumentsAsObject(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call", "call_id": "call_1", "name": "screenshot", "arguments": {"action": "capture", "region": "full"}}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleAssistant {
		t.Errorf("Role = %q, want assistant", msg.Role)
	}
	toolUse, ok := msg.Content[0].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", msg.Content[0])
	}
	if toolUse.ID != "call_1" {
		t.Errorf("ID = %q, want call_1", toolUse.ID)
	}
	if toolUse.Name != "screenshot" {
		t.Errorf("Name = %q, want screenshot", toolUse.Name)
	}
	// arguments object 应原样保留为 Input
	var args map[string]any
	if err := json.Unmarshal(toolUse.Input, &args); err != nil {
		t.Fatalf("Input 不是合法 JSON: %v", err)
	}
	if args["action"] != "capture" {
		t.Errorf("args[action] = %v, want capture", args["action"])
	}
}

// TestParseRequest_FunctionCallArgumentsAsString 验证标准 JSON string 格式仍然正常。
func TestParseRequest_FunctionCallArgumentsAsString(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call", "call_id": "call_1", "name": "get_weather", "arguments": "{\"city\":\"SF\"}"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	toolUse, ok := req.Messages[0].Content[0].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", req.Messages[0].Content[0])
	}
	var args map[string]any
	if err := json.Unmarshal(toolUse.Input, &args); err != nil {
		t.Fatalf("Input 不是合法 JSON: %v", err)
	}
	if args["city"] != "SF" {
		t.Errorf("args[city] = %v, want SF", args["city"])
	}
}

// TestParseRequest_FunctionCallOutputAsObject 验证 output 为 JSON object 时
// 序列化为 JSON 字符串存入 ToolResultBlock.Content。
func TestParseRequest_FunctionCallOutputAsObject(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call_output", "call_id": "call_1", "output": {"result": "screenshot saved", "path": "/tmp/shot.png"}}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	msg := req.Messages[0]
	if msg.Role != bamboo.RoleUser {
		t.Errorf("Role = %q, want user", msg.Role)
	}
	trBlock, ok := msg.Content[0].(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", msg.Content[0])
	}
	if trBlock.ToolUseID != "call_1" {
		t.Errorf("ToolUseID = %q, want call_1", trBlock.ToolUseID)
	}
	// object 应序列化为 JSON 字符串
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trBlock.Content), &parsed); err != nil {
		t.Fatalf("Content 不是合法 JSON 字符串: %v (content=%q)", err, trBlock.Content)
	}
	if parsed["result"] != "screenshot saved" {
		t.Errorf("parsed[result] = %v", parsed["result"])
	}
}

// TestParseRequest_FunctionCallOutputAsString 验证标准 string 格式仍然正常。
func TestParseRequest_FunctionCallOutputAsString(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "function_call_output", "call_id": "call_1", "output": "Sunny, 72F"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	trBlock, ok := req.Messages[0].Content[0].(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", req.Messages[0].Content[0])
	}
	if trBlock.Content != "Sunny, 72F" {
		t.Errorf("Content = %q, want %q", trBlock.Content, "Sunny, 72F")
	}
}

// TestParseRequest_ReasoningSummaryAsString 验证 summary 为 string（非标准）
// 时正常解析为 ThinkingBlock。
func TestParseRequest_ReasoningSummaryAsString(t *testing.T) {
	body := []byte(`{
		"model": "o3",
		"input": [
			{"type": "reasoning", "summary": "I need to analyze the screenshot carefully."}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	thinking, ok := req.Messages[0].Content[0].(*bamboo.ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", req.Messages[0].Content[0])
	}
	if thinking.Thinking != "I need to analyze the screenshot carefully." {
		t.Errorf("Thinking = %q", thinking.Thinking)
	}
}

// TestParseRequest_InputAsSingleObject 验证 input 为单个 object（非标准）
// 时自动包装为数组正常解析。
func TestParseRequest_InputAsSingleObject(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": {"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Hello"}]}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != bamboo.RoleUser {
		t.Errorf("Role = %q, want user", req.Messages[0].Role)
	}
	tb, ok := req.Messages[0].Content[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", req.Messages[0].Content[0])
	}
	if tb.Text != "Hello" {
		t.Errorf("Text = %q, want Hello", tb.Text)
	}
}

// TestParseRequest_MultiTurnScreenshotFlow 模拟 Codex 截图完整链路：
// user 提问 → function_call(screenshot, arguments=object) →
// function_call_output(截图结果) → user 带图片追问。
func TestParseRequest_MultiTurnScreenshotFlow(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "帮我看看屏幕"}]},
			{"type": "reasoning", "id": "rs_1", "summary": "需要截取屏幕截图"},
			{"type": "function_call", "call_id": "call_shot", "name": "computer_screenshot", "arguments": {"display": 0, "format": "png"}},
			{"type": "function_call_output", "call_id": "call_shot", "output": {"image_data": "iVBORw0KGgo=", "width": 1920, "height": 1080}},
			{"type": "message", "role": "user", "content": [
				{"type": "input_text", "text": "这个界面上有什么？"},
				{"type": "input_image", "image_url": "data:image/png;base64,iVBORw0KGgo="}
			]}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// 期望：user, assistant(reasoning+tool_use), user(tool_result), user(image) — 共 4 条
	if len(req.Messages) != 4 {
		t.Fatalf("Messages len = %d, want 4", len(req.Messages))
	}

	// [0] user 提问
	if req.Messages[0].Role != bamboo.RoleUser {
		t.Errorf("Messages[0].Role = %q, want user", req.Messages[0].Role)
	}

	// [1] assistant 合并轮：thinking + tool_use
	turn := req.Messages[1]
	if turn.Role != bamboo.RoleAssistant {
		t.Fatalf("Messages[1].Role = %q, want assistant", turn.Role)
	}
	if len(turn.Content) != 2 {
		t.Fatalf("assistant blocks = %d, want 2 (thinking + tool_use)", len(turn.Content))
	}
	if _, ok := turn.Content[0].(*bamboo.ThinkingBlock); !ok {
		t.Errorf("block[0] = %T, want *ThinkingBlock", turn.Content[0])
	}
	toolUse, ok := turn.Content[1].(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("block[1] = %T, want *ToolUseBlock", turn.Content[1])
	}
	if toolUse.Name != "computer_screenshot" {
		t.Errorf("toolUse.Name = %q", toolUse.Name)
	}
	// arguments object 应正确解析
	var args map[string]any
	if err := json.Unmarshal(toolUse.Input, &args); err != nil {
		t.Fatalf("toolUse.Input 不是合法 JSON: %v", err)
	}
	if args["format"] != "png" {
		t.Errorf("args[format] = %v, want png", args["format"])
	}
	if turn.ReasoningID != "rs_1" {
		t.Errorf("ReasoningID = %q, want rs_1", turn.ReasoningID)
	}

	// [2] user tool_result（output 为 object → JSON 字符串）
	if req.Messages[2].Role != bamboo.RoleUser {
		t.Errorf("Messages[2].Role = %q, want user", req.Messages[2].Role)
	}
	trBlock, ok := req.Messages[2].Content[0].(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("Messages[2].Content[0] = %T, want *ToolResultBlock", req.Messages[2].Content[0])
	}
	if trBlock.ToolUseID != "call_shot" {
		t.Errorf("ToolUseID = %q, want call_shot", trBlock.ToolUseID)
	}

	// [3] user 带图片追问
	imgMsg := req.Messages[3]
	if imgMsg.Role != bamboo.RoleUser {
		t.Errorf("Messages[3].Role = %q, want user", imgMsg.Role)
	}
	if len(imgMsg.Content) != 2 {
		t.Fatalf("image message blocks = %d, want 2 (text + image)", len(imgMsg.Content))
	}
	if _, ok := imgMsg.Content[0].(*bamboo.TextBlock); !ok {
		t.Errorf("block[0] = %T, want *TextBlock", imgMsg.Content[0])
	}
	if _, ok := imgMsg.Content[1].(*bamboo.ImageBlock); !ok {
		t.Errorf("block[1] = %T, want *ImageBlock", imgMsg.Content[1])
	}
}

// TestParseRequest_ErrorMessageIncludesDetail 验证解析失败时错误信息包含原始错误详情。
func TestParseRequest_ErrorMessageIncludesDetail(t *testing.T) {
	// 构造一个真正无法解析的 input（array 内含 number 元素）
	body := []byte(`{
		"model": "gpt-4o",
		"input": [123, "invalid"]
	}`)

	_, err := parseRequest(body)
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
	var bambooErr *pkgErrors.BambooError
	if !errors.As(err, &bambooErr) {
		t.Fatalf("expected *pkgErrors.BambooError, got %T", err)
	}
	// 错误信息应包含原始解析错误详情
	if !strings.Contains(bambooErr.Message, "failed to parse input field") {
		t.Errorf("Message = %q, should contain 'failed to parse input field'", bambooErr.Message)
	}
}

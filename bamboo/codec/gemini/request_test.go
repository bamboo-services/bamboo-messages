package gemini

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"google.golang.org/genai"
)

// helper: 将 BambooMessage 的 Content 转为具体类型方便断言
func mustTextBlock(t *testing.T, block bamboo.ContentBlock) *bamboo.TextBlock {
	t.Helper()
	tb, ok := block.(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", block)
	}
	return tb
}

func mustToolUseBlock(t *testing.T, block bamboo.ContentBlock) *bamboo.ToolUseBlock {
	t.Helper()
	tub, ok := block.(*bamboo.ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", block)
	}
	return tub
}

func mustToolResultBlock(t *testing.T, block bamboo.ContentBlock) *bamboo.ToolResultBlock {
	t.Helper()
	trb, ok := block.(*bamboo.ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", block)
	}
	return trb
}

func mustImageBlock(t *testing.T, block bamboo.ContentBlock) *bamboo.ImageBlock {
	t.Helper()
	ib, ok := block.(*bamboo.ImageBlock)
	if !ok {
		t.Fatalf("expected *ImageBlock, got %T", block)
	}
	return ib
}

func TestParseRequest_SystemInstruction(t *testing.T) {
	body := `{
		"systemInstruction": {"parts":[{"text":"You are a helpful assistant."},{"text":"Be concise."}]},
		"contents": [{"role":"user","parts":[{"text":"Hi"}]}]
	}`

	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if req.System != "You are a helpful assistant.\nBe concise." {
		t.Errorf("System = %q", req.System)
	}
}

func TestParseRequest_SystemInstruction_Empty(t *testing.T) {
	body := `{
		"contents": [{"role":"user","parts":[{"text":"Hi"}]}]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if req.System != "" {
		t.Errorf("System should be empty, got %q", req.System)
	}
}

func TestParseRequest_RoleMapping(t *testing.T) {
	body := `{
		"contents": [
			{"role":"user","parts":[{"text":"hello"}]},
			{"role":"model","parts":[{"text":"hi there"}]},
			{"role":"function","parts":[{"functionResponse":{"name":"get_weather","response":{"output":"sunny"}}}]}
		]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("Messages len = %d, want 3", len(req.Messages))
	}

	// 第一条 user
	if req.Messages[0].Role != bamboo.RoleUser {
		t.Errorf("msg[0].Role = %q, want user", req.Messages[0].Role)
	}

	// 第二条 model → assistant
	if req.Messages[1].Role != bamboo.RoleAssistant {
		t.Errorf("msg[1].Role = %q, want assistant", req.Messages[1].Role)
	}

	// 第三条 function → user + ToolResultBlock
	if req.Messages[2].Role != bamboo.RoleUser {
		t.Errorf("msg[2].Role = %q, want user", req.Messages[2].Role)
	}
	if len(req.Messages[2].Content) != 1 {
		t.Fatalf("msg[2].Content len = %d", len(req.Messages[2].Content))
	}
	trb := mustToolResultBlock(t, req.Messages[2].Content[0])
	if trb.Content != "sunny" {
		t.Errorf("ToolResult content = %q, want 'sunny'", trb.Content)
	}
}

func TestParseRequest_Parts_TextInline(t *testing.T) {
	body := `{
		"contents": [
			{"role":"user","parts":[{"text":"hello"},{"inlineData":{"mimeType":"image/png","data":"base64data"}}]}
		]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d", len(req.Messages))
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("blocks len = %d, want 2", len(blocks))
	}

	// text
	tb := mustTextBlock(t, blocks[0])
	if tb.Text != "hello" {
		t.Errorf("text = %q", tb.Text)
	}

	// inlineData → image base64
	ib := mustImageBlock(t, blocks[1])
	if ib.Source.Type != "base64" {
		t.Errorf("source type = %q, want base64", ib.Source.Type)
	}
	if ib.Source.MediaType != "image/png" {
		t.Errorf("mediaType = %q", ib.Source.MediaType)
	}
	if ib.Source.Data != "base64data" {
		t.Errorf("data = %q", ib.Source.Data)
	}
}

func TestParseRequest_Parts_FileData(t *testing.T) {
	body := `{
		"contents": [
			{"role":"user","parts":[{"fileData":{"mimeType":"image/jpeg","fileUri":"https://example.com/img.jpg"}}]}
		]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	ib := mustImageBlock(t, req.Messages[0].Content[0])
	if ib.Source.Type != "url" {
		t.Errorf("source type = %q, want url", ib.Source.Type)
	}
	if ib.Source.URL != "https://example.com/img.jpg" {
		t.Errorf("url = %q", ib.Source.URL)
	}
}

func TestParseRequest_Parts_FunctionCall_IDSynthesis(t *testing.T) {
	body := `{
		"contents": [
			{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"SF"}}}]}
		]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(blocks))
	}
	tub := mustToolUseBlock(t, blocks[0])
	// ID 应该被合成为 gemini_call_get_weather_0
	if tub.ID != "gemini_call_get_weather_0" {
		t.Errorf("ID = %q, want 'gemini_call_get_weather_0'", tub.ID)
	}
	if tub.Name != "get_weather" {
		t.Errorf("Name = %q", tub.Name)
	}
	// 验证 input args
	var args map[string]any
	if err := json.Unmarshal(tub.Input, &args); err != nil {
		t.Fatalf("failed to unmarshal input: %v", err)
	}
	if args["city"] != "SF" {
		t.Errorf("args.city = %v", args["city"])
	}
}

func TestParseRequest_Parts_FunctionCall_WithID(t *testing.T) {
	body := `{
		"contents": [
			{"role":"model","parts":[{"functionCall":{"id":"custom-id-123","name":"search","args":{"q":"test"}}}]}
		]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	tub := mustToolUseBlock(t, req.Messages[0].Content[0])
	if tub.ID != "custom-id-123" {
		t.Errorf("ID = %q, want 'custom-id-123'", tub.ID)
	}
}

func TestParseRequest_Tools_FunctionDeclarations(t *testing.T) {
	body := `{
		"contents": [{"role":"user","parts":[{"text":"hi"}]}],
		"tools": [{
			"functionDeclarations": [{
				"name": "get_weather",
				"description": "Get weather info",
				"parameters": {
					"type": "object",
					"properties": {
						"city": {"type": "string", "description": "City name"}
					},
					"required": ["city"]
				}
			}]
		}]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if len(req.Config.Tools) != 1 {
		t.Fatalf("Tools len = %d", len(req.Config.Tools))
	}
	tool := req.Config.Tools[0]
	if tool.Name != "get_weather" {
		t.Errorf("tool.Name = %q", tool.Name)
	}
	if tool.Description != "Get weather info" {
		t.Errorf("tool.Description = %q", tool.Description)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("解析 InputSchema 失败: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("schema type = %v", schema["type"])
	}
	requiredField, ok := schema["required"].([]any)
	if !ok || len(requiredField) != 1 || requiredField[0] != "city" {
		t.Errorf("Required = %v", schema["required"])
	}
}

func TestParseRequest_ToolConfig_Mode(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"AUTO", "auto"},
		{"NONE", "none"},
		{"ANY", "required"},
	}

	for _, tt := range tests {
		body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"toolConfig":{"functionCallingConfig":{"mode":"` + tt.mode + `"}}}`
		req, err := parseRequest([]byte(body))
		if err != nil {
			t.Fatalf("parseRequest(%s) error = %v", tt.mode, err)
		}
		if req.Config.ToolChoice != tt.want {
			t.Errorf("mode %q → ToolChoice = %q, want %q", tt.mode, req.Config.ToolChoice, tt.want)
		}
	}
}

func TestParseRequest_GenerationConfig(t *testing.T) {
	body := `{
		"contents": [{"role":"user","parts":[{"text":"hi"}]}],
		"generationConfig": {
			"temperature": 0.7,
			"topP": 0.9,
			"topK": 40,
			"maxOutputTokens": 4096,
			"stopSequences": ["END"],
			"responseMimeType": "application/json"
		}
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	cfg := req.Config

	if cfg.Temperature == nil || *cfg.Temperature != 0.7 {
		t.Errorf("Temperature = %v", cfg.Temperature)
	}
	if cfg.TopP == nil || *cfg.TopP != 0.9 {
		t.Errorf("TopP = %v", cfg.TopP)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", cfg.MaxTokens)
	}
	if len(cfg.StopSequences) != 1 || cfg.StopSequences[0] != "END" {
		t.Errorf("StopSequences = %v", cfg.StopSequences)
	}
	if cfg.ResponseFormat != "json_object" {
		t.Errorf("ResponseFormat = %q, want json_object", cfg.ResponseFormat)
	}
	// topK → ProviderExtra
	topK, ok := cfg.ProviderExtra["top_k"]
	if !ok {
		t.Fatal("ProviderExtra[top_k] not found")
	}
	if topK != float64(40) {
		t.Errorf("topK = %v, want 40", topK)
	}
}

func TestParseRequest_GenerationConfig_ResponseMimeType_TextPlain(t *testing.T) {
	body := `{
		"contents": [{"role":"user","parts":[{"text":"hi"}]}],
		"generationConfig": {"responseMimeType": "text/plain"}
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if req.Config.ResponseFormat != "text" {
		t.Errorf("ResponseFormat = %q, want text", req.Config.ResponseFormat)
	}
}

func TestParseRequest_InvalidJSON(t *testing.T) {
	body := `{invalid json`
	_, err := parseRequest([]byte(body))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseRequest_SafetySettings_Transparent(t *testing.T) {
	body := `{
		"contents": [{"role":"user","parts":[{"text":"hi"}]}],
		"safetySettings": [{"category":"HARM_CATEGORY_DANGEROUS_CONTENT","threshold":"BLOCK_ONLY_HIGH"}]
	}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra should not be nil")
	}
	ss, ok := req.Config.ProviderExtra["safety_settings"]
	if !ok {
		t.Fatal("safety_settings not in ProviderExtra")
	}
	// safety_settings 现在应该是 []*genai.SafetySetting 类型
	settings, ok := ss.([]*genai.SafetySetting)
	if !ok {
		t.Fatalf("safety_settings type = %T, want []*genai.SafetySetting", ss)
	}
	if len(settings) != 1 {
		t.Fatalf("settings count = %d, want 1", len(settings))
	}
}

func TestParseRequest_EmptyContents(t *testing.T) {
	body := `{"contents":[]}`
	req, err := parseRequest([]byte(body))
	if err != nil {
		t.Fatalf("parseRequest error = %v", err)
	}
	if len(req.Messages) != 0 {
		t.Errorf("Messages len = %d, want 0", len(req.Messages))
	}
}

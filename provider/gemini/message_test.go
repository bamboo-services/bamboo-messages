package gemini

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

func TestBuildMessages_ParallelToolsMergedIntoOneUserContent(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{Role: provider.RoleUser, Content: "do both"},
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "id_read", Type: "function", Function: provider.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}},
				{ID: "id_run", Type: "function", Function: provider.FunctionCall{Name: "run_terminal_command", Arguments: `{"cmd":"ls"}`}},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "id_read", Content: "file contents"},
		{Role: provider.RoleTool, ToolCallID: "id_run", Content: "ok"},
	})

	if len(contents) != 3 {
		t.Fatalf("contents len = %d, want 3 (user + model + merged FR)", len(contents))
	}
	assertRole(t, contents[1], "model")
	assertRole(t, contents[2], "user")

	fcs := functionCalls(t, contents[1])
	if len(fcs) != 2 {
		t.Fatalf("functionCall count = %d, want 2", len(fcs))
	}
	if fcs[0]["name"] != "read_file" || fcs[1]["name"] != "run_terminal_command" {
		t.Errorf("functionCall names = %v, %v", fcs[0]["name"], fcs[1]["name"])
	}

	frs := functionResponses(t, contents[2])
	if len(frs) != 2 {
		t.Fatalf("functionResponse count = %d, want 2 (merged into one content)", len(frs))
	}
	if frs[0]["name"] != "read_file" || frs[1]["name"] != "run_terminal_command" {
		t.Errorf("functionResponse names = %v, %v; must match functionCall order", frs[0]["name"], frs[1]["name"])
	}
	if frs[0]["id"] != "id_read" || frs[1]["id"] != "id_run" {
		t.Errorf("functionResponse ids = %v, %v", frs[0]["id"], frs[1]["id"])
	}
}

func TestBuildMessages_ReordersOutOfOrderToolResults(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "id_read", Type: "function", Function: provider.FunctionCall{Name: "read_file"}},
				{ID: "id_run", Type: "function", Function: provider.FunctionCall{Name: "run_terminal_command"}},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "id_run", ToolName: "run_terminal_command", Content: "ok"},
		{Role: provider.RoleTool, ToolCallID: "id_read", ToolName: "read_file", Content: "file"},
	})

	if len(contents) != 2 {
		t.Fatalf("contents len = %d, want 2", len(contents))
	}
	frs := functionResponses(t, contents[1])
	if len(frs) != 2 {
		t.Fatalf("functionResponse count = %d, want 2", len(frs))
	}
	if frs[0]["name"] != "read_file" {
		t.Errorf("parts[0].name = %v, want read_file (FC order, not arrival order)", frs[0]["name"])
	}
	if frs[1]["name"] != "run_terminal_command" {
		t.Errorf("parts[1].name = %v, want run_terminal_command", frs[1]["name"])
	}
}

func TestBuildMessages_MissingToolResultInjectsDummy(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "id_read", Type: "function", Function: provider.FunctionCall{Name: "read_file"}},
				{ID: "id_run", Type: "function", Function: provider.FunctionCall{Name: "run_terminal_command"}},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "id_run", Content: "ok"},
	})

	frs := functionResponses(t, contents[1])
	if len(frs) != 2 {
		t.Fatalf("functionResponse count = %d, want 2", len(frs))
	}
	if frs[0]["name"] != "read_file" {
		t.Errorf("dummy FR name = %v, want read_file", frs[0]["name"])
	}
	resp := unmarshalResponse(t, frs[0])
	if resp["error"] != "tool result missing" {
		t.Errorf("dummy FR error = %v, want tool result missing", resp["error"])
	}
	if _, hasOutput := resp["output"]; hasOutput {
		t.Error("dummy FR should not include output")
	}

	real := unmarshalResponse(t, frs[1])
	if real["output"] != "ok" {
		t.Errorf("kept FR output = %v, want ok", real["output"])
	}
}

func TestBuildMessages_AdjacentAssistantFunctionCallsAreClosed(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "id_read", Type: "function", Function: provider.FunctionCall{Name: "read_file"}},
			},
		},
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "id_run", Type: "function", Function: provider.FunctionCall{Name: "run_terminal_command"}},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "id_read", Content: "file"},
		{Role: provider.RoleTool, ToolCallID: "id_run", Content: "ok"},
	})

	if len(contents) != 4 {
		t.Fatalf("contents len = %d, want 4 (model, user, model, user)", len(contents))
	}
	assertRole(t, contents[0], "model")
	assertRole(t, contents[1], "user")
	assertRole(t, contents[2], "model")
	assertRole(t, contents[3], "user")

	fr1 := functionResponses(t, contents[1])
	fr2 := functionResponses(t, contents[3])
	if len(fr1) != 1 || fr1[0]["name"] != "read_file" {
		t.Errorf("first FR = %v, want read_file", fr1)
	}
	if len(fr2) != 1 || fr2[0]["name"] != "run_terminal_command" {
		t.Errorf("second FR = %v, want run_terminal_command", fr2)
	}
}

func TestBuildMessages_EmptyToolNameUsesFunctionNameNotCallID(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "toolu_01abc", Type: "function", Function: provider.FunctionCall{Name: "read_file"}},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "toolu_01abc", Content: "file"},
	})

	frs := functionResponses(t, contents[1])
	if frs[0]["name"] != "read_file" {
		t.Errorf("functionResponse.name = %v, want read_file (not tool_use_id)", frs[0]["name"])
	}
	if frs[0]["id"] != "toolu_01abc" {
		t.Errorf("functionResponse.id = %v, want toolu_01abc", frs[0]["id"])
	}
}

func TestBuildMessages_DropsOrphanToolResult(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
		{Role: provider.RoleTool, ToolCallID: "orphan", ToolName: "read_file", Content: "nope"},
	})

	if len(contents) != 1 {
		t.Fatalf("contents len = %d, want 1 (orphan FR dropped)", len(contents))
	}
	assertRole(t, contents[0], "user")
	if _, ok := contents[0]["parts"].([]map[string]any)[0]["text"]; !ok {
		t.Error("expected remaining user text content")
	}
}

func TestBuildMessages_UserTextDoesNotSplitFunctionTurn(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "id_read", Type: "function", Function: provider.FunctionCall{Name: "read_file"}},
			},
		},
		{Role: provider.RoleUser, Content: "please continue"},
		{Role: provider.RoleTool, ToolCallID: "id_read", Content: "file"},
	})

	if len(contents) != 3 {
		t.Fatalf("contents len = %d, want 3", len(contents))
	}
	assertRole(t, contents[0], "model")
	assertRole(t, contents[1], "user")
	assertRole(t, contents[2], "user")

	frs := functionResponses(t, contents[1])
	if len(frs) != 1 || frs[0]["name"] != "read_file" {
		t.Errorf("FR must immediately follow model FC, got %v", contents[1])
	}
	textParts := contents[2]["parts"].([]map[string]any)
	if textParts[0]["text"] != "please continue" {
		t.Errorf("trailing user text = %v", textParts[0]["text"])
	}
}

func TestBuildMessages_ErrorToolResultKeepsErrorFlag(t *testing.T) {
	p := NewProvider("test-key")
	contents := p.buildMessages([]provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{ID: "id_run", Type: "function", Function: provider.FunctionCall{Name: "run_terminal_command"}},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "id_run", Content: "boom", IsError: true},
	})

	resp := unmarshalResponse(t, functionResponses(t, contents[1])[0])
	if resp["output"] != "boom" || resp["error"] != "boom" {
		t.Errorf("error FR = %v, want output+error", resp)
	}
}

func TestBuildThoughtPart_RequiresGeminiCredential(t *testing.T) {
	p := NewProvider("test-api-key")

	unsigned := p.buildAssistantMessage(provider.Message{
		Role:            provider.RoleAssistant,
		Content:         "answer",
		ThinkingContent: "from chat",
	})
	parts, _ := unsigned["parts"].([]map[string]any)
	for _, part := range parts {
		if thought, _ := part["thought"].(bool); thought {
			t.Fatalf("unsigned thinking must not emit thought:true, got %#v", parts)
		}
	}

	foreign := p.buildAssistantMessage(provider.Message{
		Role:                      provider.RoleAssistant,
		Content:                   "answer",
		ThinkingContent:           "from claude",
		ThinkingSignature:         "claude_sig",
		ThinkingSignatureProvider: provider.SignatureProviderAnthropic,
	})
	parts, _ = foreign["parts"].([]map[string]any)
	for _, part := range parts {
		if thought, _ := part["thought"].(bool); thought {
			t.Fatalf("foreign signature must not emit thought:true, got %#v", parts)
		}
	}

	native := p.buildAssistantMessage(provider.Message{
		Role:                      provider.RoleAssistant,
		Content:                   "answer",
		ThinkingContent:           "gemini think",
		ThinkingSignature:         "g_sig",
		ThinkingSignatureProvider: provider.SignatureProviderGemini,
	})
	parts, _ = native["parts"].([]map[string]any)
	if len(parts) < 2 {
		t.Fatalf("expected thought + text parts, got %#v", parts)
	}
	if thought, _ := parts[0]["thought"].(bool); !thought {
		t.Fatalf("native gemini credential should emit thought:true, got %#v", parts)
	}
	if parts[0]["thoughtSignature"] != "g_sig" {
		t.Errorf("thoughtSignature = %v", parts[0]["thoughtSignature"])
	}
}

// TestBuildMessages_ToolResponseNameFallbackFromToolCallID 验证当 RoleTool 的 ToolName 为空时，
// 仍从对应 ToolCall.Function.Name 取 functionResponse.name，且 FR 紧跟 model 合成一条 user content。
func TestBuildMessages_ToolResponseNameFallbackFromToolCallID(t *testing.T) {
	p := NewProvider("test-api-key")

	msgs := []provider.Message{
		{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				{
					ID:   "call_226596",
					Type: "function",
					Function: provider.FunctionCall{
						Name:      "run_terminal_command",
						Arguments: `{"command":"pwd"}`,
					},
				},
			},
		},
		{
			Role:       provider.RoleTool,
			ToolCallID: "call_226596",
			ToolName:   "", // 故意为空（OpenAI role=tool 协议常见现象）
			Content:    "/workspace",
		},
	}

	result := p.buildMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	funcMsg := result[1]
	if funcMsg["role"] != "user" {
		t.Errorf("expected role=user, got %v", funcMsg["role"])
	}
	parts, ok := funcMsg["parts"].([]map[string]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("expected parts in function response content, got %#v", funcMsg)
	}

	funcResp, ok := parts[0]["functionResponse"].(map[string]any)
	if !ok {
		t.Fatalf("expected functionResponse in part, got %#v", parts[0])
	}

	if funcResp["name"] != "run_terminal_command" {
		t.Errorf("functionResponse.name = %q, want %q", funcResp["name"], "run_terminal_command")
	}
	if funcResp["id"] != "call_226596" {
		t.Errorf("functionResponse.id = %q, want %q", funcResp["id"], "call_226596")
	}
}

// TestBuildAssistantMessage_SignatureOnlyFunctionCall 验证 host-tool hop2 的典型形态：
// Gemini 把 thoughtSignature 打在 functionCall 同一 part 上，IR 拆成空 ThinkingBlock + ToolUse。
// 回灌时签名必须挂回 functionCall，禁止发出无 data oneof 的 {thought:true} part
// （上游会 500：Unsupported input part type: go/debugstr）。
func TestBuildAssistantMessage_SignatureOnlyFunctionCall(t *testing.T) {
	p := NewProvider("test-api-key")
	got := p.buildAssistantMessage(provider.Message{
		Role:                      provider.RoleAssistant,
		ThinkingSignature:         "g_sig",
		ThinkingSignatureProvider: provider.SignatureProviderGemini,
		ToolCalls: []provider.ToolCall{{
			ID:   "call_search",
			Type: "function",
			Function: provider.FunctionCall{
				Name:      "web_search",
				Arguments: `{"query":"weather"}`,
			},
		}},
	})

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal assistant message: %v", err)
	}
	var wire struct {
		Role  string           `json:"role"`
		Parts []map[string]any `json:"parts"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(wire.Parts) != 1 {
		t.Fatalf("parts = %#v, want 1 functionCall part", wire.Parts)
	}
	if _, hasText := wire.Parts[0]["text"]; hasText {
		t.Fatalf("signature-only thinking must not emit text/thought part, got %#v", wire.Parts[0])
	}
	if thought, _ := wire.Parts[0]["thought"].(bool); thought {
		t.Fatalf("empty thought part would be rejected by Gemini, got %#v", wire.Parts[0])
	}
	if wire.Parts[0]["thoughtSignature"] != "g_sig" {
		t.Errorf("thoughtSignature = %v, want g_sig on functionCall part", wire.Parts[0]["thoughtSignature"])
	}
	fc, ok := wire.Parts[0]["functionCall"].(map[string]any)
	if !ok {
		t.Fatalf("expected functionCall, got %#v", wire.Parts[0])
	}
	if fc["name"] != "web_search" {
		t.Errorf("functionCall.name = %v", fc["name"])
	}
	args, _ := fc["args"].(map[string]any)
	if args["query"] != "weather" {
		t.Errorf("functionCall.args = %#v", fc["args"])
	}
}

func TestBuildAssistantMessage_ThinkingTextPlusFunctionCall(t *testing.T) {
	p := NewProvider("test-api-key")
	got := p.buildAssistantMessage(provider.Message{
		Role:                      provider.RoleAssistant,
		ThinkingContent:           "need to search",
		ThinkingSignature:         "g_sig",
		ThinkingSignatureProvider: provider.SignatureProviderGemini,
		ToolCalls: []provider.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: provider.FunctionCall{Name: "web_search", Arguments: `{"query":"x"}`},
		}},
	})
	parts, _ := got["parts"].([]map[string]any)
	if len(parts) != 2 {
		t.Fatalf("parts = %#v, want thought text + functionCall", parts)
	}
	if thought, _ := parts[0]["thought"].(bool); !thought || parts[0]["text"] != "need to search" {
		t.Fatalf("thought part = %#v", parts[0])
	}
	if _, hasSig := parts[0]["thoughtSignature"]; hasSig {
		t.Fatalf("signature should ride on functionCall, got %#v", parts[0])
	}
	if parts[1]["thoughtSignature"] != "g_sig" {
		t.Errorf("functionCall thoughtSignature = %v", parts[1]["thoughtSignature"])
	}
}

func TestBuildImagePart_DataURIUsesInlineData(t *testing.T) {
	part := buildImagePart(provider.ImageContentBlock{
		Source: provider.ImageSource{
			Type: "url",
			URL:  "data:image/png;base64,iVBORw0KGgo=",
		},
	})
	if part == nil {
		t.Fatal("data URI image part is nil")
	}
	if _, hasFile := part["fileData"]; hasFile {
		t.Fatalf("data URI must not go through fileData, got %#v", part)
	}
	inline, ok := part["inlineData"].(map[string]any)
	if !ok {
		t.Fatalf("want inlineData, got %#v", part)
	}
	if inline["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v, want image/png", inline["mimeType"])
	}
	if inline["data"] != "iVBORw0KGgo=" {
		t.Errorf("data = %v, want raw base64 without data URI prefix", inline["data"])
	}
}

func TestBuildImagePart_Base64InlineData(t *testing.T) {
	part := buildImagePart(provider.ImageContentBlock{
		Source: provider.ImageSource{
			Type:      "base64",
			MediaType: "image/jpeg",
			Data:      "/9j/4AAQ",
		},
	})
	inline, _ := part["inlineData"].(map[string]any)
	if inline["mimeType"] != "image/jpeg" || inline["data"] != "/9j/4AAQ" {
		t.Fatalf("inlineData = %#v", inline)
	}
}

func TestBuildImagePart_HTTPURLUsesFileData(t *testing.T) {
	part := buildImagePart(provider.ImageContentBlock{
		Source: provider.ImageSource{
			Type:      "url",
			URL:       "https://example.com/cat.png",
			MediaType: "image/png",
		},
	})
	fileData, ok := part["fileData"].(map[string]any)
	if !ok {
		t.Fatalf("HTTP URL should stay fileData, got %#v", part)
	}
	if fileData["fileUri"] != "https://example.com/cat.png" {
		t.Errorf("fileUri = %v", fileData["fileUri"])
	}
}

func TestBuildImagePart_Base64WrappedAsDataURI(t *testing.T) {
	part := buildImagePart(provider.ImageContentBlock{
		Source: provider.ImageSource{
			Type:      "base64",
			MediaType: "image/png",
			Data:      "data:image/png;base64,iVBORw0KGgo=",
		},
	})
	inline, _ := part["inlineData"].(map[string]any)
	if inline["data"] != "iVBORw0KGgo=" {
		t.Errorf("wrapped data URI must be stripped, got %#v", inline)
	}
}

func TestGeminiFunctionArgs_NonObjectFallsBack(t *testing.T) {
	if string(geminiFunctionArgs("")) != "{}" {
		t.Errorf("empty args = %s", geminiFunctionArgs(""))
	}
	if string(geminiFunctionArgs(`["x"]`)) != "{}" {
		t.Errorf("array args = %s", geminiFunctionArgs(`["x"]`))
	}
	if string(geminiFunctionArgs(`{"q":1}`)) != `{"q":1}` {
		t.Errorf("object args = %s", geminiFunctionArgs(`{"q":1}`))
	}
}

func assertRole(t *testing.T, content map[string]any, want string) {
	t.Helper()
	if content["role"] != want {
		t.Fatalf("role = %v, want %s", content["role"], want)
	}
}

func functionCalls(t *testing.T, content map[string]any) []map[string]any {
	t.Helper()
	parts, _ := content["parts"].([]map[string]any)
	out := make([]map[string]any, 0)
	for _, part := range parts {
		if fc, ok := part["functionCall"].(map[string]any); ok {
			out = append(out, fc)
		}
	}
	return out
}

func functionResponses(t *testing.T, content map[string]any) []map[string]any {
	t.Helper()
	parts, _ := content["parts"].([]map[string]any)
	out := make([]map[string]any, 0)
	for _, part := range parts {
		if fr, ok := part["functionResponse"].(map[string]any); ok {
			out = append(out, fr)
		}
	}
	if len(out) == 0 {
		t.Fatalf("expected functionResponse parts, got %v", content)
	}
	return out
}

func unmarshalResponse(t *testing.T, fr map[string]any) map[string]any {
	t.Helper()
	raw, ok := fr["response"].(json.RawMessage)
	if !ok {
		t.Fatalf("response type = %T, want json.RawMessage", fr["response"])
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return out
}

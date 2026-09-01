package gemini

import (
	"encoding/json"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

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
// buildMessages 能自动根据 ToolCallID 反查前序 Assistant 的 ToolCall 函数名，
// 避免出现 functionResponse.name="call_xxx" 与 functionCall.name 不匹配导致 Gemini 报错。
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

	// 检查 function 消息
	funcMsg := result[1]
	if funcMsg["role"] != "function" {
		t.Errorf("expected role=function, got %v", funcMsg["role"])
	}
	parts, ok := funcMsg["parts"].([]map[string]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("expected parts in function message, got %#v", funcMsg)
	}

	funcResp, ok := parts[0]["functionResponse"].(map[string]any)
	if !ok {
		t.Fatalf("expected functionResponse in part, got %#v", parts[0])
	}

	// 验证 name 必须是函数名 "run_terminal_command"，绝不能是 "call_226596"
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

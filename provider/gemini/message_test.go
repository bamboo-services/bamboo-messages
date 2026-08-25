package gemini

import (
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

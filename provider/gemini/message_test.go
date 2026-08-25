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

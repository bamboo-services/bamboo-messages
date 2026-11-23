package gemini

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestBuildRequestBody_CachedContent 验证 Gemini 显式缓存资源引用的一等字段映射。
//
// cachedContent 是 GenerateContentRequest 的顶层字段，不能写入 generationConfig。
func TestBuildRequestBody_CachedContent(t *testing.T) {
	p := NewProvider("test-api-key")

	t.Run("first-class field", func(t *testing.T) {
		config := &provider.ChatConfig{
			Model:         "gemini-2.5-pro",
			CachedContent: "cachedContents/abc123",
		}
		body := p.buildRequestBody(nil, "", config, false)
		if got := body["cachedContent"]; got != "cachedContents/abc123" {
			t.Errorf("cachedContent = %v, want cachedContents/abc123", got)
		}
		if _, ok := p.buildContentConfig(config)["cachedContent"]; ok {
			t.Error("cachedContent must not be written into generationConfig")
		}
	})

	t.Run("provider extra fallback", func(t *testing.T) {
		config := &provider.ChatConfig{
			Model: "gemini-2.5-pro",
			ProviderExtra: map[string]any{
				"cached_content": "cachedContents/from-extra",
			},
		}
		body := p.buildRequestBody(nil, "", config, false)
		if got := body["cachedContent"]; got != "cachedContents/from-extra" {
			t.Errorf("cachedContent = %v, want cachedContents/from-extra", got)
		}
	})

	t.Run("first-class field takes precedence", func(t *testing.T) {
		config := &provider.ChatConfig{
			Model:         "gemini-2.5-pro",
			CachedContent: "cachedContents/primary",
			ProviderExtra: map[string]any{
				"cached_content": "cachedContents/fallback",
			},
		}
		body := p.buildRequestBody(nil, "", config, false)
		if got := body["cachedContent"]; got != "cachedContents/primary" {
			t.Errorf("cachedContent = %v, want cachedContents/primary", got)
		}
	})

	t.Run("not configured means omitted", func(t *testing.T) {
		config := &provider.ChatConfig{Model: "gemini-2.5-pro"}
		body := p.buildRequestBody(nil, "", config, false)
		if _, ok := body["cachedContent"]; ok {
			t.Error("cachedContent must not be sent when not configured")
		}
	})
}

// TestBuildRequestBody_CachedContentMutualExclusion 验证 cachedContent 与冲突字段的互斥保护。
//
// Gemini API 规定请求体携带 cachedContent 时，systemInstruction / tools / toolConfig
// 不可同时存在（已固化在缓存资源内），否则上游会返回 400；其余字段（contents /
// generationConfig / safetySettings / service_tier）必须正常保留。
func TestBuildRequestBody_CachedContentMutualExclusion(t *testing.T) {
	p := NewProvider("test-api-key")
	temp := 0.5
	messages := []provider.Message{{Role: provider.RoleUser, Content: "hello"}}
	baseConfig := func() *provider.ChatConfig {
		return &provider.ChatConfig{
			Model:       "gemini-2.5-pro",
			Temperature: &temp,
			ServiceTier: "priority",
			Tools: []provider.Tool{
				{
					Type:     "function",
					Function: provider.FunctionDef{Name: "get_weather", Description: "Get weather"},
				},
			},
			ToolChoice: "forced",
			ProviderExtra: map[string]any{
				"forced_tool_name": "get_weather",
				"safety_settings": []map[string]string{
					{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
				},
			},
		}
	}

	t.Run("first-class cachedContent strips conflicting fields", func(t *testing.T) {
		cfg := baseConfig()
		cfg.CachedContent = "cachedContents/item-1"

		body := p.buildRequestBody(messages, "you are helpful", cfg, false)

		if body["cachedContent"] != "cachedContents/item-1" {
			t.Errorf("cachedContent = %v, want cachedContents/item-1", body["cachedContent"])
		}
		for _, forbidden := range []string{"systemInstruction", "tools", "toolConfig"} {
			if _, exists := body[forbidden]; exists {
				t.Errorf("%s must be stripped when cachedContent is set, got %v", forbidden, body[forbidden])
			}
		}
		for _, preserved := range []string{"contents", "generationConfig", "safetySettings", "service_tier"} {
			if _, exists := body[preserved]; !exists {
				t.Errorf("%s must be preserved when cachedContent is set", preserved)
			}
		}
	})

	t.Run("provider extra cached_content strips conflicting fields", func(t *testing.T) {
		cfg := baseConfig()
		cfg.ProviderExtra["cached_content"] = "cachedContents/from-extra"

		body := p.buildRequestBody(messages, "you are helpful", cfg, true)

		if body["cachedContent"] != "cachedContents/from-extra" {
			t.Errorf("cachedContent = %v, want cachedContents/from-extra", body["cachedContent"])
		}
		for _, forbidden := range []string{"systemInstruction", "tools", "toolConfig"} {
			if _, exists := body[forbidden]; exists {
				t.Errorf("%s must be stripped when ProviderExtra cached_content is set", forbidden)
			}
		}
	})

	t.Run("without cachedContent preserves systemInstruction, tools, and toolConfig", func(t *testing.T) {
		cfg := baseConfig()

		body := p.buildRequestBody(messages, "you are helpful", cfg, false)

		if _, exists := body["cachedContent"]; exists {
			t.Error("cachedContent must be omitted when not set")
		}
		for _, required := range []string{"systemInstruction", "tools", "toolConfig", "contents", "generationConfig", "safetySettings"} {
			if _, exists := body[required]; !exists {
				t.Errorf("%s must be present when cachedContent is empty", required)
			}
		}
	})
}

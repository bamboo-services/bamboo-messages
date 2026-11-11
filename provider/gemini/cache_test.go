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

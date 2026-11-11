package completions

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// explicitCacheConfig 构造一份显式启用块级缓存断点的请求配置。
//
// prompt_cache_breakpoint 是 gpt-5.6+ 的新字段，因此块级断点必须由调用方
// 显式确认目标模型支持后才发射。
func explicitCacheConfig() *provider.ChatConfig {
	return &provider.ChatConfig{
		Model: "gpt-5.6",
		PromptCacheOptions: &provider.PromptCacheOptions{
			Mode:                   provider.PromptCacheModeExplicit,
			TTL:                    provider.CacheTTL30m,
			EnableBlockBreakpoints: true,
		},
	}
}

// TestBuildParams_PromptCacheOptions 验证请求级缓存选项的映射。
//
// Chat Completions 的 prompt_cache_options.ttl 当前仅支持 "30m"，
// 请求级取值超出白名单时必须整体省略，避免上游 400。
func TestBuildParams_PromptCacheOptions(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	t.Run("explicit mode + ttl", func(t *testing.T) {
		params := p.buildParams("", nil, explicitCacheConfig())
		opts, ok := params["prompt_cache_options"].(map[string]any)
		if !ok {
			t.Fatalf("prompt_cache_options missing, params = %v", params)
		}
		if opts["mode"] != string(provider.PromptCacheModeExplicit) {
			t.Errorf("mode = %v, want explicit", opts["mode"])
		}
		if opts["ttl"] != string(provider.CacheTTL30m) {
			t.Errorf("ttl = %v, want 30m", opts["ttl"])
		}
	})

	t.Run("unsupported request ttl is dropped", func(t *testing.T) {
		config := explicitCacheConfig()
		config.PromptCacheOptions.TTL = provider.CacheTTL12h
		params := p.buildParams("", nil, config)
		opts, ok := params["prompt_cache_options"].(map[string]any)
		if !ok {
			t.Fatalf("prompt_cache_options missing, params = %v", params)
		}
		if _, exists := opts["ttl"]; exists {
			t.Errorf("ttl must be omitted for Chat Completions (only 30m supported), got %v", opts["ttl"])
		}
	})

	t.Run("retention is emitted", func(t *testing.T) {
		config := &provider.ChatConfig{
			Model: "gpt-5.5",
			PromptCacheOptions: &provider.PromptCacheOptions{
				Retention: provider.PromptCacheRetention24h,
			},
		}
		params := p.buildParams("", nil, config)
		if got := params["prompt_cache_retention"]; got != string(provider.PromptCacheRetention24h) {
			t.Errorf("prompt_cache_retention = %v, want 24h", got)
		}
	})

	t.Run("no cache options means no fields", func(t *testing.T) {
		params := p.buildParams("", nil, &provider.ChatConfig{Model: "gpt-4o"})
		if _, ok := params["prompt_cache_options"]; ok {
			t.Error("prompt_cache_options must not be sent when not configured")
		}
		if _, ok := params["prompt_cache_retention"]; ok {
			t.Error("prompt_cache_retention must not be sent when not configured")
		}
	})
}

// TestBuildMessages_CacheBreakpoint 验证块级缓存断点的挂载位置。
//
// 断点只能挂在内容块上，因此承载断点的消息必须使用 content 数组形式；
// 未启用块级断点时保持原有字符串形式，不发射任何断点字段。
func TestBuildMessages_CacheBreakpoint(t *testing.T) {
	p := NewCompletionsProvider("test-api-key")

	messages := []provider.Message{
		{
			Role:         provider.RoleUser,
			Content:      "hello",
			CacheControl: &provider.CacheControl{Type: "ephemeral", TTL: provider.CacheTTL30m},
		},
	}

	t.Run("breakpoint enabled", func(t *testing.T) {
		result := p.buildMessages("", messages, explicitCacheConfig())
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		parts, ok := result[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("content must be an array to carry a breakpoint, got %T", result[0]["content"])
		}
		bp, ok := parts[0]["prompt_cache_breakpoint"].(map[string]any)
		if !ok {
			t.Fatalf("prompt_cache_breakpoint missing on content part: %v", parts[0])
		}
		if bp["mode"] != "explicit" {
			t.Errorf("breakpoint mode = %v, want explicit", bp["mode"])
		}
		if bp["ttl"] != string(provider.CacheTTL30m) {
			t.Errorf("breakpoint ttl = %v, want 30m", bp["ttl"])
		}
	})

	t.Run("breakpoint disabled keeps string content", func(t *testing.T) {
		result := p.buildMessages("", messages, &provider.ChatConfig{Model: "gpt-4o"})
		content, ok := result[0]["content"].(string)
		if !ok {
			t.Fatalf("content must stay a string when breakpoints are disabled, got %T", result[0]["content"])
		}
		if content != "hello" {
			t.Errorf("content = %q, want hello", content)
		}
	})

	t.Run("system breakpoint uses array form", func(t *testing.T) {
		config := explicitCacheConfig()
		config.SystemCacheControl = &provider.CacheControl{Type: "ephemeral"}
		result := p.buildMessages("system prompt", nil, config)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		parts, ok := result[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("system content must be an array to carry a breakpoint, got %T", result[0]["content"])
		}
		if _, ok := parts[0]["prompt_cache_breakpoint"].(map[string]any); !ok {
			t.Errorf("prompt_cache_breakpoint missing on system content part: %v", parts[0])
		}
	})

	t.Run("12h breakpoint ttl is supported", func(t *testing.T) {
		config := explicitCacheConfig()
		msgs := []provider.Message{
			{
				Role:         provider.RoleUser,
				Content:      "hello",
				CacheControl: &provider.CacheControl{Type: "ephemeral", TTL: provider.CacheTTL12h},
			},
		}
		result := p.buildMessages("", msgs, config)
		parts, ok := result[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("content must be an array to carry a breakpoint, got %T", result[0]["content"])
		}
		bp, ok := parts[0]["prompt_cache_breakpoint"].(map[string]any)
		if !ok {
			t.Fatalf("prompt_cache_breakpoint missing: %v", parts[0])
		}
		if bp["ttl"] != string(provider.CacheTTL12h) {
			t.Errorf("breakpoint ttl = %v, want 12h", bp["ttl"])
		}
	})

	t.Run("unsupported breakpoint ttl falls back to request ttl", func(t *testing.T) {
		config := explicitCacheConfig()
		msgs := []provider.Message{
			{
				Role:         provider.RoleUser,
				Content:      "hello",
				CacheControl: &provider.CacheControl{Type: "ephemeral", TTL: provider.CacheTTL1h},
			},
		}
		result := p.buildMessages("", msgs, config)
		parts, ok := result[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("content must be an array to carry a breakpoint, got %T", result[0]["content"])
		}
		bp, ok := parts[0]["prompt_cache_breakpoint"].(map[string]any)
		if !ok {
			t.Fatalf("prompt_cache_breakpoint missing: %v", parts[0])
		}
		if _, exists := bp["ttl"]; exists {
			t.Errorf("breakpoint ttl must be omitted for unsupported value, got %v", bp["ttl"])
		}
	})
}

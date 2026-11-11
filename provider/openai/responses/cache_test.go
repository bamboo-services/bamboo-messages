package responses

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
// Responses 的 prompt_cache_options.ttl 支持 "30m" / "12h"（与 Chat Completions
// 不同），另外还支持 prewarm 与 comparison_response_id 两个专属字段。
func TestBuildParams_PromptCacheOptions(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	t.Run("explicit mode + ttl + prewarm", func(t *testing.T) {
		config := explicitCacheConfig()
		config.ProviderExtra = map[string]any{
			"prewarm":                true,
			"comparison_response_id": "resp_123",
		}
		params := p.buildParams("gpt-5.6", "", nil, config, false)
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
		if opts["prewarm"] != true {
			t.Errorf("prewarm = %v, want true", opts["prewarm"])
		}
		if opts["comparison_response_id"] != "resp_123" {
			t.Errorf("comparison_response_id = %v, want resp_123", opts["comparison_response_id"])
		}
	})

	t.Run("12h request ttl is supported", func(t *testing.T) {
		config := explicitCacheConfig()
		config.PromptCacheOptions.TTL = provider.CacheTTL12h
		params := p.buildParams("gpt-5.6", "", nil, config, false)
		opts, ok := params["prompt_cache_options"].(map[string]any)
		if !ok {
			t.Fatalf("prompt_cache_options missing, params = %v", params)
		}
		if opts["ttl"] != string(provider.CacheTTL12h) {
			t.Errorf("ttl = %v, want 12h", opts["ttl"])
		}
	})

	t.Run("retention is emitted", func(t *testing.T) {
		config := &provider.ChatConfig{
			Model: "gpt-5.5",
			PromptCacheOptions: &provider.PromptCacheOptions{
				Retention: provider.PromptCacheRetentionInMemory,
			},
		}
		params := p.buildParams("gpt-5.5", "", nil, config, false)
		if got := params["prompt_cache_retention"]; got != string(provider.PromptCacheRetentionInMemory) {
			t.Errorf("prompt_cache_retention = %v, want in_memory", got)
		}
	})

	t.Run("no cache options means no fields", func(t *testing.T) {
		params := p.buildParams("gpt-4o", "", nil, &provider.ChatConfig{Model: "gpt-4o"}, false)
		if _, ok := params["prompt_cache_options"]; ok {
			t.Error("prompt_cache_options must not be sent when not configured")
		}
		if _, ok := params["prompt_cache_retention"]; ok {
			t.Error("prompt_cache_retention must not be sent when not configured")
		}
	})
}

// TestBuildInput_CacheBreakpoint 验证块级缓存断点挂在 input_text 块上。
//
// Responses 的断点只能挂在输入类内容块上（input_text / input_image / input_file），
// assistant 的 output_text 不支持断点。
func TestBuildInput_CacheBreakpoint(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	messages := []provider.Message{
		{
			Role:         provider.RoleUser,
			Content:      "hello",
			CacheControl: &provider.CacheControl{Type: "ephemeral", TTL: provider.CacheTTL12h},
		},
	}

	t.Run("breakpoint enabled", func(t *testing.T) {
		items := p.buildInput(messages, explicitCacheConfig())
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		parts, ok := items[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("content must be an array, got %T", items[0]["content"])
		}
		bp, ok := parts[0]["prompt_cache_breakpoint"].(map[string]any)
		if !ok {
			t.Fatalf("prompt_cache_breakpoint missing on content part: %v", parts[0])
		}
		if bp["mode"] != "explicit" {
			t.Errorf("breakpoint mode = %v, want explicit", bp["mode"])
		}
		if bp["ttl"] != string(provider.CacheTTL12h) {
			t.Errorf("breakpoint ttl = %v, want 12h", bp["ttl"])
		}
	})

	t.Run("breakpoint disabled", func(t *testing.T) {
		items := p.buildInput(messages, &provider.ChatConfig{Model: "gpt-4o"})
		parts, ok := items[0]["content"].([]map[string]any)
		if !ok {
			t.Fatalf("content must be an array, got %T", items[0]["content"])
		}
		if _, exists := parts[0]["prompt_cache_breakpoint"]; exists {
			t.Errorf("prompt_cache_breakpoint must not be emitted when disabled: %v", parts[0])
		}
	})
}

// TestBuildParams_SystemCacheUsesDeveloperMessage 验证系统提示的缓存断点。
//
// Responses 的顶层 instructions 是纯字符串，无法携带缓存断点；官方要求把
// 可复用的 developer 指令放进 developer 消息的 input_text 块里。
func TestBuildParams_SystemCacheUsesDeveloperMessage(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	config := explicitCacheConfig()
	config.SystemCacheControl = &provider.CacheControl{Type: "ephemeral"}

	params := p.buildParams("gpt-5.6", "system prompt", nil, config, false)
	if _, ok := params["instructions"]; ok {
		t.Error("instructions must not be used when the system prompt carries a cache breakpoint")
	}
	items, ok := params["input"].([]map[string]any)
	if !ok {
		t.Fatalf("input must be an array, got %T", params["input"])
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(items))
	}
	if items[0]["role"] != "developer" {
		t.Errorf("role = %v, want developer", items[0]["role"])
	}
	parts, ok := items[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content must be an array, got %T", items[0]["content"])
	}
	if _, ok := parts[0]["prompt_cache_breakpoint"].(map[string]any); !ok {
		t.Errorf("prompt_cache_breakpoint missing on developer content part: %v", parts[0])
	}
}

// TestBuildParams_SystemWithoutBreakpointKeepsInstructions 验证无断点时仍用 instructions。
func TestBuildParams_SystemWithoutBreakpointKeepsInstructions(t *testing.T) {
	p := NewResponsesProvider("test-api-key")

	params := p.buildParams("gpt-5.6", "system prompt", nil, &provider.ChatConfig{Model: "gpt-5.6"}, false)
	if got := params["instructions"]; got != "system prompt" {
		t.Errorf("instructions = %v, want system prompt", got)
	}
	items, ok := params["input"].([]map[string]any)
	if !ok {
		t.Fatalf("input must be an array, got %T", params["input"])
	}
	if len(items) != 0 {
		t.Errorf("input must stay empty, got %v", items)
	}
}

package openai

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestParseRequest_PromptCacheOptions 验证请求级缓存选项的解析。
//
// 入站 OpenAI 请求携带 prompt_cache_options / prompt_cache_retention 时，
// 必须解析为统一的 PromptCacheOptions，并开启块级断点透传，
// 保证入站的 prompt_cache_breakpoint 能在出站协议中保留。
func TestParseRequest_PromptCacheOptions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.6",
		"messages": [{"role": "user", "content": "hello"}],
		"prompt_cache_options": {"mode": "explicit", "ttl": "30m"},
		"prompt_cache_retention": "24h"
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	pco := req.Config.PromptCacheOptions
	if pco == nil {
		t.Fatal("PromptCacheOptions is nil")
	}
	if pco.Mode != provider.PromptCacheModeExplicit {
		t.Errorf("Mode = %v, want explicit", pco.Mode)
	}
	if pco.TTL != provider.CacheTTL30m {
		t.Errorf("TTL = %v, want 30m", pco.TTL)
	}
	if pco.Retention != provider.PromptCacheRetention24h {
		t.Errorf("Retention = %v, want 24h", pco.Retention)
	}
	if !pco.EnableBlockBreakpoints {
		t.Error("EnableBlockBreakpoints = false, want true")
	}
}

// TestParseRequest_NoPromptCacheOptions 验证未配置缓存选项时不产生额外字段。
func TestParseRequest_NoPromptCacheOptions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hello"}]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	if req.Config.PromptCacheOptions != nil {
		t.Errorf("PromptCacheOptions = %v, want nil", req.Config.PromptCacheOptions)
	}
}

// TestParseRequest_CacheBreakpoint 验证块级缓存断点的解析。
//
// 内容块上的 prompt_cache_breakpoint 必须映射为统一的块级缓存标记，
// system 消息上的断点映射为 system 级缓存标记。
func TestParseRequest_CacheBreakpoint(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.6",
		"messages": [
			{
				"role": "system",
				"content": [{"type": "text", "text": "system prompt", "prompt_cache_breakpoint": {"mode": "explicit", "ttl": "30m"}}]
			},
			{
				"role": "user",
				"content": [{"type": "text", "text": "hello", "prompt_cache_breakpoint": {"mode": "explicit", "ttl": "12h"}}]
			}
		],
		"prompt_cache_options": {"mode": "explicit"}
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	// system 级断点
	cc := req.Config.SystemCacheControl
	if cc == nil {
		t.Fatal("SystemCacheControl is nil")
	}
	if cc.TTL != provider.CacheTTL30m {
		t.Errorf("SystemCacheControl.TTL = %v, want 30m", cc.TTL)
	}

	// user 块级断点
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	blocks := req.Messages[0].Content
	if len(blocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(blocks))
	}
	tb, ok := blocks[0].(*bamboo.TextBlock)
	if !ok {
		t.Fatalf("unexpected block type %T", blocks[0])
	}
	if tb.CacheControl == nil {
		t.Fatal("block CacheControl is nil")
	}
	if tb.CacheControl.TTL != provider.CacheTTL12h {
		t.Errorf("block CacheControl.TTL = %v, want 12h", tb.CacheControl.TTL)
	}
}

// TestParseRequest_ProviderExtraNotOverwritten 验证 ProviderExtra 增量写入不会被后续参数覆盖。
//
// 回归场景：同时携带 tool_choice (forced_tool_name)、cached_content 与
// frequency_penalty / presence_penalty / seed 时，此前 config.ProviderExtra = extra
// 会把前面写入的键全部清空。
func TestParseRequest_ProviderExtraNotOverwritten(t *testing.T) {
	body := []byte(`{
		"model": "gemini-2.5-pro",
		"messages": [{"role": "user", "content": "hello"}],
		"tools": [{"type": "function", "function": {"name": "get_weather"}}],
		"tool_choice": {"type": "function", "function": {"name": "get_weather"}},
		"cached_content": "cachedContents/item-1",
		"frequency_penalty": 0.2,
		"presence_penalty": 0.3,
		"seed": 42
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}
	extra := req.Config.ProviderExtra
	if extra == nil {
		t.Fatal("ProviderExtra is nil")
	}
	if got := extra["forced_tool_name"]; got != "get_weather" {
		t.Errorf("forced_tool_name = %v, want get_weather", got)
	}
	if got := extra["cached_content"]; got != "cachedContents/item-1" {
		t.Errorf("cached_content = %v, want cachedContents/item-1", got)
	}
	if got := extra["frequency_penalty"]; got != 0.2 {
		t.Errorf("frequency_penalty = %v, want 0.2", got)
	}
	if got := extra["presence_penalty"]; got != 0.3 {
		t.Errorf("presence_penalty = %v, want 0.3", got)
	}
	if got := extra["seed"]; got != int64(42) {
		t.Errorf("seed = %v, want 42", got)
	}
}

// TestParseRequest_CachedContentVariants 验证 Google cached_content 在多种 Chat 入站写法下的解析。
func TestParseRequest_CachedContentVariants(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "top-level snake_case",
			body: `{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}],"cached_content":"cachedContents/snake"}`,
			want: "cachedContents/snake",
		},
		{
			name: "top-level camelCase",
			body: `{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}],"cachedContent":"cachedContents/camel"}`,
			want: "cachedContents/camel",
		},
		{
			name: "extra_body cached_content",
			body: `{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}],"extra_body":{"cached_content":"cachedContents/eb"}}`,
			want: "cachedContents/eb",
		},
		{
			name: "extra_body google cachedContent",
			body: `{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}],"extra_body":{"google":{"cachedContent":"cachedContents/eb_google"}}}`,
			want: "cachedContents/eb_google",
		},
		{
			name: "top-level snake_case overrides fallback",
			body: `{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}],"cached_content":"cachedContents/primary","cachedContent":"cachedContents/secondary","extra_body":{"cached_content":"cachedContents/eb"}}`,
			want: "cachedContents/primary",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := parseRequest([]byte(tc.body))
			if err != nil {
				t.Fatalf("parseRequest() error = %v", err)
			}
			if req.Config.CachedContent != tc.want {
				t.Errorf("CachedContent = %q, want %q", req.Config.CachedContent, tc.want)
			}
			if got, _ := req.Config.ProviderExtra["cached_content"].(string); got != tc.want {
				t.Errorf("ProviderExtra[cached_content] = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseRequest_CacheControlCompat 验证兼容风格的 cache_control 在各层级与角色的解析。
func TestParseRequest_CacheControlCompat(t *testing.T) {
	t.Run("system part, user text/image, assistant, tool, tools", func(t *testing.T) {
		body := []byte(`{
			"model": "claude-sonnet-4",
			"messages": [
				{
					"role": "system",
					"content": [{"type": "text", "text": "sys", "cache_control": {"type": "ephemeral", "ttl": "1h"}}]
				},
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "look", "cache_control": {"type": "ephemeral", "ttl": "5m"}},
						{"type": "image_url", "image_url": {"url": "https://example.com/a.png"}, "cache_control": {"type": "ephemeral"}}
					]
				},
				{
					"role": "assistant",
					"content": [{"type": "text", "text": "calling", "cache_control": {"type": "ephemeral"}}],
					"tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "search", "arguments": "{}"}}]
				},
				{
					"role": "tool",
					"tool_call_id": "call_1",
					"content": [{"type": "text", "text": "ok", "cache_control": {"type": "ephemeral", "ttl": "1h"}}]
				}
			],
			"tools": [
				{
					"type": "function",
					"function": {"name": "search", "description": "search web"},
					"cache_control": {"type": "ephemeral", "ttl": "5m"}
				},
				{
					"type": "function",
					"function": {"name": "lookup", "cache_control": {"type": "ephemeral"}}
				}
			]
		}`)

		req, err := parseRequest(body)
		if err != nil {
			t.Fatalf("parseRequest() error = %v", err)
		}

		// system
		if req.Config.SystemCacheControl == nil || req.Config.SystemCacheControl.TTL != provider.CacheTTL1h {
			t.Fatalf("SystemCacheControl = %+v, want ephemeral/1h", req.Config.SystemCacheControl)
		}

		if len(req.Messages) != 3 {
			t.Fatalf("Messages len = %d, want 3", len(req.Messages))
		}

		// user text + image_url
		userBlocks := req.Messages[0].Content
		if len(userBlocks) != 2 {
			t.Fatalf("user blocks len = %d, want 2", len(userBlocks))
		}
		ut, ok := userBlocks[0].(*bamboo.TextBlock)
		if !ok || ut.CacheControl == nil || ut.CacheControl.TTL != provider.CacheTTL5m {
			t.Errorf("user text block CacheControl = %+v, want ephemeral/5m", userBlocks[0])
		}
		ui, ok := userBlocks[1].(*bamboo.ImageBlock)
		if !ok || ui.CacheControl == nil || ui.CacheControl.Type != "ephemeral" {
			t.Errorf("user image block CacheControl = %+v, want ephemeral", userBlocks[1])
		}

		// assistant text + tool_use
		asstBlocks := req.Messages[1].Content
		if len(asstBlocks) != 2 {
			t.Fatalf("assistant blocks len = %d, want 2", len(asstBlocks))
		}
		at, ok := asstBlocks[0].(*bamboo.TextBlock)
		if !ok || at.CacheControl == nil || at.CacheControl.Type != "ephemeral" {
			t.Errorf("assistant text block CacheControl = %+v, want ephemeral", asstBlocks[0])
		}

		// tool result
		toolBlocks := req.Messages[2].Content
		if len(toolBlocks) != 1 {
			t.Fatalf("tool blocks len = %d, want 1", len(toolBlocks))
		}
		tr, ok := toolBlocks[0].(*bamboo.ToolResultBlock)
		if !ok || tr.CacheControl == nil || tr.CacheControl.TTL != provider.CacheTTL1h {
			t.Errorf("tool result block CacheControl = %+v, want ephemeral/1h", toolBlocks[0])
		}

		// tools
		if len(req.Config.Tools) != 2 {
			t.Fatalf("tools len = %d, want 2", len(req.Config.Tools))
		}
		if req.Config.Tools[0].CacheControl == nil || req.Config.Tools[0].CacheControl.TTL != provider.CacheTTL5m {
			t.Errorf("tool[0].CacheControl = %+v, want ephemeral/5m", req.Config.Tools[0].CacheControl)
		}
		if req.Config.Tools[1].CacheControl == nil || req.Config.Tools[1].CacheControl.Type != "ephemeral" {
			t.Errorf("tool[1].CacheControl = %+v, want ephemeral", req.Config.Tools[1].CacheControl)
		}
	})

	t.Run("top-level cache_control and string message fallbacks", func(t *testing.T) {
		body := []byte(`{
			"model": "claude-sonnet-4",
			"cache_control": {"type": "ephemeral", "ttl": "1h"},
			"messages": [
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "hi", "cache_control": {"type": "ephemeral"}},
				{"role": "assistant", "cache_control": {"type": "ephemeral"}, "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "ping", "arguments": "{}"}}]},
				{"role": "tool", "tool_call_id": "call_1", "content": "pong", "cache_control": {"type": "ephemeral"}}
			]
		}`)

		req, err := parseRequest(body)
		if err != nil {
			t.Fatalf("parseRequest() error = %v", err)
		}
		if req.Config.SystemCacheControl == nil || req.Config.SystemCacheControl.TTL != provider.CacheTTL1h {
			t.Errorf("SystemCacheControl fallback = %+v, want ephemeral/1h", req.Config.SystemCacheControl)
		}
		ut, ok := req.Messages[0].Content[0].(*bamboo.TextBlock)
		if !ok || ut.CacheControl == nil || ut.CacheControl.Type != "ephemeral" {
			t.Errorf("user string message fallback CacheControl = %+v", req.Messages[0].Content[0])
		}
		atu, ok := req.Messages[1].Content[0].(*bamboo.ToolUseBlock)
		if !ok || atu.CacheControl == nil || atu.CacheControl.Type != "ephemeral" {
			t.Errorf("assistant tool_use fallback CacheControl = %+v", req.Messages[1].Content[0])
		}
		tr, ok := req.Messages[2].Content[0].(*bamboo.ToolResultBlock)
		if !ok || tr.CacheControl == nil || tr.CacheControl.Type != "ephemeral" {
			t.Errorf("tool string message fallback CacheControl = %+v", req.Messages[2].Content[0])
		}
	})

	t.Run("invalid or empty cache_control is ignored safely", func(t *testing.T) {
		body := []byte(`{
			"model": "gpt-4o",
			"cache_control": null,
			"messages": [
				{"role": "user", "content": [{"type": "text", "text": "hello", "cache_control": {}}]}
			]
		}`)
		req, err := parseRequest(body)
		if err != nil {
			t.Fatalf("parseRequest() error = %v", err)
		}
		if req.Config.SystemCacheControl != nil {
			t.Errorf("SystemCacheControl = %+v, want nil", req.Config.SystemCacheControl)
		}
		tb, ok := req.Messages[0].Content[0].(*bamboo.TextBlock)
		if !ok {
			t.Fatalf("unexpected block type %T", req.Messages[0].Content[0])
		}
		if tb.CacheControl != nil {
			t.Errorf("TextBlock.CacheControl = %+v, want nil", tb.CacheControl)
		}
	})
}

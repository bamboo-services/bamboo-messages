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

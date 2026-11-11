package bamboo

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestRequestOptions_CacheControl 验证缓存控制相关 RequestOption 的落库与下传。
//
// RequestConfig 是上层业务直接使用的入口，缓存控制参数必须能在这里配置，
// 并在转换为 provider.ChatConfig 时完整透传。
func TestRequestOptions_CacheControl(t *testing.T) {
	t.Run("WithPromptCacheOptions", func(t *testing.T) {
		opts := &provider.PromptCacheOptions{
			Mode:                   provider.PromptCacheModeExplicit,
			TTL:                    provider.CacheTTL30m,
			Retention:              provider.PromptCacheRetention24h,
			EnableBlockBreakpoints: true,
		}
		cfg := &RequestConfig{}
		WithPromptCacheOptions(opts)(cfg)

		if cfg.PromptCacheOptions != opts {
			t.Fatalf("PromptCacheOptions = %v, want %v", cfg.PromptCacheOptions, opts)
		}
		if !cfg.PromptCacheOptions.BlockBreakpointsEnabled() {
			t.Error("BlockBreakpointsEnabled() = false, want true")
		}
	})

	t.Run("WithCachedContent", func(t *testing.T) {
		cfg := &RequestConfig{}
		WithCachedContent("cachedContents/abc")(cfg)
		if cfg.CachedContent != "cachedContents/abc" {
			t.Errorf("CachedContent = %q, want cachedContents/abc", cfg.CachedContent)
		}
	})

	t.Run("configToProvider propagates cache fields", func(t *testing.T) {
		opts := &provider.PromptCacheOptions{
			Mode:                   provider.PromptCacheModeExplicit,
			TTL:                    provider.CacheTTL30m,
			EnableBlockBreakpoints: true,
		}
		cfg := &RequestConfig{
			PromptCacheOptions: opts,
			CachedContent:      "cachedContents/xyz",
		}
		got := configToProvider(cfg)
		if got == nil {
			t.Fatal("configToProvider() returned nil")
		}
		if got.PromptCacheOptions != opts {
			t.Errorf("PromptCacheOptions = %v, want %v", got.PromptCacheOptions, opts)
		}
		if got.CachedContent != "cachedContents/xyz" {
			t.Errorf("CachedContent = %q, want cachedContents/xyz", got.CachedContent)
		}
	})
}

// TestProviderPromptCacheOptions_BlockBreakpointsEnabled 锁定块级断点的开启条件。
//
// prompt_cache_breakpoint 是 gpt-5.6+ 的新字段，对更早的模型发送会被上游拒绝，
// 因此默认关闭；显式声明 Mode=explicit 或显式设置 EnableBlockBreakpoints 才开启。
func TestProviderPromptCacheOptions_BlockBreakpointsEnabled(t *testing.T) {
	tests := []struct {
		name string
		opts *provider.PromptCacheOptions
		want bool
	}{
		{name: "nil", opts: nil, want: false},
		{name: "empty", opts: &provider.PromptCacheOptions{}, want: false},
		{
			name: "explicit mode implies enabled",
			opts: &provider.PromptCacheOptions{Mode: provider.PromptCacheModeExplicit},
			want: true,
		},
		{
			name: "implicit mode stays disabled",
			opts: &provider.PromptCacheOptions{Mode: provider.PromptCacheModeImplicit},
			want: false,
		},
		{
			name: "explicit opt-in enables in implicit mode",
			opts: &provider.PromptCacheOptions{
				Mode:                   provider.PromptCacheModeImplicit,
				EnableBlockBreakpoints: true,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.BlockBreakpointsEnabled(); got != tt.want {
				t.Errorf("BlockBreakpointsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

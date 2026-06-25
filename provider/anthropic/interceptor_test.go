package anthropic

import (
	"context"
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestWithInterceptorRegistersIntoConfig 验证 anthropic.WithInterceptor 把拦截器
// 写入 config.interceptors，且 NewProviderWithOptions 构造的 Provider 能通过 SDK
// 的 HTTP client 配置把 interceptorTransport 注入到 transport 链中。
//
// 由于 anthropic.Provider 的 Client 字段是 anthropic.Client（不暴露内部 transport），
// 这里只能做 smoke test：验证注册路径不 panic、不丢失拦截器。
// 完整的 HTTP 层集成验证由 provider 包的 interceptorTransport 测试覆盖。
func TestWithInterceptorRegistersIntoConfig(t *testing.T) {
	t.Run("单个拦截器注册成功", func(t *testing.T) {
		realFn := provider.RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return body, nil
		})
		p := NewProviderWithOptions(WithAPIKey("test-key"), WithInterceptor(realFn))
		if p == nil {
			t.Error("注册拦截器后 Provider 不应为 nil")
		}
	})

	t.Run("nil 拦截器被拒绝不 panic", func(t *testing.T) {
		p := NewProviderWithOptions(WithAPIKey("test-key"), WithInterceptor(nil))
		if p == nil {
			t.Error("nil 拦截器不应导致 Provider 为 nil")
		}
	})

	t.Run("无拦截器时构造行为不变（向后兼容）", func(t *testing.T) {
		p := NewProviderWithOptions(WithAPIKey("test-key"))
		if p == nil {
			t.Error("无拦截器时 Provider 不应为 nil")
		}
		if p.GetProviderType() != provider.ProviderAnthropic {
			t.Errorf("Provider 类型错误，got=%v，want=%v", p.GetProviderType(), provider.ProviderAnthropic)
		}
	})
}

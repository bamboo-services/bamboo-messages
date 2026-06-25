package provider

import (
	"context"
	"errors"
	"testing"
)

// TestWithInterceptorAddsToOptions 验证通过 WithInterceptor 函数选项可以
// 把拦截器注册到 Options.Interceptors 切片中。
//
// 这是注册机制的入口契约：业务代码调用 WithInterceptor(fn) 得到一个 Option，
// 应用到 Options 后，Options.Interceptors 必须非空且包含 fn。
func TestWithInterceptorAddsToOptions(t *testing.T) {
	t.Run("单个拦截器注册", func(t *testing.T) {
		called := 0
		interceptor := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			called++
			return body, nil
		})

		opts := ApplyOptions(WithInterceptor(interceptor))
		if len(opts.Interceptors) != 1 {
			t.Fatalf("注册 1 个拦截器后 Interceptors 长度应为 1，got=%d", len(opts.Interceptors))
		}
		// 实际调用一次，验证函数确实被注册且可执行
		_, _ = ApplyInterceptors(context.Background(), []byte("{}"), opts.Interceptors)
		if called != 1 {
			t.Errorf("拦截器未被调用或被多次调用，called=%d", called)
		}
	})

	t.Run("nil 拦截器被拒绝", func(t *testing.T) {
		// 防御：显式 nil 拦截器不应进入切片（避免后续 ApplyInterceptors 踩坑）
		opts := ApplyOptions(WithInterceptor(nil))
		if len(opts.Interceptors) != 0 {
			t.Errorf("nil 拦截器不应注册，got len=%d", len(opts.Interceptors))
		}
	})
}

// TestMultipleInterceptorsChained 验证多个 WithInterceptor 按调用顺序链式注册，
// 且 ApplyOptions 后 ApplyInterceptors 会按相同顺序执行。
//
// 链式语义是核心契约：业务代码可能分别注册日志、参数覆盖、签名等多个拦截器，
// 顺序必须稳定（先进先执行），否则会产生竞态。
func TestMultipleInterceptorsChained(t *testing.T) {
	order := []int{}

	first := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
		order = append(order, 1)
		// 修改 body 让第二个拦截器能看到
		return append(body, []byte("-first")...), nil
	})
	second := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
		order = append(order, 2)
		if string(body) != "{}-first" {
			t.Errorf("第二个拦截器未收到第一个的输出，got=%q", string(body))
		}
		return append(body, []byte("-second")...), nil
	})
	third := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
		order = append(order, 3)
		return body, nil
	})

	opts := ApplyOptions(WithInterceptor(first), WithInterceptor(second), WithInterceptor(third))
	if len(opts.Interceptors) != 3 {
		t.Fatalf("3 个拦截器注册后长度应为 3，got=%d", len(opts.Interceptors))
	}

	result, err := ApplyInterceptors(context.Background(), []byte("{}"), opts.Interceptors)
	if err != nil {
		t.Fatalf("链式应用不应返回 error: %v", err)
	}
	if got, want := string(result), "{}-first-second"; got != want {
		t.Errorf("最终 body 不正确，got=%q，want=%q", got, want)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("拦截器执行顺序错误，got=%v，want=[1 2 3]", order)
	}
}

// TestApplyOptionsEmptyYieldsEmptyInterceptors 验证无 WithInterceptor 时，
// Options.Interceptors 为 nil/空切片，保证向后兼容（零行为变化契约）。
func TestApplyOptionsEmptyYieldsEmptyInterceptors(t *testing.T) {
	opts := ApplyOptions()
	if opts == nil {
		t.Fatal("ApplyOptions 无参数时不应返回 nil Options")
	}
	if len(opts.Interceptors) != 0 {
		t.Errorf("无 WithInterceptor 时 Interceptors 应为空，got len=%d", len(opts.Interceptors))
	}

	// 无拦截器时 ApplyInterceptors 应原样返回（已在 Task 1 测试覆盖，此处重复验证集成）
	original := []byte(`{"k":"v"}`)
	got, err := ApplyInterceptors(context.Background(), original, opts.Interceptors)
	if err != nil {
		t.Fatalf("无拦截器不应返回 error: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("无拦截器应原样返回 body，got=%q", string(got))
	}
}

// TestApplyOptionsChainErrorStops 验证 ApplyOptions 注册的链中，
// 任一拦截器返回 error 时，ApplyInterceptors 立即停止并冒泡 error。
// 这是 Task 1 TestInterceptorReturnsError 的集成版本（走 ApplyOptions 路径）。
func TestApplyOptionsChainErrorStops(t *testing.T) {
	sentinel := errors.New("integration-sentinel")

	failing := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
		return nil, sentinel
	})
	after := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
		t.Error("error 之后不应继续调用后续拦截器")
		return body, nil
	})

	opts := ApplyOptions(WithInterceptor(failing), WithInterceptor(after))
	_, err := ApplyInterceptors(context.Background(), []byte("{}"), opts.Interceptors)
	if !errors.Is(err, sentinel) {
		t.Errorf("集成路径下 error 未正确冒泡，got=%v，want=%v", err, sentinel)
	}
}

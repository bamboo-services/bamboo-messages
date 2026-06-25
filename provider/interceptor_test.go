package provider

import (
	"context"
	"errors"
	"testing"
)

// TestRequestInterceptorSignature 验证 RequestInterceptor 类型的函数签名契约。
//
// RequestInterceptor 必须是一个可由 `func(context.Context, []byte) ([]byte, error)`
// 識别的函数类型，调用方依此契约注入参数覆盖逻辑。
func TestRequestInterceptorSignature(t *testing.T) {
	// 验证：一个标准签名的函数可以赋值给 RequestInterceptor 类型
	var interceptor RequestInterceptor = func(ctx context.Context, body []byte) ([]byte, error) {
		return body, nil
	}
	if interceptor == nil {
		t.Fatal("RequestInterceptor 赋值后不应为 nil")
	}

	// 验证：类型断言能够通过（即签名被识别为 RequestInterceptor）
	var _ RequestInterceptor = interceptor
}

// TestNilInterceptorNoOp 验证 nil 拦截器切片对 body 没有副作用。
//
// 调用 ApplyInterceptors 时若传入空切片或 nil，应原样返回 body、返回 nil error，
// 且不发生 panic。这是保证向后兼容（未注入拦截器时零行为变化）的关键契约。
func TestNilInterceptorNoOp(t *testing.T) {
	original := []byte(`{"temperature":0.5,"max_tokens":100}`)

	t.Run("nil 切片原样返回", func(t *testing.T) {
		got, err := ApplyInterceptors(context.Background(), original, nil)
		if err != nil {
			t.Fatalf("nil 拦截器切片不应返回 error，got: %v", err)
		}
		if string(got) != string(original) {
			t.Errorf("nil 拦截器切片原样返回失败，got=%q，want=%q", string(got), string(original))
		}
	})

	t.Run("空切片原样返回", func(t *testing.T) {
		got, err := ApplyInterceptors(context.Background(), original, []RequestInterceptor{})
		if err != nil {
			t.Fatalf("空拦截器切片不应返回 error，got: %v", err)
		}
		if string(got) != string(original) {
			t.Errorf("空拦截器切片原样返回失败，got=%q，want=%q", string(got), string(original))
		}
	})

	t.Run("nil body 也不 panic", func(t *testing.T) {
		got, err := ApplyInterceptors(context.Background(), nil, nil)
		if err != nil {
			t.Fatalf("nil body 不应返回 error，got: %v", err)
		}
		if got != nil {
			t.Errorf("nil body 应原样返回 nil，got=%q", string(got))
		}
	})
}

// TestInterceptorReturnsError 验证拦截器返回 error 时，
// ApplyInterceptors 应原样向上冒泡该 error 并立即停止后续拦截器链。
//
// 此契约保证：参数覆盖失败时，Provider 不会把错误数据发往上游，
// 而是把错误冒泡到调用方（new-api 侧再转换为 NewAPIError）。
func TestInterceptorReturnsError(t *testing.T) {
	original := []byte(`{"model":"gpt-4"}`)

	sentinel := errors.New("param override configuration invalid")

	t.Run("单个拦截器 error 冒泡", func(t *testing.T) {
		failing := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return nil, sentinel
		})
		got, err := ApplyInterceptors(context.Background(), original, []RequestInterceptor{failing})
		if !errors.Is(err, sentinel) {
			t.Errorf("error 未正确冒泡，got=%v，want=%v", err, sentinel)
		}
		if got != nil {
			t.Errorf("error 发生时应返回 nil body，got=%q", string(got))
		}
	})

	t.Run("链式 - 第二个拦截器不执行", func(t *testing.T) {
		secondCalled := false
		first := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return nil, sentinel
		})
		second := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			secondCalled = true
			return body, nil
		})
		_, err := ApplyInterceptors(context.Background(), original, []RequestInterceptor{first, second})
		if !errors.Is(err, sentinel) {
			t.Errorf("error 未正确冒泡，got=%v", err)
		}
		if secondCalled {
			t.Error("第一个拦截器失败后，第二个拦截器不应被调用")
		}
	})
}

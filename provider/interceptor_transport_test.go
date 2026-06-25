package provider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestInterceptorTransportAppliesInterceptors 验证 interceptorTransport 在转发请求前，
// 按 ApplyInterceptors 的链式语义对 request body 做改写。
//
// 这是 HTTP 层注入的核心契约：上层（new-api）注册 RequestInterceptor 后，
// 任何经过该 transport 的 HTTP 请求 body 都必须先走拦截器链再发往上游。
func TestInterceptorTransportAppliesInterceptors(t *testing.T) {
	t.Run("单拦截器修改 body 后转发", func(t *testing.T) {
		// 准备一个 mock upstream，记录收到的 body
		var capturedBody []byte
		upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
		mockServer := newMockServer(upstream)
		defer mockServer.Close()

		// 拦截器：把 temperature 字段从 0.5 改为 0.9
		modifier := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return bytes.Replace(body, []byte(`"temperature":0.5`), []byte(`"temperature":0.9`), 1), nil
		})

		// 构造 interceptorTransport，base 指向 mockServer
		transport := &interceptorTransport{
			base:          mockServer.defaultTransport(),
			interceptors:  []RequestInterceptor{modifier},
			transportName: "test",
		}

		// 构造一个 HTTP 请求
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, mockServer.URL, strings.NewReader(`{"model":"x","temperature":0.5}`))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Transport: transport}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		defer resp.Body.Close()

		// 断言上游收到了被拦截器修改后的 body
		if got := string(capturedBody); got != `{"model":"x","temperature":0.9}` {
			t.Errorf("拦截器修改未生效，上游收到=%q，want=%q", got, `{"model":"x","temperature":0.9}`)
		}
	})

	t.Run("多拦截器链式执行", func(t *testing.T) {
		var capturedBody []byte
		upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		})
		mockServer := newMockServer(upstream)
		defer mockServer.Close()

		first := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return append(body, []byte(`-first`)...), nil
		})
		second := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return append(body, []byte(`-second`)...), nil
		})

		transport := &interceptorTransport{
			base:         mockServer.defaultTransport(),
			interceptors: []RequestInterceptor{first, second},
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, mockServer.URL, strings.NewReader(`{}`))
		client := &http.Client{Transport: transport}
		_, err := client.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}

		if got := string(capturedBody); got != `{}-first-second` {
			t.Errorf("链式执行未生效，上游收到=%q，want=%q", got, `{}-first-second`)
		}
	})

	t.Run("Content-Length 被自动重算", func(t *testing.T) {
		var capturedContentLength string
		upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedContentLength = r.Header.Get("Content-Length")
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		})
		mockServer := newMockServer(upstream)
		defer mockServer.Close()

		// 拦截器把 body 从 3 字节（{}）扩充到 3+36=39 字节
		expander := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return append(body, []byte(`-expanded-with-many-extra-characters`)...), nil
		})

		transport := &interceptorTransport{
			base:         mockServer.defaultTransport(),
			interceptors: []RequestInterceptor{expander},
		}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, mockServer.URL, strings.NewReader(`{}}`))
		// 设置原始 Content-Length（应该是 3，但拦截器会改 body）
		req.ContentLength = 3

		client := &http.Client{Transport: transport}
		_, err := client.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}

		// 断言 Content-Length 被重算为新 body 长度（3 + 36 = 39）
		if capturedContentLength != "39" {
			t.Errorf("Content-Length 未重算，上游收到=%q，want=%q", capturedContentLength, "39")
		}
	})
}

// TestInterceptorTransportErrorPropagates 验证拦截器返回 error 时，
// interceptorTransport 不向上游发送请求，并把 error 返回给调用方。
func TestInterceptorTransportErrorPropagates(t *testing.T) {
	upstreamCalled := false
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})
	mockServer := newMockServer(upstream)
	defer mockServer.Close()

	sentinel := errors.New("param override config invalid")
	failing := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
		return nil, sentinel
	})

	transport := &interceptorTransport{
		base:         mockServer.defaultTransport(),
		interceptors: []RequestInterceptor{failing},
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, mockServer.URL, strings.NewReader(`{}`))
	client := &http.Client{Transport: transport}
	_, err := client.Do(req)

	if err == nil {
		t.Fatal("拦截器返回 error 时，client.Do 应返回 error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error 未正确冒泡，got=%v，want包含=%v", err, sentinel)
	}
	if upstreamCalled {
		t.Error("拦截器失败后，上游仍被调用")
	}
}

// TestInterceptorTransportNoBodyRequests 验证无 body 的请求（如 GET）不受拦截器影响。
//
// 这是边界场景：有些管理类 API 调用可能没有 body，拦截器不应被触发也不应 panic。
func TestInterceptorTransportNoBodyRequests(t *testing.T) {
	upstreamCalled := false
	var capturedBody []byte
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		capturedBody, _ = io.ReadAll(r.Body)
		if len(capturedBody) == 0 {
			capturedBody = nil
		}
		w.WriteHeader(http.StatusOK)
	})
	mockServer := newMockServer(upstream)
	defer mockServer.Close()

	interceptorCalled := false
	interceptor := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
		interceptorCalled = true
		return body, nil
	})

	transport := &interceptorTransport{
		base:         mockServer.defaultTransport(),
		interceptors: []RequestInterceptor{interceptor},
	}

	// GET 请求，无 body
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, mockServer.URL, nil)
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if !upstreamCalled {
		t.Error("上游未被调用")
	}
	if interceptorCalled {
		t.Error("无 body 请求不应触发拦截器")
	}
	if capturedBody != nil {
		t.Errorf("GET 请求不应有 body，got=%q", string(capturedBody))
	}
}

// TestNewInterceptorHTTPClient 验证 NewInterceptorHTTPClient 工厂函数。
//
// 业务代码（4 个 Provider）通过此函数把拦截器注入到 HTTP client 中，
// 而不是直接操作 interceptorTransport 内部字段。
func TestNewInterceptorHTTPClient(t *testing.T) {
	t.Run("无拦截器时返回 nil（让 Provider 用 SDK 默认 client）", func(t *testing.T) {
		got := NewInterceptorHTTPClient(nil, nil)
		if got != nil {
			t.Errorf("无拦截器应返回 nil，got=%v", got)
		}
	})

	t.Run("空切片也返回 nil", func(t *testing.T) {
		got := NewInterceptorHTTPClient(nil, []RequestInterceptor{})
		if got != nil {
			t.Errorf("空拦截器切片应返回 nil，got=%v", got)
		}
	})

	t.Run("有拦截器时返回非 nil client", func(t *testing.T) {
		fn := RequestInterceptor(func(ctx context.Context, body []byte) ([]byte, error) {
			return body, nil
		})
		got := NewInterceptorHTTPClient(http.DefaultTransport, []RequestInterceptor{fn})
		if got == nil {
			t.Error("有拦截器时应返回非 nil http.Client")
		}
		if got.Transport == nil {
			t.Error("返回的 client 应有 Transport")
		}
	})
}

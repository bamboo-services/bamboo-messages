package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================
// HTTPClient 单元测试
// ============================================
//
// 测试覆盖：
//  1. 认证头三种模式（Bearer / x-api-key / x-goog-api-key）
//  2. User-Agent 统一注入
//  3. 请求拦截器注入（修改 body）
//  4. 自定义请求头透传
//  5. URL 拼接（baseURL 尾斜杠处理）
//  6. DoWithDebug debug 日志输出
//  7. 空 apiKey 时不注入认证头

// captureRequest 辅助函数：启动一个 mock server 并捕获单个请求的关键信息。
//
// 返回 server 实例和捕获到的 request 通道。调用方负责 Close。
func captureRequest(t *testing.T, status int) (*httptest.Server, chan *capturedReq) {
	t.Helper()
	ch := make(chan *capturedReq, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		headers := make(map[string]string)
		for k, v := range r.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}

		ch <- &capturedReq{
			method:  r.Method,
			path:    r.URL.Path,
			headers: headers,
			body:    body,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	return srv, ch
}

type capturedReq struct {
	method  string
	path    string
	headers map[string]string
	body    []byte
}

// --------------------------------------------
// 认证头测试
// --------------------------------------------

func TestHTTPClient_AuthBearer(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "sk-test-key", "Authorization", "Bearer ", nil, nil)
	_, err := hc.Do(context.Background(), "POST", "/v1/chat/completions", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	auth := req.headers["Authorization"]
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Errorf("expected 'Bearer ' prefix, got: %q", auth)
	}
	if !strings.Contains(auth, "sk-test-key") {
		t.Errorf("expected api key in auth header, got: %q", auth)
	}
}

func TestHTTPClient_AuthXAPIKey(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "sk-ant-key", "x-api-key", "", nil, nil)
	_, err := hc.Do(context.Background(), "POST", "/v1/messages", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	got := req.headers["X-Api-Key"]
	if got != "sk-ant-key" {
		t.Errorf("expected 'sk-ant-key' in x-api-key header, got: %q", got)
	}
	// 不应有 Authorization 头
	if _, ok := req.headers["Authorization"]; ok {
		t.Error("should not set Authorization header when authHeader is x-api-key")
	}
}

func TestHTTPClient_AuthXGoogAPIKey(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "AIza-key", "x-goog-api-key", "", nil, nil)
	_, err := hc.Do(context.Background(), "POST", "/v1/models/gemini-pro:generateContent", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	got := req.headers["X-Goog-Api-Key"]
	if got != "AIza-key" {
		t.Errorf("expected 'AIza-key' in x-goog-api-key header, got: %q", got)
	}
}

func TestHTTPClient_NoAuthWhenAPIKeyEmpty(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "", "Authorization", "Bearer ", nil, nil)
	_, err := hc.Do(context.Background(), "GET", "/health", nil)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	if _, ok := req.headers["Authorization"]; ok {
		t.Error("should not set Authorization header when apiKey is empty")
	}
}

// --------------------------------------------
// User-Agent 测试
// --------------------------------------------

func TestHTTPClient_UserAgent(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", nil, nil)
	_, err := hc.Do(context.Background(), "POST", "/test", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	ua := req.headers["User-Agent"]
	if !strings.HasPrefix(ua, "BM-SDK/") {
		t.Errorf("expected User-Agent to start with 'BM-SDK/', got: %q", ua)
	}
}

// --------------------------------------------
// 拦截器注入测试
// --------------------------------------------

func TestHTTPClient_InterceptorModifiesBody(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	// 拦截器：向 JSON body 注入一个额外字段
	interceptor := func(ctx context.Context, body []byte) ([]byte, error) {
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			return body, nil
		}
		m["injected"] = true
		return json.Marshal(m)
	}

	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", nil, []RequestInterceptor{interceptor})
	_, err := hc.Do(context.Background(), "POST", "/v1/chat", []byte(`{"model":"gpt-4"}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch

	var result map[string]any
	if err := json.Unmarshal(req.body, &result); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	if result["model"] != "gpt-4" {
		t.Errorf("expected original field 'model'='gpt-4', got: %v", result["model"])
	}
	if result["injected"] != true {
		t.Errorf("expected injected field 'injected'=true, got: %v", result["injected"])
	}
}

func TestHTTPClient_MultipleInterceptorsChain(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	// 两个拦截器按注册顺序执行
	interceptor1 := func(ctx context.Context, body []byte) ([]byte, error) {
		var m map[string]any
		json.Unmarshal(body, &m)
		m["first"] = true
		return json.Marshal(m)
	}
	interceptor2 := func(ctx context.Context, body []byte) ([]byte, error) {
		var m map[string]any
		json.Unmarshal(body, &m)
		m["second"] = true
		return json.Marshal(m)
	}

	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", nil,
		[]RequestInterceptor{interceptor1, interceptor2})
	_, err := hc.Do(context.Background(), "POST", "/test", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	var result map[string]any
	json.Unmarshal(req.body, &result)

	if result["first"] != true || result["second"] != true {
		t.Errorf("expected both interceptors to run, got: %v", result)
	}
}

// --------------------------------------------
// 自定义请求头透传测试
// --------------------------------------------

func TestHTTPClient_CustomHeaders(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	headers := map[string]string{
		"X-Trace-Id":       "abc-123",
		"anthropic-version": "2023-06-01",
		"X-Custom":         "hello",
	}

	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", headers, nil)
	_, err := hc.Do(context.Background(), "POST", "/test", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch

	for k, v := range headers {
		// httptest 标准化 header 名为 Title-Case（如 "X-Trace-Id"）
		if got := req.headers[k]; got != v {
			// 回退检查（header 大小写不敏感，Go 标准化存储）
			found := false
			for hk, hv := range req.headers {
				if strings.EqualFold(hk, k) && hv == v {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected header %s=%q, not found in %v", k, v, req.headers)
			}
		}
	}
}

// --------------------------------------------
// URL 拼接测试
// --------------------------------------------

func TestHTTPClient_URLJoining(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", nil, nil)
	_, err := hc.Do(context.Background(), "POST", "/v1/messages", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	if req.path != "/v1/messages" {
		t.Errorf("expected path '/v1/messages', got: %q", req.path)
	}
}

func TestHTTPClient_URLJoiningTrailingSlash(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	// baseURL 带尾斜杠 + path 带前导斜杠
	hc := NewHTTPClient(srv.URL+"/", "key", "Authorization", "Bearer ", nil, nil)
	_, err := hc.Do(context.Background(), "POST", "/test/endpoint", []byte(`{}`))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	if req.path != "/test/endpoint" {
		t.Errorf("expected path '/test/endpoint' (trailing/leading slash trimmed), got: %q", req.path)
	}
}

// --------------------------------------------
// Content-Type / Content-Length 测试
// --------------------------------------------

func TestHTTPClient_ContentTypeForBody(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", nil, nil)
	body := []byte(`{"hello":"world"}`)
	_, err := hc.Do(context.Background(), "POST", "/test", body)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	ct := req.headers["Content-Type"]
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got: %q", ct)
	}
}

func TestHTTPClient_NoContentTypeForNilBody(t *testing.T) {
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", nil, nil)
	_, err := hc.Do(context.Background(), "GET", "/health", nil)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	req := <-ch
	if _, ok := req.headers["Content-Type"]; ok {
		t.Error("should not set Content-Type for nil body")
	}
}

// --------------------------------------------
// GetBaseURL 测试
// --------------------------------------------

func TestHTTPClient_GetBaseURL(t *testing.T) {
	hc := NewHTTPClient("https://api.example.com", "key", "Authorization", "Bearer ", nil, nil)
	if got := hc.GetBaseURL(); got != "https://api.example.com" {
		t.Errorf("expected baseURL 'https://api.example.com', got: %q", got)
	}
}

// --------------------------------------------
// DoWithDebug 测试
// --------------------------------------------

func TestHTTPClient_DoWithDebug(t *testing.T) {
	// 捕获 log 输出需要 redirect log output，这里仅验证 debug=false 时不崩溃
	srv, ch := captureRequest(t, 200)
	defer srv.Close()

	// debug 关闭状态下调用 DoWithDebug 应正常工作
	SetDebug(false)
	hc := NewHTTPClient(srv.URL, "key", "Authorization", "Bearer ", nil, nil)
	_, err := hc.DoWithDebug(context.Background(), "POST", "/test", []byte(`{}`), "test-provider", srv.URL+"/test")
	if err != nil {
		t.Fatalf("DoWithDebug failed: %v", err)
	}

	req := <-ch
	if req.method != "POST" {
		t.Errorf("expected method 'POST', got: %q", req.method)
	}
}

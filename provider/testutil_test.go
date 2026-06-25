package provider

import (
	"net/http"
	"net/http/httptest"
)

// mockServer 测试辅助：包装 httptest.Server，提供默认 Transport 指向自己。
type mockServer struct {
	*httptest.Server
}

// newMockServer 创建一个指向 handler 的 mock HTTP server。
//
// 使用示例：
//
//	ms := newMockServer(handler)
//	defer ms.Close()
//	transport := ms.defaultTransport() // 返回指向 ms.URL 的 http.Transport
func newMockServer(handler http.Handler) *mockServer {
	srv := httptest.NewServer(handler)
	return &mockServer{Server: srv}
}

// defaultTransport 返回指向本 mockServer 的标准 http.Transport。
//
// 用于 interceptorTransport 的 base 字段，使拦截器链最终把请求发到本 mockServer。
func (m *mockServer) defaultTransport() http.RoundTripper {
	return &http.Transport{
		// httptest.Server 已支持普通 HTTP，无需 TLS 配置
	}
}

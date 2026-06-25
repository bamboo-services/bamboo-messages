package provider

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// ============================================
// HTTP 层拦截器 Transport
// ============================================

// interceptorTransport 在 HTTP 层注入 RequestInterceptor 链的 http.RoundTripper 实现。
//
// 工作原理：
//   - RoundTrip 时读取原始请求 body
//   - 经过 ApplyInterceptors 链式应用所有拦截器
//   - 用新 body 重建请求（重算 Content-Length）
//   - 转发给 base transport（通常是 SDK 默认 transport 或自定义 client 的 transport）
//
// 适用场景：当上游 SDK（如 anthropic-sdk-go / openai-go / google genai）
// 在内部完成 marshal + HTTP 调用、外部无法插入中间步骤时，
// 通过替换 HTTP client 的 Transport 实现 []byte 级别的请求改写。
//
// 设计约束：
//   - 仅对带 body 的请求触发拦截器（GET/HEAD 等无 body 请求直接透传）
//   - 拦截器返回 error 时立即停止，请求不会到达上游
//   - body 为 nil 或空字节时仍透传（让拦截器自行决定是否处理）
type interceptorTransport struct {
	base http.RoundTripper

	// interceptors 注册的拦截器列表，按顺序执行。
	interceptors []RequestInterceptor

	// transportName 可选的 transport 标识，用于错误信息中定位来源。
	// 例如 "anthropic" / "openai-completions" 等。
	transportName string
}

// RoundTrip 实现 http.RoundTripper 接口。
//
// 流程：
//  1. 若请求无 body 或无拦截器，直接转发（零开销 fast path）
//  2. 读取原始 body 到 []byte
//  3. 调用 ApplyInterceptors 链式应用
//  4. 用新 body 重建 Request（保持 Method/URL/Header 不变）
//  5. 自动重算 Content-Length（避免长度不匹配导致上游拒绝）
//  6. 转发给 base transport
func (t *interceptorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// fast path: 无拦截器或无 body，直接透传
	if len(t.interceptors) == 0 || req == nil || req.Body == nil {
		return t.base.RoundTrip(req)
	}

	// 读取原始 body
	origBody, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("interceptorTransport(%s): read original body failed: %w", t.transportName, err)
	}
	_ = req.Body.Close()

	// 应用拦截器链
	newBody, err := ApplyInterceptors(req.Context(), origBody, t.interceptors)
	if err != nil {
		return nil, fmt.Errorf("interceptorTransport(%s): apply interceptors failed: %w", t.transportName, err)
	}

	// 重建请求（http.Request 的 Body 字段不可直接复用，必须 NewRequest 一份或用 clone）
	// 这里用最小成本：替换 Body + GetBody + ContentLength
	req.Body = io.NopCloser(bytes.NewReader(newBody))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(newBody)), nil
	}
	req.ContentLength = int64(len(newBody))

	// Content-Length header 会被 http.Transport 根据 ContentLength 字段自动设置，
	// 这里清除显式 header 让 Transport 重算（防止客户端显式写了错误的旧值）。
	req.Header.Del("Content-Length")

	return t.base.RoundTrip(req)
}

// NewInterceptorHTTPClient 构造一个注入拦截器的 *http.Client。
//
// 参数：
//   - baseTransport: 基础 RoundTripper（通常为 http.DefaultTransport 或 SDK 默认 transport）；
//     若为 nil 则使用 http.DefaultTransport
//   - interceptors: 拦截器列表，nil 或空切片时返回 nil（让 Provider 用 SDK 默认 client）
//
// 返回值：
//   - nil 表示无需注入（Provider 应保留 SDK 默认 client，避免无谓的 transport 包装）
//   - 非 nil 表示一个持有 interceptorTransport 的 http.Client
//
// 使用示例（在各 Provider 的 NewProviderWithOptions 中）：
//
//	if httpCli := provider.NewInterceptorHTTPClient(nil, opts.Interceptors); httpCli != nil {
//	    sdkOpts = append(sdkOpts, option.WithHTTPClient(httpCli))
//	}
func NewInterceptorHTTPClient(baseTransport http.RoundTripper, interceptors []RequestInterceptor) *http.Client {
	if len(interceptors) == 0 {
		return nil
	}
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	return &http.Client{
		Transport: &interceptorTransport{
			base:         baseTransport,
			interceptors: interceptors,
		},
	}
}

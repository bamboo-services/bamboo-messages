package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================
// 统一 HTTP 客户端
// ============================================

// HTTPClient 是各协议适配器共享的统一 HTTP 客户端。
//
// 在 de-sdk 重构中替代 BaseProvider[T] 泛型，作为所有适配器的底层通信基座。
// 封装了认证头注入、统一 User-Agent、自定义请求头、请求拦截器链以及
// debug 日志输出，使各适配器无需依赖具体 SDK 即可发起 HTTP 请求。
//
// 设计要点：
//   - 认证头差异化：authHeader + authPrefix 组合支持 Bearer / x-api-key / x-goog-api-key
//   - 拦截器复用：非空拦截器时通过 NewInterceptorHTTPClient 包装 *http.Client
//   - Debug 集成：DoWithDebug 在发送前调用 DebugRequest 输出请求详情
//   - URL 拼接：简单的 trim trailing "/" + "/" + trimLeadingSlash 模式，不做 path 解析
type HTTPClient struct {
	client       *http.Client
	baseURL      string
	apiKey       string
	authHeader   string // "Authorization" 或 "x-api-key" 或 "x-goog-api-key"
	authPrefix   string // "Bearer " 或 ""（x-api-key / x-goog-api-key 无前缀）
	headers      map[string]string
	interceptors []RequestInterceptor
}

// NewHTTPClient 创建统一 HTTP 客户端实例。
//
// 参数：
//   - baseURL: 基础 URL（如 "https://api.anthropic.com"）
//   - apiKey: API 密钥（留空则不设置认证头）
//   - authHeader: 认证头字段名（"Authorization" / "x-api-key" / "x-goog-api-key"）
//   - authPrefix: 认证头值前缀（"Bearer " 或 ""）
//   - headers: 额外的自定义请求头
//   - interceptors: 请求拦截器链；非空时通过 NewInterceptorHTTPClient 包装 *http.Client
//
// 当 interceptors 为空时，使用默认的 &http.Client{}（标准 Transport），
// 避免无谓的 Transport 包装开销（与 NewInterceptorHTTPClient 零包装契约一致）。
func NewHTTPClient(
	baseURL, apiKey, authHeader, authPrefix string,
	headers map[string]string,
	interceptors []RequestInterceptor,
) *HTTPClient {
	hc := &HTTPClient{
		baseURL:      baseURL,
		apiKey:       apiKey,
		authHeader:   authHeader,
		authPrefix:   authPrefix,
		headers:      headers,
		interceptors: interceptors,
	}

	// 有拦截器时用 interceptorTransport 包装，否则用默认 client
	if wrapped := NewInterceptorHTTPClient(nil, interceptors); wrapped != nil {
		hc.client = wrapped
	} else {
		hc.client = &http.Client{
			Transport: &http.Transport{
				TLSHandshakeTimeout: 10 * time.Second,
				IdleConnTimeout:     90 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		}
	}

	return hc
}

// Do 发送 HTTP 请求。
//
// 流程：
//  1. 拼接完整 URL（baseURL + path）
//  2. 通过 http.NewRequestWithContext 构造请求
//  3. 注入认证头（apiKey 非空时）
//  4. 注入统一 User-Agent（GetUserAgent()）
//  5. 注入所有自定义请求头
//  6. body 非空时设置 Content-Type: application/json 和 Content-Length
//  7. 调用 c.client.Do 发送请求
//
// 调用方负责关闭返回的 *http.Response.Body。
func (c *HTTPClient) Do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := c.buildURL(path)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http_client: create request failed: %w", err)
	}

	// 注入请求头
	c.applyHeaders(req, body)

	return c.client.Do(req)
}

// DoWithDebug 发送 HTTP 请求，并在发送前输出 debug 日志。
//
// 与 Do 行为一致，额外在请求发送前调用 DebugRequest 输出：
//   - providerType: 协议类型标识（如 "anthropic" / "openai-completions"）
//   - endpoint: 目标端点完整 URL
//   - headers: 请求头键值对（含认证头、User-Agent、自定义头）
//   - body: json.RawMessage 格式的请求体
//
// debug 日志受全局 DebugEnabled 开关控制，关闭时零开销。
func (c *HTTPClient) DoWithDebug(
	ctx context.Context, method, path string,
	body []byte,
	providerType, endpoint string,
) (*http.Response, error) {
	url := c.buildURL(path)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http_client: create request failed: %w", err)
	}

	// 注入请求头
	c.applyHeaders(req, body)

	// 构建 debug 用的 headers 快照
	debugHeaders := c.buildDebugHeaders()

	// 输出 debug 日志（endpoint 参数复用传入的 endpoint 或 url）
	actualEndpoint := endpoint
	if actualEndpoint == "" {
		actualEndpoint = url
	}
	DebugRequest(providerType, actualEndpoint, debugHeaders, json.RawMessage(body))

	return c.client.Do(req)
}

// GetBaseURL 返回客户端配置的基础 URL。
func (c *HTTPClient) GetBaseURL() string {
	return c.baseURL
}

// ============================================
// 内部辅助方法
// ============================================

// buildURL 拼接完整请求 URL。
//
// 规则：trim trailing "/" from baseURL + "/" + trimLeadingSlash(path)。
// 例如：
//   - baseURL="https://api.anthropic.com", path="/v1/messages"
//     → "https://api.anthropic.com/v1/messages"
//   - baseURL="https://api.openai.com/v1/", path="chat/completions"
//     → "https://api.openai.com/v1/chat/completions"
func (c *HTTPClient) buildURL(path string) string {
	base := strings.TrimRight(c.baseURL, "/")
	p := strings.TrimLeft(path, "/")
	return base + "/" + p
}

// applyHeaders 向 http.Request 注入所有请求头。
//
// 注入顺序：认证头 → User-Agent → 自定义头 → Content-Type（body 非空时）。
// http.Header.Set 会覆盖同名头，后设置的头覆盖先设置的。
func (c *HTTPClient) applyHeaders(req *http.Request, body []byte) {
	// 认证头
	if c.apiKey != "" && c.authHeader != "" {
		req.Header.Set(c.authHeader, c.authPrefix+c.apiKey)
	}

	// 统一 User-Agent
	req.Header.Set("User-Agent", GetUserAgent())

	// 自定义请求头
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// body 非空时设置 Content-Type
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(body))
	}
}

// buildDebugHeaders 构建 debug 日志用的请求头键值对快照。
//
// 包含认证头、User-Agent 和所有自定义头，供 DebugRequest 输出。
// 注意：DebugRequest 内部会对敏感头做脱敏处理。
func (c *HTTPClient) buildDebugHeaders() map[string]string {
	hdrs := make(map[string]string, len(c.headers)+2)

	if c.apiKey != "" && c.authHeader != "" {
		hdrs[c.authHeader] = c.authPrefix + c.apiKey
	}
	hdrs["User-Agent"] = GetUserAgent()
	for k, v := range c.headers {
		hdrs[k] = v
	}

	return hdrs
}

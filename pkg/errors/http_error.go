// Package errors 提供通用的错误类型。
package errors

import (
	stderrors "errors"
	"fmt"
)

// HTTPError 携带 HTTP 状态码的结构化错误类型。
//
// 在 Provider 适配器中，当上游返回 HTTP >= 400 时，使用此类型包装错误，
// 使状态码作为结构化字段贯穿错误链路（xError.Error → BambooError → Codec SerializeError），
// 而非仅存在于消息字符串中。
//
// 设计要点：
//   - StatusCode 保留原始 HTTP 状态码（如 429/401/400/500），供下游错误类型映射
//   - Message 为人类可读的错误描述（包含上游错误详情）
//   - Cause 为可选的原始错误（保留错误链追踪能力）
//   - 实现 error 和 Unwrap 接口，支持 errors.Is / errors.As 链式解包
type HTTPError struct {
	// StatusCode 上游 HTTP 状态码（如 429、401、500）。
	StatusCode int
	// Message 错误描述信息。
	Message string
	// Cause 原始错误原因（可选）。
	Cause error
}

// Error 实现 error 接口。
func (e *HTTPError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap 支持 errors.Is / errors.As 链式解包。
func (e *HTTPError) Unwrap() error { return e.Cause }

// NewHTTPError 创建携带 HTTP 状态码的错误实例。
//
// 参数:
//   - statusCode - HTTP 状态码（如 429、401、500）
//   - message - 错误描述信息
//   - cause - 可选的原始错误原因
func NewHTTPError(statusCode int, message string, cause ...error) *HTTPError {
	var c error
	if len(cause) > 0 && cause[0] != nil {
		c = cause[0]
	}
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
		Cause:      c,
	}
}

// NewHTTPErrorFromStdError 从标准 error 创建 HTTPError（状态码默认 500）。
//
// 用于将已有的非结构化错误包装为 HTTPError，状态码默认 500 INTERNAL。
func NewHTTPErrorFromStdError(err error) *HTTPError {
	msg := "internal error"
	if err != nil {
		msg = err.Error()
	}
	return &HTTPError{
		StatusCode: 500,
		Message:    msg,
		Cause:      err,
	}
}

// 确保 HTTPError 实现 error 接口。
var _ error = (*HTTPError)(nil)

// AsHTTPError 从错误链中提取 HTTPError（如果存在）。
//
// 使用 errors.As 遍历 Unwrap 链，找到第一个 *HTTPError。
// 返回 nil 表示错误链中不包含 HTTPError。
func AsHTTPError(err error) *HTTPError {
	var httpErr *HTTPError
	if stderrors.As(err, &httpErr) {
		return httpErr
	}
	return nil
}

package codec

import "fmt"

// ErrorType 错误类型标识，用于区分不同类别的错误。
type ErrorType string

const (
	// ErrInvalidRequest 请求格式错误或参数不合法。
	ErrInvalidRequest ErrorType = "invalid_request"

	// ErrProviderError AI 服务商返回的错误。
	ErrProviderError ErrorType = "provider_error"

	// ErrAuthError 认证失败（API Key 无效或缺失）。
	ErrAuthError ErrorType = "authentication_error"

	// ErrRateLimit 请求频率超过限制。
	ErrRateLimit ErrorType = "rate_limit_exceeded"

	// ErrInternal Codec 层内部错误。
	ErrInternal ErrorType = "internal_error"
)

// CodecError Codec 层的统一错误类型。
//
// 包含错误分类、描述信息和可选的原始错误原因。
type CodecError struct {
	Type    ErrorType // 错误分类
	Message string    // 错误描述
	Cause   error     // 原始错误原因（可选）
}

// Error 实现 error 接口。
func (e *CodecError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap 支持 errors.Is / errors.As 错误链追踪。
func (e *CodecError) Unwrap() error { return e.Cause }

// NewError 创建一个不带原因的 CodecError。
func NewError(errType ErrorType, message string) *CodecError {
	return &CodecError{Type: errType, Message: message}
}

// NewErrorWithCause 创建一个带原始原因的 CodecError。
func NewErrorWithCause(errType ErrorType, message string, cause error) *CodecError {
	return &CodecError{Type: errType, Message: message, Cause: cause}
}

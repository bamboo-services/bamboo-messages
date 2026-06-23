// Package errors 提供通用的错误类型。
//
// 该包包含以下错误类型：
//   - Error: 内部最小错误类型，替代原 bamboo-base-go/common/error.Error
//   - BambooError: Bamboo SDK 统一错误类型
//
// 这些错误类型可以被所有适配器和上层业务使用，确保错误处理的一致性。
package errors

import (
	"context"
	stderrors "errors"
	"fmt"
)

// Error 是 bamboo-messages 内部使用的最小错误类型，替代原 bamboo-base-go/common/error.Error。
//
// 设计原则：bamboo 转换链仅读取错误的消息文本（见 bamboo/convert.go 的 handleError），
// 从不访问 ErrorCode/Output/Data 等字段，因此这里只需保留 err + Message。
// 保留 *Error 指针语义，确保 convert.go 的 handleError(err *Error) 与
// internal/provider/stream.go 的 StreamEvent.Err *Error 字段零行为变化。
type Error struct {
	err     error
	Message string
}

// Error 实现 error 接口，返回底层 cause 的消息。
func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Unwrap 支持 errors.Is / errors.As 链式解包。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// NewError 兼容原 xError.NewError 签名，用于平滑替换。
//
// 参数对齐原签名 NewError(ctx, err *ErrorCode, msg ErrMessage, throw bool, getErr ...error)，
// 但 ctx / ErrorCode / throw 在 bamboo 的全部调用点都不被实际使用（throw 恒为 false），
// 故这里以 _ 占位，仅保留 msg 与 cause 的语义。
func NewError(_ context.Context, _ any, msg string, _ bool, cause ...error) *Error {
	e := stderrors.New(msg)
	if len(cause) > 0 && cause[0] != nil {
		e = cause[0]
	}
	return &Error{err: e, Message: msg}
}

// BambooError Bamboo SDK 统一错误类型。
//
// 封装底层 AI 服务商返回的错误信息，保留原始错误类型和消息，
// 同时附加协议类型标识以便上层区分错误来源。
type BambooError struct {
	// Type 错误类型标识
	Type string `json:"type"`

	// Message 错误描述信息
	Message string `json:"message"`

	// Code 错误代码（可选，由服务商定义）
	Code string `json:"code,omitempty"`

	// ProviderType 底层协议类型（可选，标识错误来源的协议适配器）
	ProviderType string `json:"provider_type,omitempty"`
}

// 错误类型常量，对应 AI 服务商的标准错误分类。
const (
	// ErrorTypeInvalidRequest 请求参数错误
	ErrorTypeInvalidRequest = "invalid_request_error"

	// ErrorTypeAuthentication 认证错误（API Key 无效或过期）
	ErrorTypeAuthentication = "authentication_error"

	// ErrorTypeRateLimit 请求频率超限
	ErrorTypeRateLimit = "rate_limit_error"

	// ErrorTypeAPI 服务端 API 错误
	ErrorTypeAPI = "api_error"

	// ErrorTypeProvider 底层协议适配器错误
	ErrorTypeProvider = "provider_error"
)

// Error 实现 error 接口。
//
// 根据是否包含 Code 字段返回不同格式的错误信息：
//   - 有 Code: "type: message (code: xxx)"
//   - 无 Code: "type: message"
func (e *BambooError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s (code: %s)", e.Type, e.Message, e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// NewBambooError 创建 BambooError 实例。
//
// 参数:
//   - errorType - 错误类型标识
//   - message - 错误描述信息
func NewBambooError(errorType, message string) *BambooError {
	return &BambooError{
		Type:    errorType,
		Message: message,
	}
}

// NewBambooErrorWithCode 创建带错误代码的 BambooError 实例。
//
// 参数:
//   - errorType - 错误类型标识
//   - message - 错误描述信息
//   - code - 错误代码（可选，由服务商定义）
func NewBambooErrorWithCode(errorType, message, code string) *BambooError {
	return &BambooError{
		Type:    errorType,
		Message: message,
		Code:    code,
	}
}

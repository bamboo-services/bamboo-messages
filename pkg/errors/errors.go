// Package errors 提供 Bamboo SDK 统一的错误类型。
package errors

import "fmt"

// BambooError Bamboo SDK 统一错误类型。
//
// 封装底层错误信息，使用 Category 分类错误，同时保留可选的 HTTP 状态码。
type BambooError struct {
	// Category 错误分类
	Category string `json:"category"`

	// Message 错误描述信息
	Message string `json:"message"`

	// StatusCode 可选的 HTTP 状态码
	StatusCode int `json:"status_code,omitempty"`

	// cause 保留底层错误（不导出、不参与 JSON），供 errors.Is / errors.As
	// 错误链穿透使用，例如客户端取消时保留 context.Canceled 语义。
	cause error
}

// Error 实现 error 接口。
func (e *BambooError) Error() string {
	return fmt.Sprintf("Bamboo[%s]错误: %s", e.Category, e.Message)
}

// Unwrap 返回底层 cause，使 *BambooError 可参与 errors.Is / errors.As 链。
// 无 cause 时返回 nil（errors.Is 会跳过本层继续向下匹配）。
func (e *BambooError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// NewBambooError 创建 BambooError 实例。
//
// 参数:
//   - category - 错误分类
//   - message - 错误描述信息
//   - statusCode - HTTP 状态码（可选，无特定状态码可传 0）
func NewBambooError(category string, message string, statusCode int) *BambooError {
	return NewBambooErrorWithCause(category, message, statusCode, nil)
}

// NewBambooErrorWithCause 创建携带底层 cause 的 BambooError 实例。
//
// 与 NewBambooError 的唯一区别是保留 cause 供 errors.Is / errors.As 穿透，
// 例如 SDK 在 ctx 取消时可用它保留 context.Canceled 语义，让上层
// 通过 errors.Is(err, context.Canceled) 识别"客户端取消"而非笼统的 SDK 错误。
//
// 参数:
//   - category - 错误分类
//   - message - 错误描述信息
//   - statusCode - HTTP 状态码（可选，无特定状态码可传 0）
//   - cause - 底层错误（可为 nil）
func NewBambooErrorWithCause(category string, message string, statusCode int, cause error) *BambooError {
	return &BambooError{
		Category:   category,
		Message:    message,
		StatusCode: statusCode,
		cause:      cause,
	}
}

// 确保 BambooError 实现 error 接口。
var _ error = (*BambooError)(nil)

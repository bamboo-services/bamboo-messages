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
}

// Error 实现 error 接口。
func (e *BambooError) Error() string {
	return fmt.Sprintf("Bamboo[%s]错误: %s", e.Category, e.Message)
}

// NewBambooError 创建 BambooError 实例。
//
// 参数:
//   - category - 错误分类
//   - message - 错误描述信息
//   - statusCode - HTTP 状态码（可选，无特定状态码可传 0）
func NewBambooError(category string, message string, statusCode int) *BambooError {
	return &BambooError{
		Category:   category,
		Message:    message,
		StatusCode: statusCode,
	}
}

// 确保 BambooError 实现 error 接口。
var _ error = (*BambooError)(nil)

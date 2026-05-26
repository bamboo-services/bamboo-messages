package bamboo

import "fmt"

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

// Error 实现 error 接口，返回格式化的错误信息。
func (e *BambooError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s (code: %s)", e.Type, e.Message, e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// NewBambooError 创建 BambooError 实例。
func NewBambooError(errorType, message string) *BambooError {
	return &BambooError{
		Type:    errorType,
		Message: message,
	}
}

// NewBambooErrorWithCode 创建带错误代码的 BambooError 实例。
func NewBambooErrorWithCode(errorType, message, code string) *BambooError {
	return &BambooError{
		Type:    errorType,
		Message: message,
		Code:    code,
	}
}

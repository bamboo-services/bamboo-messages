package bamboo

import "fmt"

// BambooError Bamboo SDK 统一错误类型。
//
// 封装底层 AI 服务商返回的错误信息，保留原始错误类型和消息，
// 同时附加协议类型标识以便上层区分错误来源。
// StatusCode 字段保留上游 HTTP 状态码，使 429/401/400 等状态码
// 作为结构化字段贯穿错误链路，而非仅存在于消息字符串中。
type BambooError struct {
	// Type 错误类型标识
	Type string `json:"type"`

	// Message 错误描述信息
	Message string `json:"message"`

	// Code 错误代码（可选，由服务商定义）
	Code string `json:"code,omitempty"`

	// ProviderType 底层协议类型（可选，标识错误来源的协议适配器）
	ProviderType string `json:"provider_type,omitempty"`

	// StatusCode 上游 HTTP 状态码（如 429/401/500），0 表示不适用。
	// 由 Provider 适配器在 HTTP >= 400 时填充，贯穿 convert → codec 链路。
	StatusCode int `json:"status_code,omitempty"`
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

// NewBambooErrorWithStatusCode 创建带 HTTP 状态码的 BambooError 实例。
//
// 在 Provider 适配器或 bamboo.go 的错误转换路径中使用，
// 确保状态码作为结构化字段贯穿整个错误链路。
//
// 参数:
//   - statusCode - 上游 HTTP 状态码（如 429/401/500）
//   - message - 错误描述信息
func NewBambooErrorWithStatusCode(statusCode int, message string) *BambooError {
	return &BambooError{
		Type:       MapStatusCodeToErrorType(statusCode),
		Message:    message,
		StatusCode: statusCode,
	}
}

// MapStatusCodeToErrorType 将 HTTP 状态码映射为 BambooError 错误类型。
//
// 映射规则:
//   - 429 → ErrorTypeRateLimit（请求频率超限）
//   - 401/403 → ErrorTypeAuthentication（认证错误）
//   - 400 → ErrorTypeInvalidRequest（请求参数错误）
//   - 5xx → ErrorTypeAPI（服务端 API 错误）
//   - 其他 → ErrorTypeProvider（底层协议适配器错误）
func MapStatusCodeToErrorType(statusCode int) string {
	switch {
	case statusCode == 429:
		return ErrorTypeRateLimit
	case statusCode == 401 || statusCode == 403:
		return ErrorTypeAuthentication
	case statusCode == 400:
		return ErrorTypeInvalidRequest
	case statusCode >= 500:
		return ErrorTypeAPI
	default:
		return ErrorTypeProvider
	}
}

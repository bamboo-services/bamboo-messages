package anthropic

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// anthropicErrorResponse Anthropic 错误响应 JSON 结构。
//
// 参见: https://docs.anthropic.com/en/api/errors
type anthropicErrorResponse struct {
	Type  string             `json:"type"`
	Error anthropicErrorBody `json:"error"`
}

type anthropicErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// serializeError 将错误序列化为 Anthropic 格式的错误响应。
//
// 输出格式:
//
//	{"type":"error","error":{"type":"invalid_request_error","message":"..."}}
//
// 错误类型通过 BambooError.StatusCode 映射:
//   - 401/403 → authentication_error
//   - 429     → rate_limit_error
//   - 4xx     → invalid_request_error
//   - 5xx/0   → api_error
func serializeError(err error) []byte {
	errType := "api_error"
	message := "internal error"

	var bambooErr *bamboo.BambooError
	if errors.As(err, &bambooErr) {
		message = bambooErr.Message
		errType = mapStatusCodeToAnthropicType(bambooErr.StatusCode)
	} else if err != nil {
		message = err.Error()
	}

	resp := anthropicErrorResponse{
		Type: "error",
		Error: anthropicErrorBody{
			Type:    errType,
			Message: message,
		},
	}

	data, _ := json.Marshal(resp)
	return data
}

// mapStatusCodeToAnthropicType 将 HTTP 状态码映射为 Anthropic 错误类型字符串。
func mapStatusCodeToAnthropicType(statusCode int) string {
	switch {
	case statusCode == 401 || statusCode == 403:
		return "authentication_error"
	case statusCode == 429:
		return "rate_limit_error"
	case statusCode >= 400 && statusCode < 500:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

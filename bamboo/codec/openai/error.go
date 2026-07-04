package openai

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// openaiErrorResponse OpenAI 错误响应 JSON 结构。
type openaiErrorResponse struct {
	Error openaiErrorBody `json:"error"`
}

type openaiErrorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

// serializeError 将错误序列化为 OpenAI 格式的错误响应。
//
// 错误类型通过 BambooError.StatusCode 映射:
//   - 401/403 → authentication_error
//   - 429     → rate_limit_exceeded
//   - 4xx     → invalid_request_error
//   - 5xx/0   → api_error
func serializeError(err error) []byte {
	errType := "api_error"
	message := "internal error"

	var bambooErr *bamboo.BambooError
	if errors.As(err, &bambooErr) {
		message = bambooErr.Message
		errType = mapStatusCodeToOpenAIType(bambooErr.StatusCode)
	} else if err != nil {
		message = err.Error()
	}

	resp := openaiErrorResponse{
		Error: openaiErrorBody{
			Message: message,
			Type:    errType,
			Param:   nil,
			Code:    nil,
		},
	}

	data, _ := json.Marshal(resp)
	return data
}

// mapStatusCodeToOpenAIType 将 HTTP 状态码映射为 OpenAI 错误类型字符串。
func mapStatusCodeToOpenAIType(statusCode int) string {
	switch {
	case statusCode == 401 || statusCode == 403:
		return "authentication_error"
	case statusCode == 429:
		return "rate_limit_exceeded"
	case statusCode >= 400 && statusCode < 500:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

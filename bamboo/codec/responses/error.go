package responses

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// responsesErrorResponse OpenAI Responses 格式的错误响应 JSON 结构。
type responsesErrorResponse struct {
	Type    string      `json:"type"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Param   interface{} `json:"param"`
	EventID interface{} `json:"event_id"`
}

// serializeError 将错误序列化为 OpenAI Responses 格式的错误响应。
//
// 错误代码通过 BambooError.StatusCode 映射:
//   - 401/403 → authentication_error
//   - 429     → rate_limit_exceeded
//   - 4xx     → invalid_request
//   - 5xx/0   → server_error
func serializeError(err error) []byte {
	code := "server_error"
	message := "internal error"

	var bambooErr *bamboo.BambooError
	if errors.As(err, &bambooErr) {
		message = bambooErr.Message
		code = mapStatusCodeToResponsesCode(bambooErr.StatusCode)
	} else if err != nil {
		message = err.Error()
	}

	resp := responsesErrorResponse{
		Type:    "error",
		Code:    code,
		Message: message,
		Param:   nil,
		EventID: nil,
	}

	data, _ := json.Marshal(resp)
	return data
}

// mapStatusCodeToResponsesCode 将 HTTP 状态码映射为 Responses 错误代码。
func mapStatusCodeToResponsesCode(statusCode int) string {
	switch {
	case statusCode == 401 || statusCode == 403:
		return "authentication_error"
	case statusCode == 429:
		return "rate_limit_exceeded"
	case statusCode >= 400 && statusCode < 500:
		return "invalid_request"
	default:
		return "server_error"
	}
}

package gemini

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// geminiErrorResponse Gemini 非流式错误响应结构。
//
// 参考: https://cloud.google.com/apis/design/errors#http_mapping
type geminiErrorResponse struct {
	Error geminiErrorBody `json:"error"`
}

type geminiErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// serializeError 将错误序列化为 Gemini 格式的错误响应。
//
// 错误 code + status 通过 BambooError.StatusCode 映射（遵循 Google API 规范）:
//   - 400 → INVALID_ARGUMENT
//   - 401 → UNAUTHENTICATED
//   - 403 → PERMISSION_DENIED
//   - 429 → RESOURCE_EXHAUSTED
//   - 5xx/0 → INTERNAL
func serializeError(err error) []byte {
	code := 500
	status := "INTERNAL"
	message := "internal error"

	var bambooErr *bamboo.BambooError
	if errors.As(err, &bambooErr) {
		message = bambooErr.Message
		code, status = mapStatusCodeToGemini(bambooErr.StatusCode)
	} else if err != nil {
		message = err.Error()
	}

	resp := geminiErrorResponse{
		Error: geminiErrorBody{
			Code:    code,
			Message: message,
			Status:  status,
		},
	}

	data, _ := json.Marshal(resp)
	return data
}

// mapStatusCodeToGemini 将 HTTP 状态码映射为 Gemini error code + status。
func mapStatusCodeToGemini(statusCode int) (int, string) {
	switch {
	case statusCode == 400:
		return 400, "INVALID_ARGUMENT"
	case statusCode == 401:
		return 401, "UNAUTHENTICATED"
	case statusCode == 403:
		return 403, "PERMISSION_DENIED"
	case statusCode == 429:
		return 429, "RESOURCE_EXHAUSTED"
	case statusCode >= 400 && statusCode < 500:
		return 400, "INVALID_ARGUMENT"
	default:
		return 500, "INTERNAL"
	}
}

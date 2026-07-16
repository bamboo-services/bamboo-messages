package bamboo

import (
	"encoding/json"
	"errors"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
)

// bambooErrorResponse bamboo 原生错误响应 JSON 结构。
//
// 输出格式:
//
//	{"type":"error","error":{"category":"...","message":"...","status_code":...}}
type bambooErrorResponse struct {
	Type  string             `json:"type"`
	Error bambooErrorPayload `json:"error"`
}

// bambooErrorPayload 错误负载，直接复用 BambooError 的三字段结构。
type bambooErrorPayload struct {
	Category   string `json:"category"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
}

// serializeError 将错误序列化为 bamboo 原生格式的错误响应。
//
// 若 err 为 *bamboo.BambooError 则直接透传其字段；
// 否则降级为 Category="SDK" 的通用错误。
func serializeError(err error) []byte {
	category := "SDK"
	message := "internal error"
	statusCode := 0

	var bambooErr *bmbamboo.BambooError
	if errors.As(err, &bambooErr) {
		category = bambooErr.Category
		message = bambooErr.Message
		statusCode = bambooErr.StatusCode
	} else if err != nil {
		message = err.Error()
	}

	resp := bambooErrorResponse{
		Type: "error",
		Error: bambooErrorPayload{
			Category:   category,
			Message:    message,
			StatusCode: statusCode,
		},
	}

	data, _ := json.Marshal(resp)
	return data
}

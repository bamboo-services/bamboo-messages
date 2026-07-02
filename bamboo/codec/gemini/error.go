package gemini

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
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
func serializeError(err error) []byte {
	code := 500
	status := "INTERNAL"
	message := "internal error"

	var codecErr *codec.CodecError
	var bambooErr *bamboo.BambooError
	if errors.As(err, &codecErr) {
		message = codecErr.Message
		code, status = mapCodecErrorToGemini(codecErr.Type)
	} else if errors.As(err, &bambooErr) {
		message = bambooErr.Message
		code, status = mapBambooErrorToGemini(bambooErr)
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

// mapCodecErrorToGemini 将 codec ErrorType 映射为 Gemini error code + status。
//
// 映射规则（遵循 Google API 规范）:
//   - ErrInvalidRequest → 400, INVALID_ARGUMENT
//   - ErrAuthError      → 401, UNAUTHENTICATED
//   - ErrRateLimit      → 429, RESOURCE_EXHAUSTED
//   - ErrProviderError  → 500, INTERNAL
//   - ErrInternal       → 500, INTERNAL
func mapCodecErrorToGemini(t codec.ErrorType) (int, string) {
	switch t {
	case codec.ErrInvalidRequest:
		return 400, "INVALID_ARGUMENT"
	case codec.ErrAuthError:
		return 401, "UNAUTHENTICATED"
	case codec.ErrRateLimit:
		return 429, "RESOURCE_EXHAUSTED"
	case codec.ErrProviderError:
		return 500, "INTERNAL"
	case codec.ErrInternal:
		return 500, "INTERNAL"
	default:
		return 500, "INTERNAL"
	}
}

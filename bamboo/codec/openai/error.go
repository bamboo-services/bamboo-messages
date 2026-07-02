package openai

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
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
func serializeError(err error) []byte {
	errType := "api_error"
	message := "internal error"

	var codecErr *codec.CodecError
	var bambooErr *bamboo.BambooError
	if errors.As(err, &codecErr) {
		message = codecErr.Message
		errType = mapCodecErrorType(codecErr.Type)
	} else if errors.As(err, &bambooErr) {
		message = bambooErr.Message
		errType = bambooErr.Type
		if errType == "" {
			errType = "api_error"
		}
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

// mapCodecErrorType 将 codec ErrorType 映射为 OpenAI 错误类型字符串。
func mapCodecErrorType(t codec.ErrorType) string {
	switch t {
	case codec.ErrInvalidRequest:
		return "invalid_request_error"
	case codec.ErrProviderError:
		return "api_error"
	case codec.ErrAuthError:
		return "authentication_error"
	case codec.ErrRateLimit:
		return "rate_limit_exceeded"
	case codec.ErrInternal:
		return "api_error"
	default:
		return "api_error"
	}
}

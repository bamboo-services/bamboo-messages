package responses

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
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
func serializeError(err error) []byte {
	code := "server_error"
	message := "internal error"

	var codecErr *codec.CodecError
	var bambooErr *bamboo.BambooError
	if errors.As(err, &codecErr) {
		message = codecErr.Message
		code = mapResponsesErrorCode(codecErr.Type)
	} else if errors.As(err, &bambooErr) {
		message = bambooErr.Message
		code = mapBambooErrorToResponsesCode(bambooErr)
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

// mapResponsesErrorCode 将 codec ErrorType 映射为 Responses 错误代码。
func mapResponsesErrorCode(t codec.ErrorType) string {
	switch t {
	case codec.ErrInvalidRequest:
		return "invalid_request"
	case codec.ErrProviderError:
		return "provider_error"
	case codec.ErrAuthError:
		return "authentication_error"
	case codec.ErrRateLimit:
		return "rate_limit_exceeded"
	case codec.ErrInternal:
		return "server_error"
	default:
		return "server_error"
	}
}

func mapBambooErrorToResponsesCode(err *bamboo.BambooError) string {
	switch err.Type {
	case bamboo.ErrorTypeInvalidRequest:
		return "invalid_request"
	case bamboo.ErrorTypeAuthentication:
		return "authentication_error"
	case bamboo.ErrorTypeRateLimit:
		return "rate_limit_exceeded"
	case bamboo.ErrorTypeAPI, bamboo.ErrorTypeProvider:
		return "provider_error"
	default:
		return "server_error"
	}
}

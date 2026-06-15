package anthropic

import (
	"encoding/json"
	"errors"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
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
func serializeError(err error) []byte {
	errType := "api_error"
	message := "internal error"

	// 优先尝试提取 CodecError
	var codecErr *codec.CodecError
	if errors.As(err, &codecErr) {
		message = codecErr.Message
		errType = mapCodecErrorType(codecErr.Type)
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

// mapCodecErrorType 将 codec ErrorType 映射为 Anthropic 错误类型字符串。
//
// 映射规则:
//   - ErrInvalidRequest → "invalid_request_error"
//   - ErrProviderError  → "api_error"
//   - ErrAuthError      → "authentication_error"
//   - ErrRateLimit      → "rate_limit_error"
//   - 其他              → "api_error"
func mapCodecErrorType(t codec.ErrorType) string {
	switch t {
	case codec.ErrInvalidRequest:
		return "invalid_request_error"
	case codec.ErrProviderError:
		return "api_error"
	case codec.ErrAuthError:
		return "authentication_error"
	case codec.ErrRateLimit:
		return "rate_limit_error"
	case codec.ErrInternal:
		return "api_error"
	default:
		return "api_error"
	}
}

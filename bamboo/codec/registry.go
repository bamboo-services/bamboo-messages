package codec

import "fmt"

// 全局 Codec 注册变量。
//
// 由各格式子包的 init() 函数或显式赋值设置。
// 使用前必须确保对应变量已被赋值，否则 Get() 返回的 Codec 为 nil。
var (
	// OpenAI OpenAI Chat Completions 协议的 Codec 实例。
	OpenAI Codec

	// Anthropic Anthropic Messages 协议的 Codec 实例。
	Anthropic Codec

	// Responses OpenAI Responses 协议的 Codec 实例。
	Responses Codec

	// Gemini Google Gemini 协议的 Codec 实例。
	Gemini Codec

	// Bamboo bamboo 原生协议的 Codec 实例。
	Bamboo Codec
)

// Get 根据格式标识获取已注册的 Codec 实例。
//
// 若对应格式的 Codec 尚未注册（为 nil），仍会返回 nil 和 nil error，
// 调用方应自行判断 Codec 是否可用。
func Get(format FormatType) (Codec, error) {
	switch format {
	case FormatOpenAI:
		return OpenAI, nil
	case FormatAnthropic:
		return Anthropic, nil
	case FormatResponses:
		return Responses, nil
	case FormatGemini:
		return Gemini, nil
	case FormatBamboo:
		return Bamboo, nil
	default:
		return nil, fmt.Errorf("codec: unsupported format %q", format)
	}
}

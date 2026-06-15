package codec

import "github.com/bamboo-services/bamboo-messages/bamboo"

// FormatType 标识一种外部协议格式。
type FormatType string

const (
	// FormatOpenAI OpenAI Chat Completions 协议格式。
	FormatOpenAI FormatType = "openai"

	// FormatAnthropic Anthropic Messages 协议格式。
	FormatAnthropic FormatType = "anthropic"

	// FormatResponses OpenAI Responses 协议格式。
	FormatResponses FormatType = "responses"

	// FormatGemini Google Gemini 协议格式。
	FormatGemini FormatType = "gemini"
)

// Codec 一种外部协议格式的完整编解码能力。
//
// 负责将外部协议的请求体解析为统一的 RelayRequest，
// 并将 Bamboo 内部类型序列化为外部协议格式的响应。
type Codec interface {
	// Format 返回该 Codec 支持的协议格式标识。
	Format() FormatType

	// ParseRequest 将外部协议的请求体解析为统一的 RelayRequest。
	ParseRequest(body []byte) (*RelayRequest, error)

	// SerializeResponse 将 Bamboo 统一响应序列化为外部协议格式的 JSON 字节。
	SerializeResponse(resp *bamboo.Response) ([]byte, error)

	// SerializeError 将错误序列化为外部协议格式的错误响应字节。
	SerializeError(err error) []byte

	// NewSerializer 创建一个新的流式序列化器实例。
	//
	// 每个流需要一个独立的 StreamSerializer，因为它是有状态的。
	NewSerializer() StreamSerializer
}

// StreamSerializer 流式序列化器，每个流创建一个独立实例。
//
// 将 Bamboo 的 StreamEvent 转换为外部协议格式的 SSE 数据帧。
type StreamSerializer interface {
	// Serialize 将单个 StreamEvent 序列化为外部协议格式的数据帧。
	//
	// 返回的字节通常是一个完整的 SSE data: 行（含换行符）。
	Serialize(event bamboo.StreamEvent) ([]byte, error)

	// Flush 刷新缓冲区，返回所有待发送的数据。
	//
	// 在流结束时调用，确保所有缓冲数据被发送。
	Flush() ([]byte, error)
}

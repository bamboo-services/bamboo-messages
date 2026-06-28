package openai

import (
	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// openaiCodec OpenAI Chat Completions 协议的 Codec 实现。
type openaiCodec struct{}

// Format 返回该 Codec 支持的协议格式标识。
func (c *openaiCodec) Format() codec.FormatType {
	return codec.FormatOpenAI
}

// ParseRequest 将 OpenAI Chat Completions 请求体解析为统一 RelayRequest。
func (c *openaiCodec) ParseRequest(body []byte) (*codec.RelayRequest, error) {
	return parseRequest(body)
}

// SerializeResponse 将 Bamboo 统一响应序列化为 OpenAI Chat Completions JSON。
func (c *openaiCodec) SerializeResponse(resp *bamboo.Response) ([]byte, error) {
	return serializeResponse(resp)
}

// SerializeError 将错误序列化为 OpenAI 格式的错误响应。
func (c *openaiCodec) SerializeError(err error) []byte {
	return serializeError(err)
}

// NewSerializer 创建一个新的 OpenAI 流式序列化器实例。
func (c *openaiCodec) NewSerializer(model string) codec.StreamSerializer {
	return newStreamSerializer(model)
}

// Codec 全局 OpenAI Codec 实例，供外部直接使用。
var Codec codec.Codec = &openaiCodec{}

// init 注册到 codec 全局变量。
func init() {
	codec.OpenAI = Codec
}

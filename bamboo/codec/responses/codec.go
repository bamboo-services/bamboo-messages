package responses

import (
	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// responsesCodec OpenAI Responses 协议的 Codec 实现。
type responsesCodec struct{}

// Format 返回该 Codec 支持的协议格式标识。
func (c *responsesCodec) Format() codec.FormatType {
	return codec.FormatResponses
}

// ParseRequest 将 OpenAI Responses 请求体解析为统一 RelayRequest。
func (c *responsesCodec) ParseRequest(body []byte) (*codec.RelayRequest, error) {
	return parseRequest(body)
}

// SerializeResponse 将 Bamboo 统一响应序列化为 OpenAI Responses JSON。
func (c *responsesCodec) SerializeResponse(resp *bamboo.Response) ([]byte, error) {
	return serializeResponse(resp)
}

// SerializeError 将错误序列化为 OpenAI Responses 格式的错误响应。
func (c *responsesCodec) SerializeError(err error) []byte {
	return serializeError(err)
}

// NewSerializer 创建一个新的 OpenAI Responses 流式序列化器实例。
func (c *responsesCodec) NewSerializer(model string) codec.StreamSerializer {
	return newStreamSerializer(model)
}

// Codec 全局 OpenAI Responses Codec 实例，供外部直接使用。
var Codec codec.Codec = &responsesCodec{}

// init 注册到 codec 全局变量。
func init() {
	codec.Responses = Codec
}

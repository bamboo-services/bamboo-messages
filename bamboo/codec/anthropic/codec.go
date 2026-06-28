package anthropic

import (
	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// anthropicCodec Anthropic Messages 协议的 Codec 实现。
type anthropicCodec struct{}

// Format 返回该 Codec 支持的协议格式标识。
func (c *anthropicCodec) Format() codec.FormatType {
	return codec.FormatAnthropic
}

// ParseRequest 将 Anthropic Messages 请求体解析为统一 RelayRequest。
func (c *anthropicCodec) ParseRequest(body []byte) (*codec.RelayRequest, error) {
	return parseRequest(body)
}

// SerializeResponse 将 Bamboo 统一响应序列化为 Anthropic Messages JSON。
func (c *anthropicCodec) SerializeResponse(resp *bamboo.Response) ([]byte, error) {
	return serializeResponse(resp)
}

// SerializeError 将错误序列化为 Anthropic 格式的错误响应。
func (c *anthropicCodec) SerializeError(err error) []byte {
	return serializeError(err)
}

// NewSerializer 创建一个新的 Anthropic 流式序列化器实例。
func (c *anthropicCodec) NewSerializer(model string) codec.StreamSerializer {
	return newStreamSerializer(model)
}

// Codec 全局 Anthropic Codec 实例，供外部直接使用。
var Codec codec.Codec = &anthropicCodec{}

// init 注册到 codec 全局变量。
func init() {
	codec.Anthropic = Codec
}

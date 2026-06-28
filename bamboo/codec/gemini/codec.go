package gemini

import (
	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// geminiCodec Google Gemini 协议的 Codec 实现。
type geminiCodec struct{}

// Format 返回该 Codec 支持的协议格式标识。
func (c *geminiCodec) Format() codec.FormatType {
	return codec.FormatGemini
}

// ParseRequest 将 Gemini GenerateContent 请求体解析为统一 RelayRequest。
func (c *geminiCodec) ParseRequest(body []byte) (*codec.RelayRequest, error) {
	return parseRequest(body)
}

// SerializeResponse 将 Bamboo 统一响应序列化为 Gemini GenerateContentResponse JSON。
func (c *geminiCodec) SerializeResponse(resp *bamboo.Response) ([]byte, error) {
	return serializeResponse(resp)
}

// SerializeError 将错误序列化为 Gemini 格式的错误响应。
func (c *geminiCodec) SerializeError(err error) []byte {
	return serializeError(err)
}

// NewSerializer 创建一个新的 Gemini 流式序列化器实例。
func (c *geminiCodec) NewSerializer(model string) codec.StreamSerializer {
	return newStreamSerializer(model)
}

// Codec 全局 Gemini Codec 实例，供外部直接使用。
var Codec codec.Codec = &geminiCodec{}

// init 注册到 codec 全局变量。
func init() {
	codec.Gemini = Codec
}

package bamboo

import (
	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
	bmcodec "github.com/bamboo-services/bamboo-messages/bamboo/codec"
)

// bambooCodec bamboo 原生协议的 Codec 实现。
//
// 作为 identity-transform Codec：请求体和响应体均直接使用 bamboo 原生类型，
// 不做任何协议格式转换。仅用于 bamboo ↔ bamboo 的同协议直连场景。
type bambooCodec struct{}

// Format 返回该 Codec 支持的协议格式标识。
func (c *bambooCodec) Format() bmcodec.FormatType {
	return bmcodec.FormatBamboo
}

// ParseRequest 将 bamboo 原生请求体解析为统一 RelayRequest。
func (c *bambooCodec) ParseRequest(body []byte) (*bmcodec.RelayRequest, error) {
	return parseRequest(body)
}

// SerializeResponse 将 Bamboo 统一响应序列化为 bamboo 原生 JSON。
func (c *bambooCodec) SerializeResponse(resp *bmbamboo.Response) ([]byte, error) {
	return serializeResponse(resp)
}

// SerializeError 将错误序列化为 bamboo 原生格式的错误响应。
func (c *bambooCodec) SerializeError(err error) []byte {
	return serializeError(err)
}

// NewSerializer 创建一个新的 bamboo 流式序列化器实例。
func (c *bambooCodec) NewSerializer(model string) bmcodec.StreamSerializer {
	return newStreamSerializer(model)
}

// Codec 全局 bamboo Codec 实例，供外部直接使用。
var Codec bmcodec.Codec = &bambooCodec{}

// init 注册到 codec 全局变量。
func init() {
	bmcodec.Bamboo = Codec
}

package codec

import "github.com/bamboo-services/bamboo-messages/bamboo"

// RelayRequest 解析后的统一请求中间表示。
//
// 由各 Codec 的 ParseRequest 从外部协议请求体解析得到，
// 是 Codec 层向 Bamboo 内部传递请求的核心类型。
type RelayRequest struct {
	// Messages 对话消息列表，已从外部协议格式转换为 Bamboo 统一格式。
	Messages []bamboo.BambooMessage

	// System 系统提示词。
	System string

	// Config 请求配置，已从外部协议格式转换为 Bamboo 统一格式。
	Config *bamboo.RequestConfig

	// IsStream 是否为流式请求。
	IsStream bool
}

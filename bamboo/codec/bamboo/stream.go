package bamboo

import (
	"encoding/json"
	"fmt"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
	bmcodec "github.com/bamboo-services/bamboo-messages/bamboo/codec"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

// bambooStreamSerializer bamboo 原生协议的流式序列化器。
//
// bamboo 的 StreamEvent 本身即为 Anthropic 风格的自描述事件结构，
// 序列化只需对整个 StreamEvent 做 JSON 编码并封装为 SSE event/data 双行帧，
// 无需按事件类型分派处理。所有 8 种事件类型（message_start / content_block_start /
// content_block_delta / content_block_stop / message_delta / message_stop / ping / error）
// 均统一走同一路径。
//
// 无 [DONE] 标记 —— message_stop 事件即为终止信号，与 Anthropic 协议一致。
type bambooStreamSerializer struct {
	model string
}

// newStreamSerializer 创建一个新的 bamboo 流式序列化器实例。
//
// 每个流创建独立实例，避免跨流状态污染。model 参数保留以兼容 Codec 接口约定，
// 当前实现不主动注入 model 字段（StreamEvent 自身已携带完整信息）。
func newStreamSerializer(model string) bmcodec.StreamSerializer {
	return &bambooStreamSerializer{model: model}
}

// Serialize 将单个 StreamEvent 序列化为 bamboo 原生 SSE 数据帧。
//
// 输出格式: `event: {type}\ndata: {json}\n\n`
//
// StreamEvent 的 Delta 字段（any 类型）可持有 *StreamDelta 或 *MessageDelta，
// ContentBlock 字段（接口类型）可持有 *TextBlock 等具体指针，
// json.Marshal 会依据具体类型的 json tag 正确序列化，无需手动分派。
func (s *bambooStreamSerializer) Serialize(event bmbamboo.StreamEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, pkgErrors.NewBambooError("下游", fmt.Sprintf("failed to marshal stream event: %v", err), 0)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, data)), nil
}

// Flush 刷新缓冲区。
//
// bamboo 流式协议没有 [DONE] 标记，message_stop 事件即为终止信号，
// 因此 Flush 无需输出任何内容。
func (s *bambooStreamSerializer) Flush() ([]byte, error) {
	return nil, nil
}

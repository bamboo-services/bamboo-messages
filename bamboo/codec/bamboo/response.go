package bamboo

import (
	"encoding/json"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
)

// serializeResponse 将 Bamboo 统一响应序列化为 bamboo 原生 JSON。
//
// identity 转换：直接 json.Marshal(*bamboo.Response)，
// ContentBlock 的多态序列化由各 block 类型自身的 JSON tag 保证。
func serializeResponse(resp *bmbamboo.Response) ([]byte, error) {
	return json.Marshal(resp)
}

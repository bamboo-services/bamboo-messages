package bamboo

import (
	"encoding/json"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
	bmcodec "github.com/bamboo-services/bamboo-messages/bamboo/codec"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

// bambooEnvelope bamboo 原生请求的信封结构。
//
// 作为 identity-transform，直接复用 bamboo 门面层的类型，
// 不引入任何中间 DTO。
type bambooEnvelope struct {
	Messages []bmbamboo.BambooMessage `json:"messages"`
	System   string                   `json:"system,omitempty"`
	Config   *bmbamboo.RequestConfig  `json:"config,omitempty"`
	Stream   bool                     `json:"stream,omitempty"`
}

// parseRequest 将 bamboo 原生请求体解析为统一 RelayRequest。
//
// identity 转换：JSON → bambooEnvelope → RelayRequest。
// 若 Config 为 nil，则填充零值 RequestConfig，保证下游使用安全。
func parseRequest(body []byte) (*bmcodec.RelayRequest, error) {
	var env bambooEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, pkgErrors.NewBambooError("下游", "failed to parse bamboo request body", 0)
	}

	if env.Config == nil {
		env.Config = &bmbamboo.RequestConfig{}
	}

	return &bmcodec.RelayRequest{
		Messages: env.Messages,
		System:   env.System,
		Config:   env.Config,
		IsStream: env.Stream,
	}, nil
}

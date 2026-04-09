package responses

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ResponsesProvider OpenAI (GPT) AI 服务提供商实现，基于 Responses API
type ResponsesProvider provider.BaseProvider[openai.Client]

// NewResponsesProvider 创建 OpenAI Responses Provider 实例
func NewResponsesProvider(apiKey string) *ResponsesProvider {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &ResponsesProvider{
		Client: client,
	}
}

// GetProviderType 获取提供商类型
func (p *ResponsesProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAIResponses
}

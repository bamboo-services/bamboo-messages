package completions

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// CompletionsProvider OpenAI (GPT) AI 服务提供商实现，基于 Chat Completions API
type CompletionsProvider provider.BaseProvider[openai.Client]

// NewCompletionsProvider 创建 OpenAI Completions Provider 实例
func NewCompletionsProvider(apiKey string) *CompletionsProvider {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &CompletionsProvider{
		Client: client,
	}
}

// GetProviderType 获取提供商类型
func (p *CompletionsProvider) GetProviderType() provider.ProviderType {
	return provider.ProviderOpenAICompletions
}

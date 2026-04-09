package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Provider Anthropic (Claude) AI 服务提供商实现
type Provider provider.BaseProvider[anthropic.Client]

// NewProvider 创建 Anthropic Provider 实例
func NewProvider(apiKey string) *Provider {
	client := anthropic.NewClient(
		option.WithHeader("User-Agent", "vesper-ling/agent 0.0.1"),
		option.WithAPIKey(apiKey),
	)

	return &Provider{
		Client: client,
	}
}

// GetProviderType 获取提供商类型
func (p *Provider) GetProviderType() provider.ProviderType {
	return provider.ProviderAnthropic
}

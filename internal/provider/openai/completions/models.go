package completions

import (
	"github.com/openai/openai-go/v3"
)

// GetAvailableModels 获取可用模型列表。
//
// 返回 OpenAI Chat Completions 协议支持的模型名称数组。
func (p *CompletionsProvider) GetAvailableModels() []string {
	return []string{
		openai.ChatModelGPT4o,
		openai.ChatModelGPT4oMini,
		openai.ChatModelGPT4_1,
		openai.ChatModelGPT4_1Mini,
		openai.ChatModelGPT4_1Nano,
		openai.ChatModelO3,
		openai.ChatModelO3Mini,
		openai.ChatModelO4Mini,
	}
}

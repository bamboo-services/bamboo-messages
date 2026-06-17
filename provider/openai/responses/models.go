package responses

import (
	"github.com/openai/openai-go/v3"
)

// GetAvailableModels 获取可用模型列表。
//
// 返回 OpenAI Responses 协议支持的模型名称列表，
// 包括 GPT-4.1、GPT-4o、O3、O4 等系列模型。
func (p *ResponsesProvider) GetAvailableModels() []string {
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

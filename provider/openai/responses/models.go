package responses

// OpenAI Responses API 支持的模型常量。
//
// 去 SDK 化后不再依赖 openai-go SDK 的模型常量，
// 此处直接使用字符串字面量，与 OpenAI 官方模型 ID 保持一致。
const (
	ModelGPT4o      = "gpt-4o"
	ModelGPT4oMini  = "gpt-4o-mini"
	ModelGPT4_1     = "gpt-4.1"
	ModelGPT4_1Mini = "gpt-4.1-mini"
	ModelGPT4_1Nano = "gpt-4.1-nano"
	ModelO3         = "o3"
	ModelO3Mini     = "o3-mini"
	ModelO4Mini     = "o4-mini"
)

// GetAvailableModels 获取可用模型列表。
//
// 返回 OpenAI Responses 协议支持的模型名称列表，
// 包括 GPT-4.1、GPT-4o、O3、O4 等系列模型。
func (p *ResponsesProvider) GetAvailableModels() []string {
	return []string{
		ModelGPT4o,
		ModelGPT4oMini,
		ModelGPT4_1,
		ModelGPT4_1Mini,
		ModelGPT4_1Nano,
		ModelO3,
		ModelO3Mini,
		ModelO4Mini,
	}
}

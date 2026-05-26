package anthropic

// GetAvailableModels 获取可用模型列表。
//
// 返回 Anthropic 支持的模型名称列表，包括 Claude 4 和 Claude 3 系列。
func (p *Provider) GetAvailableModels() []string {
	return []string{
		"claude-sonnet-4-20250514",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}
}

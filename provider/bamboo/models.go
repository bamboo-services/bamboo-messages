package bamboo

// GetAvailableModels 获取当前 Provider 支持的模型列表。
//
// 当前 bamboo 原生协议为开放端点，不固定模型白名单，返回空列表。
func (p *Provider) GetAvailableModels() []string {
	return []string{}
}

package gemini

// ============================================
// Gemini 模型常量
// ============================================

const (
	ModelGemini25Flash = "gemini-2.5-flash" // Gemini 2.5 Flash — 高速低成本
	ModelGemini25Pro   = "gemini-2.5-pro"   // Gemini 2.5 Pro — 最强推理
	ModelGemini20Flash = "gemini-2.0-flash" // Gemini 2.0 Flash — 上一代快速模型
)

// GetAvailableModels 获取可用模型列表。
//
// 返回 Gemini 支持的模型名称列表。
func (p *Provider) GetAvailableModels() []string {
	return []string{
		ModelGemini25Flash,
		ModelGemini25Pro,
		ModelGemini20Flash,
	}
}

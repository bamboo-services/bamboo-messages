package gemini

import "google.golang.org/genai"

// GeminiProviderOption 配置 Gemini 请求特有参数的函数选项。
//
// 用于设置 TopK、SafetySettings 等 Gemini 协议特有的请求参数。
// 与 Provider 的 Option（APIKey/BaseURL/Headers）不同，
// GeminiProviderOption 面向单次请求级别的参数配置。
type GeminiProviderOption func(*geminiRequestConfig)

// geminiRequestConfig Gemini 请求级别的特有参数配置。
//
// 存储 TopK、SafetySettings 等 Gemini 协议特有的请求参数。
// 后续在请求执行时，这些参数会被合并到 ProviderExtra 中透传。
type geminiRequestConfig struct {
	topK           *float64
	safetySettings []*genai.SafetySetting
}

// WithTopK 设置 Top-K 采样参数（Gemini 特有）。
//
// 控制模型在生成下一个 token 时，从概率最高的 K 个候选中随机选取。
// 值越小，输出越确定性；值越大，输出越多样性。
func WithTopK(v float64) GeminiProviderOption {
	return func(c *geminiRequestConfig) { c.topK = &v }
}

// WithSafetySettings 设置安全过滤配置。
//
// Gemini 原生支持按 harm category 和 threshold 进行内容安全过滤。
// 通过此 Option 可覆盖默认的安全策略。
func WithSafetySettings(settings []*genai.SafetySetting) GeminiProviderOption {
	return func(c *geminiRequestConfig) { c.safetySettings = settings }
}

package anthropic

// AnthropicMessagesOption 配置 Anthropic Messages 请求特有参数的函数选项。
//
// 用于设置 TopK、BudgetTokens 等 Anthropic 协议特有的请求参数。
// 与 Provider 的 Option（APIKey/BaseURL/Headers）不同，
// AnthropicMessagesOption 面向单次请求级别的参数配置。
type AnthropicMessagesOption func(*anthropicRequestConfig)

// anthropicRequestConfig Anthropic Messages 请求级别的特有参数配置。
//
// 存储 TopK、BudgetTokens 等 Anthropic 协议特有的请求参数。
// 后续在请求执行时，这些参数会被合并到 ProviderExtra 中透传。
type anthropicRequestConfig struct {
	topK         *float64
	budgetTokens *int64
}

// WithTopK 设置 Top-K 采样参数（Anthropic 特有）。
//
// 控制模型在生成下一个 token 时，从概率最高的 K 个候选中随机选取。
// 值越小，输出越确定性；值越大，输出越多样性。
func WithTopK(v float64) AnthropicMessagesOption {
	return func(c *anthropicRequestConfig) { c.topK = &v }
}

// WithBudgetTokens 设置思考 token 预算。
//
// 控制模型思考过程（thinking）可使用的最大 token 数。
//
// Deprecated: 请使用 ThinkingConfig.Effort 替代，Anthropic Opus 4.7+ 已废弃 BudgetTokens。
func WithBudgetTokens(v int64) AnthropicMessagesOption {
	return func(c *anthropicRequestConfig) { c.budgetTokens = &v }
}

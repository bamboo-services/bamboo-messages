package completions

// OpenaiCompletionsOption 配置 OpenAI Chat Completions 请求特有参数的函数选项。
//
// 用于设置 FrequencyPenalty、PresencePenalty、Seed、Prediction 等
// OpenAI Chat Completions 协议特有的请求参数。
// 与 Provider 的 Option（APIKey/BaseURL/Headers）不同，
// OpenaiCompletionsOption 面向单次请求级别的参数配置。
type OpenaiCompletionsOption func(*completionsRequestConfig)

// completionsRequestConfig OpenAI Chat Completions 请求级别的特有参数配置。
//
// 存储 FrequencyPenalty、PresencePenalty、Seed、Prediction 等
// OpenAI Chat Completions 协议特有的请求参数。
// 后续在请求执行时，这些参数会被合并到 ProviderExtra 中透传。
type completionsRequestConfig struct {
	frequencyPenalty *float64
	presencePenalty  *float64
	seed             *int64
	prediction       any
}

// WithFrequencyPenalty 设置频率惩罚参数。
//
// 正值会降低模型重复已出现 token 的概率，使输出更加多样化。
// 取值范围通常为 -2.0 ~ 2.0。
func WithFrequencyPenalty(v float64) OpenaiCompletionsOption {
	return func(c *completionsRequestConfig) { c.frequencyPenalty = &v }
}

// WithPresencePenalty 设置存在惩罚参数。
//
// 正值会降低模型谈论已出现话题的概率，使输出涵盖更多新话题。
// 取值范围通常为 -2.0 ~ 2.0。
func WithPresencePenalty(v float64) OpenaiCompletionsOption {
	return func(c *completionsRequestConfig) { c.presencePenalty = &v }
}

// WithSeed 设置随机种子。
//
// 指定后模型会尝试产生确定性输出（Best-effort deterministic），
// 相同的 seed + 相同的输入应产生相似的输出。
func WithSeed(v int64) OpenaiCompletionsOption {
	return func(c *completionsRequestConfig) { c.seed = &v }
}

// WithPrediction 设置预测内容。
//
// 用于 OpenAI 的 Predicted Outputs 功能，提供预期输出内容
// 以加速模型生成速度。类型取决于具体协议参数结构。
func WithPrediction(v any) OpenaiCompletionsOption {
	return func(c *completionsRequestConfig) { c.prediction = v }
}

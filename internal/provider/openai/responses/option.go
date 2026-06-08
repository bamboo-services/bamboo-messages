package responses

// OpenaiResponsesOption 配置 OpenAI Responses 请求特有参数的函数选项。
//
// 用于设置 Store、Modalities、PreviousResponseID、Truncation 等
// OpenAI Responses 协议特有的请求参数。
// 与 Provider 的 Option（APIKey/BaseURL/Headers）不同，
// OpenaiResponsesOption 面向单次请求级别的参数配置。
type OpenaiResponsesOption func(*responsesRequestConfig)

// responsesRequestConfig OpenAI Responses 请求级别的特有参数配置。
//
// 存储 Store、Modalities、PreviousResponseID、Truncation 等
// OpenAI Responses 协议特有的请求参数。
// 后续在请求执行时，这些参数会被合并到 ProviderExtra 中透传。
type responsesRequestConfig struct {
	store              *bool
	modalities         any
	previousResponseID *string
	truncation         *string
}

// WithStore 设置是否在 OpenAI 端存储响应。
//
// 为 true 时 OpenAI 会保存本次响应，可通过后续请求引用。
func WithStore(v bool) OpenaiResponsesOption {
	return func(c *responsesRequestConfig) { c.store = &v }
}

// WithModalities 设置输出模态。
//
// 指定模型输出应包含的模态类型，如文本、音频等。
// 类型取决于具体协议参数结构。
func WithModalities(v any) OpenaiResponsesOption {
	return func(c *responsesRequestConfig) { c.modalities = v }
}

// WithPreviousResponseID 设置前置响应 ID。
//
// 用于多轮对话链式调用，将前一次响应作为本次请求的上下文。
// OpenAI Responses 协议通过此 ID 实现对话历史关联。
func WithPreviousResponseID(v string) OpenaiResponsesOption {
	return func(c *responsesRequestConfig) { c.previousResponseID = &v }
}

// WithTruncation 设置消息截断策略。
//
// 当输入消息超过模型上下文窗口时，指定截断方式。
// 例如 "auto" 自动截断，保持最新消息优先。
func WithTruncation(v string) OpenaiResponsesOption {
	return func(c *responsesRequestConfig) { c.truncation = &v }
}

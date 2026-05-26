package bamboo

// RequestConfig 请求配置，用于控制 AI 模型的生成行为。
//
// Temperature 和 TopP 使用指针类型以区分"未设置"和"零值"两种状态：
//   - nil: 未设置，使用服务端默认值
//   - &0.0: 显式设置为 0
type RequestConfig struct {
	// Model 模型名称，如 "claude-sonnet-4-20250514"
	Model string `json:"model"`

	// MaxTokens 最大生成 token 数量
	MaxTokens int64 `json:"max_tokens"`

	// Temperature 温度参数 (0-2)，控制输出的随机性
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP Top-p 采样参数，控制输出的多样性
	TopP *float64 `json:"top_p,omitempty"`

	// StopSequences 停止序列列表，模型遇到其中任一序列时停止生成
	StopSequences []string `json:"stop_sequences,omitempty"`

	// Tools 可用工具列表
	Tools []Tool `json:"tools,omitempty"`

	// Metadata 附加元数据，用于传递请求级别的自定义信息
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PtrFloat64 返回 float64 指针，用于设置 RequestConfig 的可选字段。
//
// 用法: config.Temperature = PtrFloat64(0.7)
func PtrFloat64(v float64) *float64 {
	return &v
}

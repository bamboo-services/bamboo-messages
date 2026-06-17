package bamboo

import "github.com/bamboo-services/bamboo-messages/provider"

// ThinkingConfig 思考/推理配置，控制模型的推理行为。
//
// 类型别名，指向 provider 层的 ThinkingConfig。
type ThinkingConfig = provider.ThinkingConfig

// RequestConfig 请求配置，用于控制 AI 模型的生成行为。
//
// Temperature 和 TopP 使用指针类型以区分"未设置"和"零值"两种状态：
//   - nil: 未设置，使用服务端默认值
//   - &0.0: 显式设置为 0
type RequestConfig struct {
	Model string `json:"model"` // 模型名称，如 "claude-sonnet-4-20250514"

	MaxTokens int64 `json:"max_tokens"` // 最大生成 token 数量

	Temperature *float64 `json:"temperature,omitempty"` // 温度参数 (0-2)，控制输出的随机性

	TopP *float64 `json:"top_p,omitempty"` // Top-p 采样参数，控制输出的多样性

	StopSequences []string `json:"stop_sequences,omitempty"` // 停止序列列表，模型遇到其中任一序列时停止生成

	Tools []Tool `json:"tools,omitempty"` // 可用工具列表

	Metadata map[string]string `json:"metadata,omitempty"` // 附加元数据，用于传递请求级别的自定义信息

	UserID string `json:"user_id,omitempty"` // 用户标识，用于追踪和关联请求来源

	ToolChoice string `json:"tool_choice,omitempty"` // 工具选择策略，如 "auto"、"none"、"required"

	ResponseFormat string `json:"response_format,omitempty"` // 响应格式，如 "json_object"、"text"

	ParallelToolCalls bool `json:"parallel_tool_calls,omitempty"` // 是否允许并行工具调用

	ThinkingConfig *ThinkingConfig `json:"thinking_config,omitempty"` // 思考/推理配置

	ProviderExtra map[string]any `json:"provider_extra,omitempty"` // Provider 特有参数，用于传递各 Provider 独有的扩展配置
}

// PtrFloat64 返回 float64 指针，用于设置 RequestConfig 的可选字段。
//
// 用法: config.Temperature = PtrFloat64(0.7)
func PtrFloat64(v float64) *float64 {
	return &v
}

// PtrBool 返回 bool 指针，用于设置 RequestConfig 的可选字段。
//
// 用法: config.ThinkingConfig.Enabled = PtrBool(true)
func PtrBool(v bool) *bool {
	return &v
}

// PtrInt64 返回 int64 指针，用于设置 RequestConfig 的可选字段。
//
// 用法: config.ThinkingConfig.BudgetTokens = PtrInt64(10000)
func PtrInt64(v int64) *int64 {
	return &v
}
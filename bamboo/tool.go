package bamboo

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// Tool 工具定义，描述 AI 模型可调用的外部工具。
//
// InputSchema 使用 JSON Schema 格式定义工具参数结构，
// 以 json.RawMessage 形式原样保留，确保完整透传所有 JSON Schema 字段。
type Tool struct {
	Name         string                `json:"name"`                    // 工具名称，需在工具列表中唯一
	Description  string                `json:"description,omitempty"`   // 工具功能描述，帮助模型理解何时使用该工具
	InputSchema  json.RawMessage       `json:"input_schema"`            // 工具参数的 JSON Schema 定义（原始 JSON 透传）
	CacheControl *provider.CacheControl `json:"cache_control,omitempty"` // 缓存控制标记（Anthropic prompt caching）
}

package bamboo

// Tool 工具定义，描述 AI 模型可调用的外部工具。
//
// InputSchema 使用 JSON Schema 格式定义工具参数结构。
type Tool struct {
	Name        string      `json:"name"`                  // 工具名称，需在工具列表中唯一
	Description string      `json:"description,omitempty"` // 工具功能描述，帮助模型理解何时使用该工具
	InputSchema InputSchema `json:"input_schema"`          // 工具参数的 JSON Schema 定义
}

// InputSchema 工具参数的 JSON Schema 定义。
//
// Type 固定为 "object"，Properties 描述各参数字段，
// Required 列出必填参数名。
type InputSchema struct {
	Type       string                   `json:"type"`    // Schema 类型，固定为 "object"
	Properties map[string]PropertyDef   `json:"properties,omitempty"` // 参数字段定义映射
	Required   []string                 `json:"required,omitempty"`   // 必填参数名称列表
}

// PropertyDef 工具参数属性定义。
//
// 遵循 JSON Schema 规范，描述工具参数中单个属性的类型、
// 描述、枚举值和数组元素定义。
type PropertyDef struct {
	Type        string   `json:"type"`         // 属性类型，如 "string"、"number"、"boolean"、"array"、"object"
	Description string   `json:"description,omitempty"` // 属性描述
	Enum        []string `json:"enum,omitempty"`        // 枚举值列表（可选，仅当 Type 为 "string" 时有效）
	Items       any      `json:"items,omitempty"`       // 数组元素定义（可选，仅当 Type 为 "array" 时使用）
}

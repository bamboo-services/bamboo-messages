package bamboo

// Tool 工具定义，描述 AI 模型可调用的外部工具。
//
// InputSchema 使用 JSON Schema 格式定义工具参数结构。
type Tool struct {
	// Name 工具名称，需在工具列表中唯一
	Name string `json:"name"`

	// Description 工具功能描述，帮助模型理解何时使用该工具
	Description string `json:"description,omitempty"`

	// InputSchema 工具参数的 JSON Schema 定义
	InputSchema InputSchema `json:"input_schema"`
}

// InputSchema 工具参数的 JSON Schema 定义。
//
// Type 固定为 "object"，Properties 描述各参数字段，
// Required 列出必填参数名。
type InputSchema struct {
	// Type Schema 类型，固定为 "object"
	Type string `json:"type"`

	// Properties 参数字段定义映射
	Properties map[string]PropertyDef `json:"properties,omitempty"`

	// Required 必填参数名称列表
	Required []string `json:"required,omitempty"`
}

// PropertyDef 工具参数属性定义，遵循 JSON Schema 规范。
type PropertyDef struct {
	// Type 属性类型，如 "string"、"number"、"boolean"、"array"、"object"
	Type string `json:"type"`

	// Description 属性描述
	Description string `json:"description,omitempty"`

	// Enum 枚举值列表（可选，仅当 Type 为 "string" 时有效）
	Enum []string `json:"enum,omitempty"`

	// Items 数组元素定义（可选，仅当 Type 为 "array" 时使用）
	Items any `json:"items,omitempty"`
}

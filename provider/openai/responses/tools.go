package responses

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildTools 将内部工具定义转换为 OpenAI Responses API 工具参数格式。
//
// Responses API 的工具定义格式与 Completions API 不同：
// function 相关字段位于顶层，而非嵌套在 "function" 字段下。
//
// 格式示例:
//
//	{"type":"function","name":"get_weather","description":"...","parameters":{...}}
func buildTools(tools []provider.Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		fn := map[string]any{
			"type": "function",
			"name": tool.Function.Name,
		}
		if tool.Function.Description != "" {
			fn["description"] = tool.Function.Description
		}
		if tool.Function.Parameters != nil {
			fn["parameters"] = tool.Function.Parameters
		}
		result = append(result, fn)
	}
	return result
}

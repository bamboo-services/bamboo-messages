package gemini

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildTools 将内部工具定义转换为 Gemini REST API 工具参数格式。
//
// 将 provider.Tool 映射为 [{"functionDeclarations": [...]}] 的 map 格式，
// 仅转换 type="function" 的工具。Parameters 使用 json.RawMessage 直接透传 JSON Schema，
// 避免引入外部 Schema 类型。
func buildTools(tools []provider.Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		d := map[string]any{"name": tool.Function.Name}
		if tool.Function.Description != "" {
			d["description"] = tool.Function.Description
		}
		if tool.Function.Parameters != nil {
			// Parameters 为 map[string]any，序列化为 json.RawMessage 保留原始 JSON Schema
			if paramsBytes, err := json.Marshal(tool.Function.Parameters); err == nil {
				d["parameters"] = json.RawMessage(paramsBytes)
			}
		}
		decls = append(decls, d)
	}
	if len(decls) == 0 {
		return nil
	}
	return []map[string]any{{"functionDeclarations": decls}}
}

// jsonUnmarshal 是 encoding/json.Unmarshal 的薄包装，便于测试 mock。
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

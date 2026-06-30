package completions

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildTools 将内部工具定义转换为 OpenAI Completions 工具参数格式。
//
// 将 provider.Tool 映射为 {"type":"function","function":{...}} 的 map 格式，
// 仅支持 function 类型工具。
func buildTools(tools []provider.Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		fn := map[string]any{"name": tool.Function.Name}
		if tool.Function.Description != "" {
			fn["description"] = tool.Function.Description
		}
		if tool.Function.Parameters != nil {
			fn["parameters"] = tool.Function.Parameters
		}
		result = append(result, map[string]any{
			"type":     "function",
			"function": fn,
		})
	}
	return result
}

// buildResponseFormat 将泛型 ResponseFormat 参数转换为 OpenAI Completions 可接受的格式。
//
// 支持三种输入形式：
//   - map[string]any：从 map 中提取 "type" 字段，支持 "text"、"json_object"
//   - string：直接按字符串值映射
//   - 其他类型：返回 nil
//
// 输出为 map[string]any 或 nil，便于直接写入请求体。
func buildResponseFormat(rf any) any {
	switch v := rf.(type) {
	case string:
		return stringToResponseFormat(v)
	case map[string]any:
		if t, ok := v["type"]; ok {
			if tStr, ok := t.(string); ok {
				return stringToResponseFormat(tStr)
			}
		}
	}
	return nil
}

// stringToResponseFormat 将字符串类型的 ResponseFormat 转换为 map 格式。
//
// 支持 "text" 和 "json_object" 两种格式。
func stringToResponseFormat(formatType string) any {
	switch formatType {
	case "text":
		return map[string]any{"type": "text"}
	case "json_object":
		return map[string]any{"type": "json_object"}
	}
	return nil
}

// buildStop 将停止词列表转换为 OpenAI Completions Stop 参数格式。
//
// 空列表返回 nil，否则返回原始字符串数组。
func buildStop(stop []string) any {
	if len(stop) == 0 {
		return nil
	}
	return stop
}

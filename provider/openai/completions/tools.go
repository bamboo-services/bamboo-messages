package completions

import (
	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// buildTools 将内部工具定义转换为 OpenAI Completions SDK 工具参数格式。
//
// 将 provider.Tool 映射为 OpenAI SDK 的 FunctionTool，仅支持 function 类型。
func buildTools(tools []provider.Tool) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Type == "function" {
			result = append(result, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        tool.Function.Name,
				Description: openai.String(tool.Function.Description),
				Parameters:  tool.Function.Parameters,
			}))
		}
	}
	return result
}

// buildResponseFormat 将泛型 ResponseFormat 参数转换为 OpenAI Completions SDK 类型。
//
// 支持三种输入形式：
//   - map[string]any：从 map 中提取 "type" 字段，支持 "text"、"json_object" 值
//   - string：直接按字符串值映射
//   - SDK 原生类型：直接透传（向后兼容）
func buildResponseFormat(rf any) openai.ChatCompletionNewParamsResponseFormatUnion {
	switch v := rf.(type) {
	case openai.ChatCompletionNewParamsResponseFormatUnion:
		return v
	case string:
		return stringToResponseFormat(v)
	case map[string]any:
		if t, ok := v["type"]; ok {
			if tStr, ok := t.(string); ok {
				return stringToResponseFormat(tStr)
			}
		}
	}
	return openai.ChatCompletionNewParamsResponseFormatUnion{}
}

// stringToResponseFormat 将字符串类型的 ResponseFormat 转换为 SDK 联合类型。
//
// 支持 "text" 和 "json_object" 两种格式。
func stringToResponseFormat(formatType string) openai.ChatCompletionNewParamsResponseFormatUnion {
	switch formatType {
	case "text":
		return openai.ChatCompletionNewParamsResponseFormatUnion{
			OfText: openai.Ptr(shared.NewResponseFormatTextParam()),
		}
	case "json_object":
		return openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: openai.Ptr(shared.NewResponseFormatJSONObjectParam()),
		}
	}
	return openai.ChatCompletionNewParamsResponseFormatUnion{}
}

// buildStop 将停止词列表转换为 OpenAI Completions SDK Stop 参数格式。
//
// 将字符串数组包装为 ChatCompletionNewParamsStopUnion。
func buildStop(stop []string) openai.ChatCompletionNewParamsStopUnion {
	if len(stop) == 0 {
		return openai.ChatCompletionNewParamsStopUnion{}
	}
	return openai.ChatCompletionNewParamsStopUnion{
		OfStringArray: stop,
	}
}

package responses

import (
	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// buildTools 将内部工具定义转换为 OpenAI Responses SDK 工具参数格式。
//
// 将 provider.Tool 数组转换为 openai-go/v3 的 ToolUnionParam 列表，
// 目前仅支持 function 类型的工具。
func buildTools(tools []provider.Tool) []responses.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	result := make([]responses.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Type == "function" {
			function := responses.FunctionToolParam{
				Name:       tool.Function.Name,
				Parameters: tool.Function.Parameters,
			}
			if tool.Function.Description != "" {
				function.Description = param.NewOpt(tool.Function.Description)
			}
			result = append(result, responses.ToolUnionParam{
				OfFunction: &function,
			})
		}
	}
	return result
}

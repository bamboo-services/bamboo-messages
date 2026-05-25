package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildTools 将内部工具定义转换为 Anthropic SDK 工具参数格式
func buildTools(tools []provider.Tool) []anthropic.BetaToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	result := make([]anthropic.BetaToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Type == "function" {
			result = append(result, anthropic.BetaToolUnionParam{
				OfTool: &anthropic.BetaToolParam{
					Name:        tool.Function.Name,
					Description: anthropic.String(tool.Function.Description),
					InputSchema: anthropic.BetaToolInputSchemaParam{
						Type:       "object",
						Properties: tool.Function.Parameters,
					},
				},
			})
		}
	}
	return result
}

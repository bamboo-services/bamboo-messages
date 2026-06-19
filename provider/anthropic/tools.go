package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildTools 将内部工具定义转换为 Anthropic SDK 工具参数格式。
//
// 仅转换 type="function" 的工具，构建 BetaToolParam 包含 name、description、input_schema。
//
// BetaToolInputSchemaParam 的 MarshalJSON 使用 MarshalWithExtras：
// 先序列化 Type/Properties/Required 结构体字段，再通过 sjson 将 ExtraFields 合并进去（同名字段覆盖）。
// 因此将完整 JSON Schema 放入 ExtraFields 可确保所有字段（type/properties/required/
// additionalProperties/$defs 等）原样出现在最终的 input_schema 中，无丢失、无嵌套错位。
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
						ExtraFields: tool.Function.Parameters,
					},
				},
			})
		}
	}
	return result
}

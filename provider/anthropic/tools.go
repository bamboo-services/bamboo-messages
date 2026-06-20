package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

func buildTools(tools []provider.Tool) []anthropic.BetaToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	result := make([]anthropic.BetaToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Type == "function" {
			toolParam := &anthropic.BetaToolParam{
				Name:        tool.Function.Name,
				Description: anthropic.String(tool.Function.Description),
				InputSchema: anthropic.BetaToolInputSchemaParam{
					ExtraFields: tool.Function.Parameters,
				},
			}
			if tool.CacheControl != nil {
				toolParam.CacheControl = toAnthropicCacheControl(tool.CacheControl)
			}
			result = append(result, anthropic.BetaToolUnionParam{
				OfTool: toolParam,
			})
		}
	}
	return result
}

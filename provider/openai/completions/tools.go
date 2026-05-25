package completions

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildTools 将内部工具定义转换为 OpenAI Completions SDK 工具参数格式
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

// buildStop 将停止词列表转换为 OpenAI Completions SDK Stop 参数格式
func buildStop(stop []string) openai.ChatCompletionNewParamsStopUnion {
	if len(stop) == 0 {
		return openai.ChatCompletionNewParamsStopUnion{}
	}
	return openai.ChatCompletionNewParamsStopUnion{
		OfStringArray: stop,
	}
}

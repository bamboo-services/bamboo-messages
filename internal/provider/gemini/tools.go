package gemini

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/internal/provider"
	"google.golang.org/genai"
)

// buildTools 将内部工具定义转换为 Gemini SDK 工具参数格式。
//
// 仅转换 type="function" 的工具，合并到单个 genai.Tool 的 FunctionDeclarations 中。
// 使用 ParametersJsonSchema 字段直接传递 JSON Schema map，避免 Schema 结构体转换。
func buildTools(tools []provider.Tool) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}
	funcDecls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		funcDecls = append(funcDecls, &genai.FunctionDeclaration{
			Name:                 tool.Function.Name,
			Description:          tool.Function.Description,
			ParametersJsonSchema: tool.Function.Parameters,
		})
	}
	if len(funcDecls) == 0 {
		return nil
	}
	return []*genai.Tool{{
		FunctionDeclarations: funcDecls,
	}}
}

// jsonUnmarshal 是 encoding/json.Unmarshal 的薄包装，便于测试 mock。
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

package responses

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// buildInput 将内部消息格式转换为 OpenAI Responses API 输入格式
func (p *ResponsesProvider) buildInput(systemPrompt string, messages []provider.Message) responses.ResponseNewParamsInputUnion {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(messages)+1)

	if systemPrompt != "" {
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleSystem,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(systemPrompt),
				},
			},
		})
	}

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			items = append(items, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(msg.Content),
					},
				},
			})
		case provider.RoleAssistant:
			items = append(items, p.buildAssistantItem(msg))
		case provider.RoleTool:
			items = append(items, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: msg.ToolCallID,
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: openai.String(msg.Content),
					},
				},
			})
		}
	}

	return responses.ResponseNewParamsInputUnion{
		OfInputItemList: items,
	}
}

// buildAssistantItem 构建助手消息项（支持文本和工具调用）
func (p *ResponsesProvider) buildAssistantItem(msg provider.Message) responses.ResponseInputItemUnionParam {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(msg.ToolCalls)+1)

	if msg.Content != "" {
		items = append(items, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleAssistant,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(msg.Content),
				},
			},
		})
	}

	for _, tc := range msg.ToolCalls {
		items = append(items, responses.ResponseInputItemUnionParam{
			OfFunctionCall: &responses.ResponseFunctionToolCallParam{
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	if len(items) == 1 {
		return items[0]
	}
	return items[0]
}

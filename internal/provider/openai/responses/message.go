package responses

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// ==============================
// 内部方法
// ==============================

// buildInput 将内部消息格式转换为 OpenAI Responses API 输入格式。
//
// 将 provider.Message 数组转换为 OpenAI SDK 的 ResponseInputItemUnionParam 列表，
// 支持 system、user、assistant、tool 四种角色的消息。
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
			items = append(items, p.buildAssistantItem(msg)...)
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

// buildAssistantItem 构建助手消息项（支持文本和工具调用）。
//
// 将助手消息（可能包含文本内容和工具调用）转换为 OpenAI SDK 输入项列表，
// 一个助手消息可能拆分为多个输入项。
func (p *ResponsesProvider) buildAssistantItem(msg provider.Message) []responses.ResponseInputItemUnionParam {
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

	if len(items) == 0 {
		return items
	}

	return items
}

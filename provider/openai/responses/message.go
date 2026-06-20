package responses

import (
	"log"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
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
			if len(msg.ContentBlocks) > 0 {
				parts := make(responses.ResponseInputMessageContentListParam, 0, len(msg.ContentBlocks)+1)

				if msg.Content != "" {
					parts = append(parts, responses.ResponseInputContentUnionParam{
						OfInputText: &responses.ResponseInputTextParam{
							Text: msg.Content,
						},
					})
				}

				for _, cb := range msg.ContentBlocks {
					switch cb.BlockType() {
					case "image":
						if img, ok := cb.(provider.ImageContentBlock); ok {
							if img.Source.Type == "base64" {
								// base64 编码图片 → data URI 格式
								dataURI := "data:" + img.Source.MediaType + ";base64," + img.Source.Data
								parts = append(parts, responses.ResponseInputContentUnionParam{
									OfInputImage: &responses.ResponseInputImageParam{
										ImageURL: param.Opt[string]{Value: dataURI},
									},
								})
							} else if img.Source.Type == "url" {
								// URL 图片直接传递
								parts = append(parts, responses.ResponseInputContentUnionParam{
									OfInputImage: &responses.ResponseInputImageParam{
										ImageURL: param.Opt[string]{Value: img.Source.URL},
									},
								})
							}
						}
					case "document":
						// 文档内容块：text 类型转为文本部分，其余记录警告后忽略
						if doc, ok := cb.(provider.DocumentContentBlock); ok {
							if doc.Source.Type == "url" || doc.Source.Type == "base64" {
								log.Printf("[provider/openai-responses] DocumentBlock(source=%q) 不支持，已忽略", doc.Source.Type)
							} else {
								log.Printf("[provider/openai-responses] DocumentBlock 未知来源类型=%q，已忽略", doc.Source.Type)
							}
						}
					}
				}

				items = append(items, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfInputItemContentList: parts,
						},
					},
				})
			} else {
				// 纯文本消息：向后兼容
				items = append(items, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: openai.String(msg.Content),
						},
					},
				})
			}
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

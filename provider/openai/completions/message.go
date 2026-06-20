package completions

import (
	"log"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
)

// ==============================
// 内部方法
// ==============================

// buildMessages 将内部消息格式转换为 OpenAI Chat Completions API 消息格式。
//
// 将 provider.Message 映射为 OpenAI SDK 的 System/User/Assistant/Tool 消息。
func (p *CompletionsProvider) buildMessages(systemPrompt string, messages []provider.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)

	if systemPrompt != "" {
		result = append(result, openai.SystemMessage(systemPrompt))
	}

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			if len(msg.ContentBlocks) > 0 {
				parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.ContentBlocks)+1)
				if msg.Content != "" {
					parts = append(parts, openai.TextContentPart(msg.Content))
				}
				for _, cb := range msg.ContentBlocks {
					switch cb.BlockType() {
					case "image":
						if img, ok := cb.(provider.ImageContentBlock); ok {
							if img.Source.Type == "base64" {
								dataURI := "data:" + img.Source.MediaType + ";base64," + img.Source.Data
								parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
									URL:    dataURI,
									Detail: "auto",
								}))
							} else if img.Source.Type == "url" {
								parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
									URL:    img.Source.URL,
									Detail: "auto",
								}))
							}
						}
					case "document":
						// 文档内容块：text 类型转为文本部分，其余记录警告后忽略
						if doc, ok := cb.(provider.DocumentContentBlock); ok {
							if doc.Source.Type == "url" || doc.Source.Type == "base64" {
								log.Printf("[provider/openai-completions] DocumentBlock(source=%q) 不支持，已忽略", doc.Source.Type)
							} else {
								log.Printf("[provider/openai-completions] DocumentBlock 未知来源类型=%q，已忽略", doc.Source.Type)
							}
						}
					}
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfArrayOfContentParts: parts,
						},
					},
				})
			} else {
				result = append(result, openai.UserMessage(msg.Content))
			}
		case provider.RoleAssistant:
			result = append(result, p.buildAssistantMessage(msg))
		case provider.RoleTool:
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	return result
}

// buildAssistantMessage 构建助手消息（支持文本和工具调用）。
//
// 将 provider.Message 映射为 OpenAI SDK 的 Assistant 消息，包含 Content 和 ToolCalls。
func (p *CompletionsProvider) buildAssistantMessage(msg provider.Message) openai.ChatCompletionMessageParamUnion {
	assistantMsg := openai.ChatCompletionAssistantMessageParam{}

	if msg.Content != "" {
		assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: param.NewOpt(msg.Content),
		}
	}

	// 仅在存在实际工具调用时填充 ToolCalls，避免序列化出空数组 "tool_calls": []。
	// 部分第三方 OpenAI 兼容端点（如 Kimi coding API）会将空数组视为无效请求，
	// 导致返回 choices=0 的空响应。
	if len(msg.ToolCalls) > 0 {
		assistantMsg.ToolCalls = make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				},
			})
		}
	}

	return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg}
}

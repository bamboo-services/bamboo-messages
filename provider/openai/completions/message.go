package completions

import (
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
						// 静默忽略 — OpenAI Completions 不支持文档块
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
	assistantMsg := openai.ChatCompletionAssistantMessageParam{
		ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{},
	}

	if msg.Content != "" {
		assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: param.NewOpt(msg.Content),
		}
	}

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

	return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg}
}

package completions

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// buildMessages 将内部消息格式转换为 OpenAI Chat Completions API 消息格式
func (p *CompletionsProvider) buildMessages(systemPrompt string, messages []provider.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)

	if systemPrompt != "" {
		result = append(result, openai.SystemMessage(systemPrompt))
	}

	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			result = append(result, openai.UserMessage(msg.Content))
		case provider.RoleAssistant:
			result = append(result, p.buildAssistantMessage(msg))
		case provider.RoleTool:
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}

	return result
}

// buildAssistantMessage 构建助手消息（支持文本和工具调用）
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

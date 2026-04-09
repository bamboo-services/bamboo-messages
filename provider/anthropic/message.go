package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// buildMessages 将内部消息格式转换为 Anthropic SDK 消息格式
func (p *Provider) buildMessages(messages []provider.Message) []anthropic.BetaMessageParam {
	result := make([]anthropic.BetaMessageParam, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			result = append(result, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(msg.Content)))
		case provider.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(msg.ToolCalls)+1)
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewBetaTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					blocks = append(blocks, anthropic.NewBetaToolUseBlock(tc.ID, tc.Function.Arguments, tc.Function.Name))
				}
				result = append(result, anthropic.BetaMessageParam{
					Role:    anthropic.BetaMessageParamRoleAssistant,
					Content: blocks,
				})
			} else {
				result = append(result, anthropic.BetaMessageParam{
					Role:    anthropic.BetaMessageParamRoleAssistant,
					Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(msg.Content)},
				})
			}
		case provider.RoleTool:
			result = append(result, anthropic.NewBetaUserMessage(
				anthropic.NewBetaToolResultBlock(msg.ToolCallID, msg.Content, false),
			))
		}
	}
	return result
}

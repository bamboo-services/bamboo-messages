package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// ==============================
// 内部方法
// ==============================

// buildMessages 将内部消息格式转换为 Anthropic SDK 消息格式。
//
// 根据 Role 构建对应的 Anthropic BetaMessageParam：
// - RoleUser: NewBetaUserMessage(BetaTextBlock)
// - RoleAssistant: 支持普通文本和工具调用，工具调用时需包含 text 和 tool_use blocks
// - RoleTool: NewBetaUserMessage(BetaToolResultBlock)
func (p *Provider) buildMessages(messages []provider.Message) []anthropic.BetaMessageParam {
	result := make([]anthropic.BetaMessageParam, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			if len(msg.ContentBlocks) > 0 {
				blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(msg.ContentBlocks)+1)
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewBetaTextBlock(msg.Content))
				}
				for _, cb := range msg.ContentBlocks {
					switch cb.BlockType() {
					case "image":
						if img, ok := cb.(provider.ImageContentBlock); ok {
							if img.Source.Type == "base64" {
								blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
									OfImage: &anthropic.BetaImageBlockParam{
										Source: anthropic.BetaImageBlockParamSourceUnion{
											OfBase64: &anthropic.BetaBase64ImageSourceParam{
												Data:      img.Source.Data,
												MediaType: anthropic.BetaBase64ImageSourceMediaType(img.Source.MediaType),
											},
										},
									},
								})
							} else if img.Source.Type == "url" {
								blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
									OfImage: &anthropic.BetaImageBlockParam{
										Source: anthropic.BetaImageBlockParamSourceUnion{
											OfURL: &anthropic.BetaURLImageSourceParam{
												URL: img.Source.URL,
											},
										},
									},
								})
							}
						}
					case "document":
						if doc, ok := cb.(provider.DocumentContentBlock); ok {
							if doc.Source.Type == "base64" {
								blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
									OfDocument: &anthropic.BetaRequestDocumentBlockParam{
										Source: anthropic.BetaRequestDocumentBlockSourceUnionParam{
											OfBase64: &anthropic.BetaBase64PDFSourceParam{
												Data: doc.Source.Data,
											},
										},
									},
								})
							} else if doc.Source.Type == "url" {
								blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
									OfDocument: &anthropic.BetaRequestDocumentBlockParam{
										Source: anthropic.BetaRequestDocumentBlockSourceUnionParam{
											OfURL: &anthropic.BetaURLPDFSourceParam{
												URL: doc.Source.URL,
											},
										},
									},
								})
							}
						}
					}
				}
				applyMsgCacheControl(blocks, msg.CacheControl)
				result = append(result, anthropic.BetaMessageParam{
					Role:    anthropic.BetaMessageParamRoleUser,
					Content: blocks,
				})
			} else {
				block := anthropic.NewBetaTextBlock(msg.Content)
				if msg.CacheControl != nil && block.OfText != nil {
					block.OfText.CacheControl = toAnthropicCacheControl(msg.CacheControl)
				}
				result = append(result, anthropic.NewBetaUserMessage(block))
			}
		case provider.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(msg.ToolCalls)+1)
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewBetaTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					blocks = append(blocks, anthropic.NewBetaToolUseBlock(tc.ID, tc.Function.Arguments, tc.Function.Name))
				}
				applyMsgCacheControl(blocks, msg.CacheControl)
				result = append(result, anthropic.BetaMessageParam{
					Role:    anthropic.BetaMessageParamRoleAssistant,
					Content: blocks,
				})
			} else {
				block := anthropic.NewBetaTextBlock(msg.Content)
				if msg.CacheControl != nil && block.OfText != nil {
					block.OfText.CacheControl = toAnthropicCacheControl(msg.CacheControl)
				}
				result = append(result, anthropic.BetaMessageParam{
					Role:    anthropic.BetaMessageParamRoleAssistant,
					Content: []anthropic.BetaContentBlockParamUnion{block},
				})
			}
		case provider.RoleTool:
			block := anthropic.NewBetaToolResultBlock(msg.ToolCallID, msg.Content, msg.IsError)
			if msg.CacheControl != nil && block.OfToolResult != nil {
				block.OfToolResult.CacheControl = toAnthropicCacheControl(msg.CacheControl)
			}
			result = append(result, anthropic.NewBetaUserMessage(block))
		}
	}
	return result
}

// applyMsgCacheControl 将消息级别的 CacheControl 标记应用到最后一个 content block。
//
// Anthropic 的 cache_control 是块级断点，provider.Message.CacheControl 表示
// "这条消息的最后一个块需要缓存"，因此将标记设置到最后一个 block 上。
func applyMsgCacheControl(blocks []anthropic.BetaContentBlockParamUnion, cc *provider.CacheControl) {
	if cc == nil || len(blocks) == 0 {
		return
	}
	last := &blocks[len(blocks)-1]
	anthropicCC := toAnthropicCacheControl(cc)
	if last.OfText != nil {
		last.OfText.CacheControl = anthropicCC
	} else if last.OfToolUse != nil {
		last.OfToolUse.CacheControl = anthropicCC
	} else if last.OfImage != nil {
		last.OfImage.CacheControl = anthropicCC
	} else if last.OfDocument != nil {
		last.OfDocument.CacheControl = anthropicCC
	} else if last.OfToolResult != nil {
		last.OfToolResult.CacheControl = anthropicCC
	}
}

// toAnthropicCacheControl 将 provider.CacheControl 转换为 Anthropic SDK 的 BetaCacheControlEphemeralParam。
func toAnthropicCacheControl(cc *provider.CacheControl) anthropic.BetaCacheControlEphemeralParam {
	param := anthropic.NewBetaCacheControlEphemeralParam()
	if cc != nil && cc.TTL == provider.CacheTTL1h {
		param.TTL = anthropic.BetaCacheControlEphemeralTTLTTL1h
	}
	return param
}

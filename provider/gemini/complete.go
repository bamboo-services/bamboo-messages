package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
	"google.golang.org/genai"
)

// Complete 非流式对话。
//
// 无系统提示的非流式对话，内部调用 CompleteWithSystem 并传入空 systemPrompt。
// 同步返回完整响应和可能的错误。
func (p *Provider) Complete(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	return p.CompleteWithSystem(ctx, "", messages, config)
}

// CompleteWithSystem 带系统提示的非流式对话。
//
// 将统一 provider.Message 转换为 Gemini 协议格式，
// 通过底层 SDK 发起同步请求（GenerateContent），返回 CompletionResult。
// 支持系统提示、温度、TopP、MaxTokens、Stop 序列、工具调用、Thinking 配置、ToolChoice 等参数。
func (p *Provider) CompleteWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) (*provider.CompletionResult, error) {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	contents := p.buildMessages(messages)
	gc := p.buildContentConfig(systemPrompt, config)

	resp, err := p.Client.Models.GenerateContent(ctx, config.Model, contents, gc)
	if err != nil {
		return nil, xError.NewError(ctx, nil, "Gemini 非流式对话失败", false, err)
	}

	result := &provider.CompletionResult{
		FinishReason: provider.FinishReasonStop,
	}

	// 提取 Token 用量
	if resp.UsageMetadata != nil {
		result.Usage = provider.UsageData{
			InputTokens:  int64(resp.UsageMetadata.PromptTokenCount),
			OutputTokens: int64(resp.UsageMetadata.CandidatesTokenCount),
		}
	}

	// 遍历响应内容
	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		result.FinishReason = mapFinishReason(candidate.FinishReason)

		if candidate.Content != nil {
			for i, part := range candidate.Content.Parts {
				// 文本内容（忽略 Thought==true 的推理内容）
				if !part.Thought && part.Text != "" {
					result.Content += part.Text
				}
				// 工具调用
				if part.FunctionCall != nil {
					id := part.FunctionCall.ID
					if id == "" {
						id = fmt.Sprintf("gemini_call_%s_%d", part.FunctionCall.Name, i)
					}
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
						ID:   id,
						Type: "function",
						Function: provider.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsBytes),
						},
					})
					// 如果有工具调用且 FinishReason 未明确指定，设为 ToolCalls
					if result.FinishReason == provider.FinishReasonStop {
						result.FinishReason = provider.FinishReasonToolCalls
					}
				}
			}
		}
	}

	return result, nil
}

// ==============================
// 内部方法（从 complete.go 共享）
// ============================================

// buildContentConfig 构建 Gemini GenerateContentConfig。
//
// 将 provider.ChatConfig 的统一字段映射到 Gemini 配置：
//   - Temperature / TopP → *float32
//   - MaxTokens → MaxOutputTokens (int32)
//   - Stop → StopSequences
//   - Tools → []*genai.Tool
//   - ThinkingConfig.Effort → genai.ThinkingConfig + ThinkingLevel
//   - ToolChoice → ToolConfig.FunctionCallingConfig.Mode
//   - TopK / SafetySettings → 从 ProviderExtra 提取
func (p *Provider) buildContentConfig(systemPrompt string, config *provider.ChatConfig) *genai.GenerateContentConfig {
	gc := &genai.GenerateContentConfig{}

	// 设置系统提示
	if systemPrompt != "" {
		gc.SystemInstruction = genai.NewContentFromText(systemPrompt, "")
	}

	// 设置可选参数（检查 nil 避免空指针解引用）
	if config.Temperature != nil {
		temp := float32(*config.Temperature)
		gc.Temperature = &temp
	}
	if config.TopP != nil {
		topP := float32(*config.TopP)
		gc.TopP = &topP
	}

	// MaxTokens
	if config.MaxTokens > 0 {
		gc.MaxOutputTokens = int32(config.MaxTokens)
	}

	// Stop sequences
	if len(config.Stop) > 0 {
		gc.StopSequences = config.Stop
	}

	// Tools
	if tools := buildTools(config.Tools); tools != nil {
		gc.Tools = tools
	}

	// ThinkingConfig 映射: none→不设置, low/medium/high→对应 ThinkingLevel
	if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
		gc.ThinkingConfig = mapThinkingConfig(config.ThinkingConfig.Effort)
	}

	// TopK（从 ProviderExtra 提取）
	if topK, ok := provider.GetExtraFloat64(config.ProviderExtra, "top_k"); ok {
		topK32 := float32(topK)
		gc.TopK = &topK32
	}

	// SafetySettings（从 ProviderExtra 提取）
	if settings, ok := provider.GetExtraAny(config.ProviderExtra, "safety_settings"); ok {
		if safetySettings, ok := settings.([]*genai.SafetySetting); ok {
			gc.SafetySettings = safetySettings
		}
	}

	// ToolChoice 映射
	if config.ToolChoice != "" {
		gc.ToolConfig = &genai.ToolConfig{
			FunctionCallingConfig: mapToolChoice(config.ToolChoice),
		}
	}

	// ResponseFormat 映射
	if config.ResponseFormat == "json_object" {
		gc.ResponseMIMEType = "application/json"
	}

	// Labels（元数据）
	if len(config.Metadata) > 0 {
		gc.Labels = config.Metadata
	}

	return gc
}

// mapThinkingConfig 将统一的 ThinkingConfig.Effort 映射为 genai.ThinkingConfig。
//
//   - "none" → nil（不启用思考）
//   - "low" → IncludeThoughts + ThinkingLevelLow
//   - "medium" → IncludeThoughts + ThinkingLevelMedium
//   - "high" → IncludeThoughts + ThinkingLevelHigh
func mapThinkingConfig(effort string) *genai.ThinkingConfig {
	switch effort {
	case "low":
		return &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   genai.ThinkingLevelLow,
		}
	case "medium":
		return &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   genai.ThinkingLevelMedium,
		}
	case "high":
		return &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   genai.ThinkingLevelHigh,
		}
	default:
		return nil
	}
}

// mapToolChoice 将统一的 ToolChoice 字符串映射为 genai.FunctionCallingConfig。
//
//   - "auto" → ModeAuto
//   - "none" → ModeNone
//   - "required" / "forced" → ModeAny
func mapToolChoice(choice string) *genai.FunctionCallingConfig {
	switch choice {
	case "auto":
		return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto}
	case "none":
		return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeNone}
	case "required", "forced", "any":
		return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny}
	default:
		return &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto}
	}
}

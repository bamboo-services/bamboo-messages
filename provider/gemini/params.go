package gemini

import (
	"github.com/bamboo-services/bamboo-messages/provider"
	"google.golang.org/genai"
)

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

	// CachedContent — Gemini 外部缓存资源引用（从 ProviderExtra 提取）
	if cc, ok := provider.GetExtraString(config.ProviderExtra, "cached_content"); ok && cc != "" {
		gc.CachedContent = cc
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

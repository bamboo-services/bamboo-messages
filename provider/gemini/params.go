package gemini

import (
	"context"
	"fmt"
	"math"
	"strings"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildRequestBody 构建 Gemini generateContent / streamGenerateContent 完整请求体。
//
// 将消息历史、系统提示、生成配置、工具声明与工具调用策略组装为 map[string]any，
// 直接对应 Gemini REST API 的 JSON 结构。stream 参数用于调用方区分端点
// （streamGenerateContent?alt=sse / generateContent），不影响 body 本身。
func (p *Provider) buildRequestBody(messages []provider.Message, systemPrompt string, config *provider.ChatConfig, stream bool) map[string]any {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	body := map[string]any{"contents": p.buildMessages(messages)}

	// 系统提示 — Gemini 使用顶层 systemInstruction 字段
	if systemPrompt != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": systemPrompt}},
		}
	}

	// 生成配置（仅 GenerationConfig 白名单字段）
	if gc := p.buildContentConfig(config); len(gc) > 0 {
		body["generationConfig"] = gc
	}

	// 工具声明
	if tools := buildTools(config.Tools); tools != nil {
		body["tools"] = tools
	}

	// 工具调用策略
	if tc := buildToolConfig(config.ToolChoice); tc != nil {
		body["toolConfig"] = tc
	}

	// safetySettings / cachedContent 是 GenerateContentRequest 顶层字段，
	// 不能放进 generationConfig，否则 protobuf 校验会报 Unknown name。
	if settings, ok := provider.GetExtraAny(config.ProviderExtra, "safety_settings"); ok {
		body["safetySettings"] = settings
	}
	if cc, ok := provider.GetExtraString(config.ProviderExtra, "cached_content"); ok && cc != "" {
		body["cachedContent"] = cc
	}

	return body
}

// buildContentConfig 构建 Gemini generationConfig。
//
// 将 provider.ChatConfig 的统一字段映射到 Gemini REST API 的 generationConfig：
//   - Temperature / TopP → 顶层浮点字段
//   - TopK → 从 ProviderExtra 提取（Gemini 特有参数）
//   - MaxTokens → maxOutputTokens（int32 溢出保护）
//   - Stop → stopSequences
//   - ThinkingConfig.Effort → thinkingConfig {includeThoughts, thinkingLevel}
//   - ResponseFormat → responseMimeType
//
// safetySettings / cachedContent 属于请求顶层字段，由 buildRequestBody 提取。
// UserID / Metadata 不映射：Gemini Developer API 的 GenerationConfig 无 labels，
// 写入会触发 protobuf "Unknown name labels at request.generation_config"。
func (p *Provider) buildContentConfig(config *provider.ChatConfig) map[string]any {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	gc := map[string]any{}

	// 温度
	if config.Temperature != nil {
		gc["temperature"] = *config.Temperature
	}

	// Top-P
	if config.TopP != nil {
		gc["topP"] = *config.TopP
	}

	// TopK — Gemini 特有参数，从 ProviderExtra 提取
	if topK, ok := provider.GetExtraFloat64(config.ProviderExtra, "top_k"); ok && topK > 0 {
		gc["topK"] = topK
	}

	// MaxOutputTokens — int64 → int32 溢出保护
	if config.MaxTokens > 0 {
		if config.MaxTokens > math.MaxInt32 {
			gc["maxOutputTokens"] = math.MaxInt32
		} else {
			gc["maxOutputTokens"] = int(config.MaxTokens)
		}
	}

	// 停止序列
	if len(config.Stop) > 0 {
		gc["stopSequences"] = config.Stop
	}

	if thinking := buildGeminiThinkingConfig(config); thinking != nil {
		gc["thinkingConfig"] = thinking
	}

	// 响应格式 — json_object → application/json
	if config.ResponseFormat == "json_object" {
		gc["responseMimeType"] = "application/json"
	}

	// UserID / Metadata — Gemini Developer API 无对应字段，忽略以免写入非法 labels
	if config.UserID != "" && provider.DebugEnabled {
		xLog.WithName("provider/gemini").SugarWarn(context.Background(),
			fmt.Sprintf("UserID=%q 已被忽略（Gemini GenerationConfig 无 labels 字段）", config.UserID))
	}
	if len(config.Metadata) > 0 && provider.DebugEnabled {
		xLog.WithName("provider/gemini").SugarWarn(context.Background(),
			"Metadata 已被忽略（Gemini GenerationConfig 无 labels 字段）")
	}

	// ParallelToolCalls — Gemini 不支持此参数，仅记录 debug 日志
	if config.ParallelToolCalls && provider.DebugEnabled {
		xLog.WithName("provider/gemini").SugarWarn(context.Background(),
			"ParallelToolCalls=true 不被 Gemini 协议支持，已忽略")
	}

	return gc
}

func buildGeminiThinkingConfig(config *provider.ChatConfig) map[string]any {
	if config == nil {
		return nil
	}
	effort := ""
	if config.ThinkingConfig != nil {
		effort = config.ThinkingConfig.Effort
	}
	if effort == "none" {
		return map[string]any{"thinkingBudget": 0}
	}
	includeThoughts := effort != ""
	if v, ok := provider.GetExtraBool(config.ProviderExtra, "include_thoughts"); ok {
		includeThoughts = v
	}
	if !includeThoughts {
		return nil
	}
	tc := map[string]any{"includeThoughts": true}
	if effort == "" {
		return tc
	}
	effort = provider.NormalizeReasoningEffort(effort)
	model := strings.ToLower(config.Model)
	switch {
	case strings.Contains(model, "gemini-3"):
		tc["thinkingLevel"] = effort
	case strings.Contains(model, "2.5") || strings.Contains(model, "gemini-2"):
		tc["thinkingBudget"] = effortToGeminiBudget(effort)
	default:
		tc["thinkingLevel"] = effort
		tc["thinkingBudget"] = effortToGeminiBudget(effort)
	}
	return tc
}

func effortToGeminiBudget(effort string) int {
	switch effort {
	case "minimal", "low":
		return 1024
	case "medium":
		return 8192
	case "high":
		return 16384
	case "xhigh":
		return -1
	default:
		return 8192
	}
}

// buildToolConfig 将统一的 ToolChoice 字符串映射为 Gemini toolConfig。
//
// Gemini FunctionCallingConfig.Mode 取值：AUTO / NONE / ANY。
//   - "auto"     → AUTO
//   - "none"     → NONE
//   - "required" / "forced" / "any" → ANY
//   - 空         → nil（不设置 toolConfig）
func buildToolConfig(toolChoice string) map[string]any {
	switch toolChoice {
	case "auto":
		return map[string]any{
			"functionCallingConfig": map[string]any{"mode": "AUTO"},
		}
	case "none":
		return map[string]any{
			"functionCallingConfig": map[string]any{"mode": "NONE"},
		}
	case "required", "forced", "any":
		return map[string]any{
			"functionCallingConfig": map[string]any{"mode": "ANY"},
		}
	default:
		return nil
	}
}

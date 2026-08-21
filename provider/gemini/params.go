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

	// 生成配置
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
//   - SafetySettings → 从 ProviderExtra 提取
//   - Labels → UserID 映射到 labels["user_id"] + Metadata 合并
//   - CachedContent → 从 ProviderExtra 提取（Gemini 外部缓存引用）
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

	// SafetySettings — 从 ProviderExtra 提取（Gemini 特有参数）
	if settings, ok := provider.GetExtraAny(config.ProviderExtra, "safety_settings"); ok {
		gc["safetySettings"] = settings
	}

	// Labels — UserID 映射到 labels["user_id"]，合并 Metadata
	labels := map[string]string{}
	if config.UserID != "" {
		labels["user_id"] = config.UserID
		if provider.DebugEnabled {
			xLog.WithName("provider/gemini").SugarWarn(context.Background(),
				fmt.Sprintf("UserID=%q 已映射到 Labels[user_id]（Gemini 无原生 UserID 支持）", config.UserID))
		}
	}
	for k, v := range config.Metadata {
		labels[k] = v
	}
	if len(labels) > 0 {
		gc["labels"] = labels
	}

	// ParallelToolCalls — Gemini 不支持此参数，仅记录 debug 日志
	if config.ParallelToolCalls && provider.DebugEnabled {
		xLog.WithName("provider/gemini").SugarWarn(context.Background(),
			"ParallelToolCalls=true 不被 Gemini 协议支持，已忽略")
	}

	// CachedContent — Gemini 外部缓存资源引用（从 ProviderExtra 提取）
	if cc, ok := provider.GetExtraString(config.ProviderExtra, "cached_content"); ok && cc != "" {
		gc["cachedContent"] = cc
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

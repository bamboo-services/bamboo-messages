package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/provider"
)

const (
	paramTopK                   = "top_k"
	paramCacheNormalize         = "anthropic_cache_normalize"
	paramCacheNormalizeMetadata = "anthropic_cache_normalize_metadata"
)

// buildParams 构建 Anthropic Messages 请求参数。
//
// 将统一的 provider.ChatConfig 转换为 Anthropic Messages API 的请求体结构，
// 包含所有参数构建逻辑（Model/Messages/MaxTokens/Temperature/TopP/Stop/Tools/
// ThinkingConfig/TopK/ToolChoice/Metadata 等）。
//
// 此方法由 ChatWithSystem 和 CompleteWithSystem 共享调用，
// 确保流式和非流式路径使用一致的参数构建逻辑。
func (p *Provider) buildParams(systemPrompt string, messages []provider.Message, config *provider.ChatConfig) messageCreateRequest {
	if config == nil {
		config = &provider.ChatConfig{}
	}

	// ResponseFormat best-effort 适配：Anthropic 不原生支持 ResponseFormat，
	// 当设置为 "json_object" 时注入系统提示指令作为替代方案
	if config.ResponseFormat == "json_object" {
		instruction := "Respond with valid JSON only."
		if systemPrompt != "" {
			systemPrompt = systemPrompt + "\n\n" + instruction
		} else {
			systemPrompt = instruction
		}
		if provider.DebugEnabled {
			xLog.WithName("provider/anthropic").SugarWarn(context.Background(),
				fmt.Sprintf("ResponseFormat=%q 不被原生支持，已注入系统提示指令作为替代", config.ResponseFormat))
		}
	}

	// ParallelToolCalls — Anthropic 不支持此参数，仅记录 debug 日志
	if config.ParallelToolCalls && provider.DebugEnabled {
		xLog.WithName("provider/anthropic").SugarWarn(context.Background(),
			"ParallelToolCalls=true 不被 Anthropic 协议支持，已忽略")
	}

	params := messageCreateRequest{
		Model:     config.Model,
		MaxTokens: int(config.MaxTokens),
		Messages:  p.buildMessages(messages),
	}

	// 设置系统提示
	if systemPrompt != "" {
		if config.SystemCacheControl != nil {
			// 带 cache_control 的 system 块数组格式
			sysBlock := map[string]any{
				"type":          "text",
				"text":          systemPrompt,
				"cache_control": buildCacheControl(config.SystemCacheControl),
			}
			params.System = []map[string]any{sysBlock}
		} else {
			// 纯字符串格式
			params.System = systemPrompt
		}
	}

	// 可选参数（检查 nil 避免空指针解引用）
	if config.Temperature != nil {
		params.Temperature = config.Temperature
	}
	if config.TopP != nil {
		params.TopP = config.TopP
	}

	if len(config.Stop) > 0 {
		params.StopSequences = config.Stop
	}

	if tools := buildTools(config.Tools); tools != nil {
		params.Tools = tools
	}

	// ThinkingConfig 映射
	//   effort == "none" → 不设置 thinking 字段
	//   非 Legacy 模式（Anthropic 官方端点）:
	//     - 优先使用 ProviderExtra["thinking"] 原始配置（跨协议场景保留 type 和 budget_tokens）
	//     - 回退合成 {type:"adaptive"}
	//   Legacy 模式（GLM/Kimi 等第三方兼容端点）:
	//     - 优先使用 ProviderExtra["thinking"] 并归一化（adaptive→enabled，保留 budget_tokens）
	//     - 回退合成 {type:"enabled"}
	if config.ThinkingConfig != nil && config.ThinkingConfig.Effort != "" {
		if config.ThinkingConfig.Effort != "none" {
			if tc := parseThinkingFromExtra(config.ProviderExtra); tc != nil {
				if p.legacyCompat && tc.Type == "adaptive" {
					tc.Type = "enabled"
				}
				params.Thinking = tc
			} else {
				if p.legacyCompat {
					params.Thinking = &thinkingConfig{Type: "enabled"}
				} else {
					params.Thinking = &thinkingConfig{Type: "adaptive"}
				}
			}
		}
	}

	// TopK 透传（优先 float64，回退 int64）
	if topK, ok := provider.GetExtraFloat64(config.ProviderExtra, paramTopK); ok {
		k := int(topK)
		params.TopK = &k
	} else if topK, ok := provider.GetExtraInt64(config.ProviderExtra, paramTopK); ok {
		k := int(topK)
		params.TopK = &k
	}

	// ToolChoice 映射: auto→{"type":"auto"}, any/required/forced→{"type":"any"}, none→{"type":"none"}
	if config.ToolChoice != "" {
		switch config.ToolChoice {
		case "auto":
			params.ToolChoice = map[string]any{"type": "auto"}
		case "any", "required", "forced":
			params.ToolChoice = map[string]any{"type": "any"}
		case "none":
			params.ToolChoice = map[string]any{"type": "none"}
		}
	}

	// Metadata（UserID + 自定义元数据）
	// 当启用了 cache normalize 时不发送 metadata，避免与缓存断点冲突
	cacheNormalize := anthropicCacheNormalizationEnabled(config)
	if (config.UserID != "" || len(config.Metadata) > 0) && !cacheNormalize {
		meta := &metadata{}
		if config.UserID != "" {
			meta.UserID = config.UserID
		}
		params.Metadata = meta
	}

	return params
}

// anthropicCacheNormalizationEnabled 检查是否启用了缓存标准化模式。
func anthropicCacheNormalizationEnabled(config *provider.ChatConfig) bool {
	if config == nil {
		return false
	}
	if enabled, ok := provider.GetExtraBool(config.ProviderExtra, paramCacheNormalize); ok {
		return enabled
	}
	if enabled, ok := provider.GetExtraBool(config.ProviderExtra, paramCacheNormalizeMetadata); ok {
		return enabled
	}
	return false
}

// parseThinkingFromExtra 从 ProviderExtra["thinking"] 解析 thinkingConfig。
//
// 跨协议场景下（如 Anthropic 入口 → 任意适配器），codec 层将原始 thinking JSON
// 存入 ProviderExtra["thinking"]，此函数将其解析为 thinkingConfig 结构体。
// 返回 nil 表示 ProviderExtra 中无 thinking 或解析失败。
func parseThinkingFromExtra(extra map[string]any) *thinkingConfig {
	raw, ok := provider.GetExtraAny(extra, "thinking")
	if !ok || raw == nil {
		return nil
	}

	var tc thinkingConfig
	switch v := raw.(type) {
	case json.RawMessage:
		if err := json.Unmarshal(v, &tc); err != nil {
			return nil
		}
	case []byte:
		if err := json.Unmarshal(v, &tc); err != nil {
			return nil
		}
	case map[string]any:
		if t, ok := v["type"].(string); ok {
			tc.Type = t
		}
		if bt, ok := v["budget_tokens"].(float64); ok {
			tc.BudgetTokens = int(bt)
		}
	default:
		return nil
	}

	if tc.Type == "" {
		return nil
	}
	return &tc
}

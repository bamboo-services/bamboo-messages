package anthropic

import (
	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildTools 将统一的工具定义列表转换为 Anthropic Messages 协议格式。
//
// Anthropic 工具使用 "input_schema"（而非 OpenAI 的 "parameters"）描述参数结构，
// 同时支持通过 "cache_control" 标记对工具列表进行 Prompt Caching。
//
// 仅处理 Type == "function" 的工具定义，返回 nil 表示无可用工具。
func buildTools(tools []provider.Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		m := map[string]any{
			"name": tool.Function.Name,
		}
		// 可选字段：仅在非空时设置，避免生成多余 JSON 键
		if tool.Function.Description != "" {
			m["description"] = tool.Function.Description
		}
		if tool.Function.Parameters != nil {
			m["input_schema"] = tool.Function.Parameters
		}
		// Prompt Caching 断点标记
		if tool.CacheControl != nil {
			m["cache_control"] = buildCacheControl(tool.CacheControl)
		}
		result = append(result, m)
	}
	return result
}

// buildCacheControl 将 provider.CacheControl 转换为 Anthropic cache_control 结构。
//
// Anthropic 仅支持 "ephemeral" 类型，TTL 可选 "5m"（默认）或 "1h"。
func buildCacheControl(cc *provider.CacheControl) map[string]any {
	if cc == nil {
		return map[string]any{"type": "ephemeral"}
	}
	m := map[string]any{"type": "ephemeral"}
	if cc.TTL == provider.CacheTTL1h {
		m["ttl"] = "1h"
	}
	return m
}

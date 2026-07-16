package bamboo

import (
	"encoding/json"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// buildParams 构建 bamboo 原生协议请求体。
//
// 将 provider 层的 messages 和 ChatConfig 转换为 bamboo wire 格式的请求信封。
// 注意：此函数不设置 Stream 字段（默认 false），由调用方（chat.go/complete.go）
// 根据流式/非流式场景自行设置。
func buildParams(system string, messages []provider.Message, config *provider.ChatConfig) wireRequest {
	req := wireRequest{
		Messages: buildMessages(messages),
	}
	if system != "" {
		req.System = system
	}
	if config != nil {
		req.Config = buildWireRequestConfig(config)
	}
	return req
}

// buildWireRequestConfig 将 provider.ChatConfig 转换为 wireRequestConfig。
//
// 1:1 映射所有公共字段，包括 ThinkingConfig 和 SystemCacheControl 的特殊处理。
// config 为 nil 时返回 nil（wireRequest.Config 的 omitempty 会自动省略）。
func buildWireRequestConfig(config *provider.ChatConfig) *wireRequestConfig {
	if config == nil {
		return nil
	}
	cfg := &wireRequestConfig{
		Model:             config.Model,
		MaxTokens:         config.MaxTokens,
		Temperature:       config.Temperature,
		TopP:              config.TopP,
		StopSequences:     config.Stop,
		Metadata:          config.Metadata,
		UserID:            config.UserID,
		ToolChoice:        config.ToolChoice,
		ResponseFormat:    config.ResponseFormat,
		ParallelToolCalls: config.ParallelToolCalls,
		PromptCacheKey:    config.PromptCacheKey,
		ProviderExtra:     config.ProviderExtra,
	}
	if config.ThinkingConfig != nil {
		cfg.ThinkingConfig = &wireThinkingConfig{
			Effort:  config.ThinkingConfig.Effort,
			Display: config.ThinkingConfig.Display,
		}
	}
	if config.SystemCacheControl != nil {
		cfg.SystemCacheControl = marshalCacheControl(config.SystemCacheControl)
	}
	if tools := buildTools(config.Tools); tools != nil {
		cfg.Tools = tools
	}
	return cfg
}

// buildTools 将 provider.Tool 列表转换为 bamboo wire 工具定义列表。
//
// provider.Tool 为 OpenAI 风格 {type:"function", function:{name, description, parameters}}，
// bamboo wire 工具定义为扁平结构 {name, description, input_schema, cache_control}。
// 仅处理 Type == "function" 的工具定义，返回 nil 表示无可用工具。
func buildTools(tools []provider.Tool) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		wt := wireTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: marshalInputSchema(tool.Function.Parameters),
		}
		if tool.CacheControl != nil {
			wt.CacheControl = marshalCacheControl(tool.CacheControl)
		}
		result = append(result, wt)
	}
	return result
}

// marshalInputSchema 将 map[string]any 参数定义序列化为 json.RawMessage。
//
// 空 map 或 nil 返回 `{}`，确保 wireTool.InputSchema（无 omitempty）
// 始终输出合法的 JSON Schema。
func marshalInputSchema(params map[string]any) json.RawMessage {
	if len(params) == 0 {
		return json.RawMessage(`{}`)
	}
	data, err := json.Marshal(params)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

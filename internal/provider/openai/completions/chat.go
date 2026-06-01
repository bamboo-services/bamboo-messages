package completions

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	xError "github.com/bamboo-services/bamboo-base-go/common/error"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
)

// Chat 流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Chat Completions 格式，
// 通过底层 SDK 发起流式请求，返回 StreamEvent channel。
func (p *CompletionsProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 在消息列表前插入系统提示，然后发起流式对话。
func (p *CompletionsProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		if config == nil {
			config = &provider.ChatConfig{}
		}

		// 发送流开始事件
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
		case <-ctx.Done():
			return
		}

		params := openai.ChatCompletionNewParams{
			Model:    config.Model,
			Messages: p.buildMessages(systemPrompt, messages),
		}

		if config.MaxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(config.MaxTokens)
		}

		if config.Temperature != nil {
			params.Temperature = openai.Float(*config.Temperature)
		}

		if config.TopP != nil {
			params.TopP = openai.Float(*config.TopP)
		}

		if len(config.Stop) > 0 {
			params.Stop = buildStop(config.Stop)
		}

		if tools := buildTools(config.Tools); tools != nil {
			params.Tools = tools
		}

		if config.ThinkingConfig != nil && config.ThinkingConfig.ReasoningEffort != "" {
			params.ReasoningEffort = shared.ReasoningEffort(config.ThinkingConfig.ReasoningEffort)
		}

		if fp, ok := provider.GetExtraFloat64(config.ProviderExtra, provider.ProviderExtraKeyFrequencyPenalty); ok {
			params.FrequencyPenalty = openai.Float(fp)
		}

		if pp, ok := provider.GetExtraFloat64(config.ProviderExtra, provider.ProviderExtraKeyPresencePenalty); ok {
			params.PresencePenalty = openai.Float(pp)
		}

		if seed, ok := provider.GetExtraInt64(config.ProviderExtra, provider.ProviderExtraKeySeed); ok {
			params.Seed = openai.Int(seed)
		}

		// 用户标识
		if u, ok := provider.GetExtraString(config.ProviderExtra, provider.ProviderExtraKeyUser); ok {
			params.User = openai.String(u)
		}

		// 预测内容（用于加速已知内容的生成）
		if pred, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyPrediction); ok {
			if prediction, ok := pred.(openai.ChatCompletionPredictionContentParam); ok {
				params.Prediction = prediction
			}
		}

		// 是否启用并行工具调用
		if ptc, ok := provider.GetExtraBool(config.ProviderExtra, provider.ProviderExtraKeyParallelToolCalls); ok {
			params.ParallelToolCalls = openai.Bool(ptc)
		}

		// 附加元数据
		if len(config.Metadata) > 0 {
			params.Metadata = shared.Metadata(config.Metadata)
		}

		if tc, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyToolChoice); ok {
			// 优先处理字符串形式的 ToolChoice（"auto" / "none" / "required"）
			if s, ok := tc.(string); ok {
				params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(s)}
			} else if toolChoice, ok := tc.(openai.ChatCompletionToolChoiceOptionUnionParam); ok {
				params.ToolChoice = toolChoice
			}
		}

		if rf, ok := provider.GetExtraAny(config.ProviderExtra, provider.ProviderExtraKeyResponseFormat); ok {
			params.ResponseFormat = buildResponseFormat(rf)
		}

		// 启用 usage 流式返回
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}

		stream := p.Client.Chat.Completions.NewStreaming(ctx, params)
		defer stream.Close()

		// 追踪文本块是否已开始，用于合成 OpenAI Completions 缺失的 content_block_start 事件
		textBlockStarted := false
		thinkingBlockStarted := false

		for stream.Next() {
			chunk := stream.Current()
			events := p.handleChunk(chunk, &textBlockStarted, &thinkingBlockStarted)
			for _, e := range events {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  xError.NewError(ctx, xError.OperationFailed, "OpenAI Completions 流式对话失败", false, err),
			}:
			case <-ctx.Done():
			}
			return
		}

		// 发送完成事件
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

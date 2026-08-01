package completions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Chat 流式对话。
//
// 将统一的 provider.Message 转换为 OpenAI Chat Completions 格式，
// 通过 httpClient 发起流式请求，返回 StreamEvent channel。
func (p *CompletionsProvider) Chat(ctx context.Context, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	return p.ChatWithSystem(ctx, "", messages, config)
}

// ChatWithSystem 带系统提示的流式对话。
//
// 在消息列表前插入系统提示，然后发起流式对话。
// 使用 httpClient.DoWithDebug 发送 HTTP 请求，通过 SSEScanner 解析 SSE 流，
// 不依赖 openai-go SDK。
func (p *CompletionsProvider) ChatWithSystem(ctx context.Context, systemPrompt string, messages []provider.Message, config *provider.ChatConfig) <-chan provider.StreamEvent {
	eventCh := make(chan provider.StreamEvent, 64)

	go func() {
		defer close(eventCh)

		// 构建请求参数
		params := p.buildParams(systemPrompt, messages, config)
		params["stream"] = true
		if streamOpts := p.buildStreamOptions(); streamOpts != nil {
			params["stream_options"] = streamOpts
		}

		bodyBytes, err := json.Marshal(params)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Completions 请求参数序列化失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		endpoint := "POST /chat/completions (streaming, model=" + config.Model + ")"
		resp, err := p.httpClient.DoWithDebug(ctx, http.MethodPost, "/chat/completions", bodyBytes, string(p.GetProviderType()), endpoint)
		if err != nil {
			select {
			case eventCh <- provider.StreamEvent{
				Type: provider.StreamTypeError,
				Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Completions 流式对话请求失败: %v", err), 0),
			}:
			case <-ctx.Done():
			}
			return
		}

		// HTTP 状态码检查：>= 400 时读取 body 解析错误
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			select {
			case eventCh <- provider.StreamEvent{
				Type:       provider.StreamTypeError,
				Err:        pkgErrors.NewBambooError("上游", formatUpstreamError(resp.StatusCode, body), resp.StatusCode),
				StatusCode: resp.StatusCode,
			}:
			case <-ctx.Done():
			}
			return
		}

		// 创建 SSE 扫描器
		scanner := provider.NewSSEScanner(resp.Body)
		provider.DebugSSEResponse("openai", resp.StatusCode, provider.ResponseHeadersToMap(resp.Header))
		defer func() {
			// best-effort drain：读取残余数据以确保连接可被 Transport 复用（HTTP/1.1 keep-alive）
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = scanner.Close()
			_ = resp.Body.Close()
		}()

		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false
		stopSent := false

		// 内联 think 标签剥离器：仅在启用 WithStripThinkTags 时创建，
		// 与 SSE 事件循环共享 textBlockStarted/thinkingBlockStarted 状态。
		var stripper *thinkTagStripper
		if p.stripThinkTags {
			stripper = newThinkTagStripper()
		}

		// SSE 事件循环
		for {
			eventType, data, done, scanErr := scanner.Next()
			provider.DebugSSEFrame("openai", eventType, data)
			if done {
				break
			}
			if scanErr != nil {
				// io.EOF 表示流正常耗尽，不视为错误
				if errors.Is(scanErr, io.EOF) {
					break
				}
				// 已有内容时优雅降级：后续 !stopSent 补发 Stop + Done，
				// converter 发出完整终止序列，不暴力中断已输出的流。
				if startSent {
					break
				}
				select {
				case eventCh <- provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  pkgErrors.NewBambooError("上游", fmt.Sprintf("OpenAI Completions 流读取错误: %v", scanErr), 0),
				}:
				case <-ctx.Done():
					return
				}
				break
			}

			// 发送 Start 事件（首个有效帧时）
			if !startSent {
				startSent = true
				select {
				case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
				case <-ctx.Done():
					return
				}
			}

			// 反序列化 chunk
			var chunk chatCompletionChunk
			if jsonErr := json.Unmarshal(data, &chunk); jsonErr != nil {
				// 跳过无法解析的帧（SSEScanner 已做 json.Valid 校验，此处为业务层兜底）
				if provider.DebugEnabled {
					xLog.WithName("provider/openai-completions").SugarWarn(context.Background(),
						fmt.Sprintf("跳过无法解析的 chunk: %v, raw=%s", jsonErr, truncateBody(data)))
				}
				continue
			}

			// 处理 chunk → 事件
			events := p.handleChunk(chunk, &textBlockStarted, &thinkingBlockStarted, &stopSent, stripper)

			for _, e := range events {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		// 释放剥离器缓冲区残余（如正文末尾不完整的 "<" 候选），保证内容不丢失
		if stripper != nil {
			for _, e := range syncBlockState(stripper.flush(), &textBlockStarted, &thinkingBlockStarted) {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		// 流正常结束或降级结束但未收到 finish_reason，补发 Stop 事件
		if !stopSent {
			finishReason := provider.ResolveDegradedReason(
				p.degradedReason,
				config != nil && len(config.Tools) > 0,
			)
			select {
			case eventCh <- provider.StreamEvent{
				Type:         provider.StreamTypeStop,
				FinishReason: finishReason,
			}:
			case <-ctx.Done():
				return
			}
		}

		// 发送 Done 事件
		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

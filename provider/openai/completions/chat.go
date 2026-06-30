package completions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

var defaultStreamDrainTimeout = 5 * time.Second

type streamErrKind int

const (
	errKindNone       streamErrKind = iota
	errKindJSONParse
	errKindFatal
)

func classifyStreamError(err error) streamErrKind {
	if err == nil {
		return errKindNone
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return errKindJSONParse
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errKindFatal
	}

	msg := err.Error()
	for _, kw := range []string{
		"invalid character",
		"unexpected end of JSON",
		"JSON parse error",
		"cannot unmarshal",
		"error calling MarshalJSON",
	} {
		if strings.Contains(msg, kw) {
			return errKindJSONParse
		}
	}
	return errKindFatal
}

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
				Err:  xError.NewError(ctx, nil, fmt.Sprintf("OpenAI Completions 请求参数序列化失败: %v", err), false, err),
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
				Err:  xError.NewError(ctx, nil, fmt.Sprintf("OpenAI Completions 流式对话请求失败: %v", err), false, err),
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
				Type: provider.StreamTypeError,
				Err:  xError.NewError(ctx, nil, formatUpstreamError(resp.StatusCode, body), false, nil),
			}:
			case <-ctx.Done():
			}
			return
		}

		// 创建 SSE 扫描器
		scanner := provider.NewSSEScanner(resp.Body)
		defer func() {
			_ = scanner.Close()
			_ = resp.Body.Close()
		}()

		// 流式 drain 超时控制
		drainTimeout := p.streamDrainTimeout
		if drainTimeout <= 0 {
			drainTimeout = defaultStreamDrainTimeout
		}

		var drainMu sync.Mutex
		drainTimedOut := false
		drainStarted := false

		startDrainTimer := func() {
			drainMu.Lock()
			already := drainStarted
			drainStarted = true
			drainMu.Unlock()
			if already {
				return
			}
			time.AfterFunc(drainTimeout, func() {
				drainMu.Lock()
				drainTimedOut = true
				drainMu.Unlock()
				_ = resp.Body.Close()
			})
		}

		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false
		stopSent := false

		// SSE 事件循环
		for {
			_, data, done, scanErr := scanner.Next()
			if done {
				break
			}
			if scanErr != nil {
				// io.EOF 表示流正常耗尽，不视为错误
				if errors.Is(scanErr, io.EOF) {
					break
				}
				// 其他错误交给 classifyStreamError 处理
				errKind := classifyStreamError(scanErr)
				if errKind == errKindJSONParse {
					if !stopSent {
						stopSent = true
						select {
						case eventCh <- provider.StreamEvent{
							Type:         provider.StreamTypeStop,
							FinishReason: provider.FinishReasonStop,
						}:
						case <-ctx.Done():
							return
						}
					}
				} else {
					select {
					case eventCh <- provider.StreamEvent{
						Type: provider.StreamTypeError,
						Err:  xError.NewError(ctx, nil, fmt.Sprintf("OpenAI Completions 流读取错误: %v", scanErr), false, scanErr),
					}:
					case <-ctx.Done():
						return
					}
				}
				break
			}

			// 发送 Start 事件（首个有效帧时）
			if !startSent {
				startSent = true
				startDrainTimer()
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
					log.Printf("[provider/openai-completions] 跳过无法解析的 chunk: %v, raw=%s", jsonErr, truncateBody(data))
				}
				continue
			}

			// 处理 chunk → 事件
			events := p.handleChunk(chunk, &textBlockStarted, &thinkingBlockStarted, &stopSent)

			if stopSent {
				startDrainTimer()
			}

			for _, e := range events {
				select {
				case eventCh <- e:
				case <-ctx.Done():
					return
				}
			}
		}

		// 检查 drain 是否超时
		drainMu.Lock()
		timedOut := drainTimedOut
		drainMu.Unlock()

		if !timedOut && !stopSent {
			// 流正常结束但未收到 finish_reason，补发 Stop 事件
			select {
			case eventCh <- provider.StreamEvent{
				Type:         provider.StreamTypeStop,
				FinishReason: provider.FinishReasonStop,
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

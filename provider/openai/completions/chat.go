package completions

import (
	"context"
	"strings"
	"sync"
	"time"

	xError "github.com/bamboo-services/bamboo-messages/internal/xerr"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// streamDrainTimeout 是收到 finish_reason 后等待上游关闭连接的最大时长。
//
// 背景：openai-go SDK 的 Stream.Next() 在收到 data: [DONE] 后不 break，
// 而是继续 drain 直到底层 HTTP 连接关闭（io.EOF）。部分上游（如智谱 GLM）
// 在发送 [DONE] 后不主动关闭 TCP 连接，导致 Stream.Next() 永久阻塞。
// 此超时作为兜底：finish_reason 到达后若连接在此时长内仍未关闭，则强制关闭流。
//
// 5 秒足够覆盖正常情况下 usage chunk 和 [DONE] 帧的到达间隔，
// 同时避免异常场景下 goroutine 长时间挂起。
const streamDrainTimeout = 5 * time.Second

// streamErrKind 流错误分类，用于区分可降级的 JSON 解析错误和致命错误。
type streamErrKind int

const (
	errKindNone       streamErrKind = iota // nil 错误
	errKindJSONParse                       // JSON 解析错误（可降级处理）
	errKindFatal                           // 致命错误（网络故障、上游主动报错等）
)

// jsonErrKeywords 是 json.Unmarshal / json.NewDecoder 失败时常见的错误消息片段。
//
// openai-go SDK 的 ssestream.Stream.Next() 在 L216 调用 json.Unmarshal(data, &nxt)，
// 当上游（如智谱 GLM Issue #66）发送截断或粘连的 SSE JSON 帧时，此调用失败，
// 错误存储在 stream.Err() 中返回给调用方。
// 这些错误消息来源于 Go encoding/json 包的标准输出。
var jsonErrKeywords = []string{
	"invalid character",        // json.Unmarshal 最常见：非法字符
	"unexpected end of JSON",   // JSON 截断（空 data 或不完整帧）
	"JSON parse error",         // 通用 JSON 解析错误描述
	"expected",                 // "expected ',' or '}'" 等结构错误
	"cannot unmarshal",         // 类型不匹配
	"error calling MarshalJSON", // 自定义 MarshalJSON 失败
}

// classifyStreamError 对 stream.Err() 返回的错误进行分类。
//
// 部分上游（如智谱 GLM Coding-MAX）存在已知的 SSE 帧截断 Bug
//（zai-org/GLM-5#66）：两个 data: 帧粘连导致 JSON 不完整，
// openai-go SDK 的 json.Unmarshal 失败后 stream.Next() 返回 false。
//
// 此函数区分两类错误：
//   - errKindJSONParse：JSON 解析失败，但底层 TCP/HTTP 连接正常。
//     调用方可选择降级处理（合成 Stop 事件），而非将整个流标记为错误。
//   - errKindFatal：网络层故障（连接重置、EOF）或上游主动 SSE error，
//     必须作为错误上报。
//
// 分类策略：检查错误消息是否包含 JSON 解析相关的关键词。
// context.Canceled / io.EOF / connection reset 等不属于 JSON 解析错误。
func classifyStreamError(err error) streamErrKind {
	if err == nil {
		return errKindNone
	}

	msg := err.Error()
	for _, kw := range jsonErrKeywords {
		if strings.Contains(msg, kw) {
			return errKindJSONParse
		}
	}
	return errKindFatal
}

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

		params := p.buildParams(systemPrompt, messages, config)
		params.StreamOptions = p.buildStreamOptions()

		provider.DebugRequest(
			"openai-completions",
			"POST /chat/completions (streaming, model="+config.Model+")",
			nil,
			params,
		)

		stream := p.Client.Chat.Completions.NewStreaming(ctx, params)
		defer stream.Close()

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
			time.AfterFunc(streamDrainTimeout, func() {
				drainMu.Lock()
				drainTimedOut = true
				drainMu.Unlock()
				_ = stream.Close()
			})
		}

		textBlockStarted := false
		thinkingBlockStarted := false
		startSent := false
		stopSent := false

		for stream.Next() {
			if !startSent {
				startSent = true
				select {
				case eventCh <- provider.StreamEvent{Type: provider.StreamTypeStart}:
				case <-ctx.Done():
					return
				}
			}

			chunk := stream.Current()
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

		drainMu.Lock()
		timedOut := drainTimedOut
		drainMu.Unlock()

		if !timedOut {
			if err := stream.Err(); err != nil {
				errKind := classifyStreamError(err)

				// JSON 解析错误降级：当上游（如智谱 GLM Issue #66）发送截断/粘连的
				// SSE JSON 帧导致 json.Unmarshal 失败时，若已发送过内容（startSent），
				// 不上报致命错误，而是合成 Stop 事件让流优雅终止。
				//
				// 这样下游客户端收到正常的 finish_reason 而非 Error，
				// 避免整个对话被判定为失败（即使大部分内容已成功传输）。
				if errKind == errKindJSONParse && startSent {
					if !stopSent {
						select {
						case eventCh <- provider.StreamEvent{
							Type:         provider.StreamTypeStop,
							FinishReason: provider.FinishReasonStop,
						}:
						case <-ctx.Done():
							return
						}
					}
					// JSON 解析错误已降级，跳过 Error 事件，直接进入 Done
				} else {
					select {
					case eventCh <- provider.StreamEvent{
						Type: provider.StreamTypeError,
						Err:  xError.NewError(ctx, nil, formatUpstreamError(err), false, err),
					}:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		select {
		case eventCh <- provider.StreamEvent{Type: provider.StreamTypeDone}:
		case <-ctx.Done():
		}
	}()

	return eventCh
}

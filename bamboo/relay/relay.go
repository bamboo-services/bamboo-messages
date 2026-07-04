package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// Relay 非流式协议互转。
//
// 将 inFormat 协议格式的请求体交给底层 Provider 处理，
// 并将响应序列化为 outFormat 协议格式返回。
//
// 参数：
//   - ctx: 上下文，支持取消和超时
//   - p: 底层协议适配器（Provider 接口实现）
//   - body: 输入协议的原始请求体 JSON
//   - inFormat: 输入协议格式（如 codec.FormatOpenAI）
//   - outFormat: 输出协议格式（如 codec.FormatAnthropic）
//   - opts: 可选配置（Usage/Error 回调）
//
// 返回 outFormat 格式的响应 JSON 字节，或错误。
//
// 内部流程：
//  1. 通过 inCodec.ParseRequest(body) 解析为统一 RelayRequest
//  2. 使用 bamboo.NewClient(p).Complete() 调用底层 Provider
//  3. 通过 outCodec.SerializeResponse() 序列化为目标格式
func Relay(
	ctx context.Context,
	p provider.Provider,
	body []byte,
	inFormat codec.FormatType,
	outFormat codec.FormatType,
	opts ...Option,
) ([]byte, error) {
	cfg := applyOptions(opts...)

	// ── 获取 codecs ──
	inCodec, err := codec.Get(inFormat)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}
	if inCodec == nil {
		err = pkgErrors.NewBambooError("下游", fmt.Sprintf("codec %q is not registered", inFormat), 0)
		cfg.triggerError(err)
		return nil, err
	}

	outCodec, err := codec.Get(outFormat)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}
	if outCodec == nil {
		err = pkgErrors.NewBambooError("下游", fmt.Sprintf("codec %q is not registered", outFormat), 0)
		cfg.triggerError(err)
		return nil, err
	}

	// ── 解析请求 ──
	debugRelayInput("Relay", inFormat, outFormat, body)

	req, err := inCodec.ParseRequest(body)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}

	debugRelayParsed("Relay", inFormat, req)

	// ── 调用 Provider（通过 bamboo client 包装） ──
	client := bamboo.NewClient(p)
	resp, err := client.Complete(ctx, req.Messages, req.System, req.Config)
	if err != nil {
		// 即使出错，如果 resp 非 nil 也尝试触发 usage 回调（兜底）
		if resp != nil {
			cfg.triggerUsage(resp.Usage)
		}
		cfg.triggerError(err)

		// 将上游错误序列化为目标协议格式的错误响应 body，
		// 确保调用方（如 newapi）拿到协议格式的错误 JSON 而非空 body。
		errorBody := outCodec.SerializeError(err)
		debugRelayResponse("Relay", inFormat, outFormat, errorBody)
		return errorBody, toBambooError(err)
	}

	// ── 触发 Usage 回调 ──
	cfg.triggerUsage(resp.Usage)

	// ── 序列化响应 ──
	data, err := outCodec.SerializeResponse(resp)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}
	debugRelayResponse("Relay", inFormat, outFormat, data)

	return data, nil
}

// RelayStream 流式协议互转。
//
// 将 inFormat 协议格式的请求体交给底层 Provider 处理（流式），
// 并将流事件逐步序列化为 outFormat 协议的 SSE 数据帧。
//
// 参数：
//   - ctx: 上下文，支持取消和超时
//   - p: 底层协议适配器
//   - body: 输入协议的原始请求体 JSON
//   - inFormat: 输入协议格式
//   - outFormat: 输出协议格式
//   - opts: 可选配置（Usage/Error 回调）
//
// 返回 SSE 数据帧 channel（流结束后自动关闭），或错误。
//
// 内部流程：
//  1. 通过 inCodec.ParseRequest(body) 解析为统一 RelayRequest
//  2. 使用 bamboo.NewClient(p).Chat() 获取流事件 channel
//  3. 启动 goroutine：遍历事件 → outCodec 序列化 → 发送到输出 channel
//  4. 流结束后调用 serializer.Flush() 输出终止标记（如 `data: [DONE]`）
func RelayStream(
	ctx context.Context,
	p provider.Provider,
	body []byte,
	inFormat codec.FormatType,
	outFormat codec.FormatType,
	opts ...Option,
) (<-chan []byte, error) {
	cfg := applyOptions(opts...)

	// ── 获取 codecs ──
	inCodec, err := codec.Get(inFormat)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}
	if inCodec == nil {
		err = pkgErrors.NewBambooError("下游", fmt.Sprintf("codec %q is not registered", inFormat), 0)
		cfg.triggerError(err)
		return nil, err
	}

	outCodec, err := codec.Get(outFormat)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}
	if outCodec == nil {
		err = pkgErrors.NewBambooError("下游", fmt.Sprintf("codec %q is not registered", outFormat), 0)
		cfg.triggerError(err)
		return nil, err
	}

	// ── 解析请求 ──
	debugRelayInput("RelayStream", inFormat, outFormat, body)

	req, err := inCodec.ParseRequest(body)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}

	debugRelayParsed("RelayStream", inFormat, req)

	// ── 调用 Provider 获取流事件 channel ──
	client := bamboo.NewClient(p)
	eventCh, err := client.Chat(ctx, req.Messages, req.System, req.Config)
	if err != nil {
		cfg.triggerError(err)
		return nil, toBambooError(err)
	}

	// ── 启动转换 goroutine ──
	serializer := outCodec.NewSerializer(req.Config.Model)
	out := make(chan []byte, 64)

	go func() {
		var lastUsage *bamboo.Usage
		usageTriggered := false
		var accumulatedOutput strings.Builder

		// ── 平滑缓冲器（可选）──
		var pacer *SmoothPacer
		if cfg.Smooth != nil && cfg.Smooth.Level != SmoothLevelOff {
			pacer = NewSmoothPacer(outFormat, cfg.Smooth.Params, out, ctx)
			if cfg.OnRateSample != nil {
				pacer.SetRateSampleCallback(cfg.OnRateSample)
			}
		}

		// defer LIFO 顺序: panicRecovery → close(out) → pacerCleanup → usageFallback
		// pacerCleanup 必须在 close(out) 之前执行（LIFO 中后注册先执行）
		defer func() {
			if r := recover(); r != nil {
				cfg.triggerError(pkgErrors.NewBambooError("下游", fmt.Sprintf("goroutine panic: %v", r), 0))
				if pacer != nil {
					pacer.SignalEnd()
				}
			}
		}()
		defer close(out)
		defer func() {
			if pacer != nil {
				pacer.SignalEnd()
				pacer.Wait()
			}
		}()
		defer func() {
			if !usageTriggered {
				if lastUsage != nil {
					cfg.triggerUsage(*lastUsage)
				} else if cfg.EstimateOnMissingUsage {
					estimated := estimateUsage(req.Messages, req.System, accumulatedOutput)
					cfg.triggerUsage(estimated)
				}
			}
		}()

		for event := range eventCh {
			// 处理错误事件
			if event.Type == bamboo.EventError && event.Error != nil {
				cfg.triggerError(event.Error)
			}

			// 处理 Usage 事件
			// 注意：StreamConverter.handleStop() 在 usage 缺失时会生成零值 &Usage{}，
			// 需要检查 InputTokens/OutputTokens 是否全零以区分真实 usage 和占位 usage。
			if event.Usage != nil && (event.Usage.InputTokens > 0 || event.Usage.OutputTokens > 0) {
				lastUsage = event.Usage
				cfg.triggerUsage(*event.Usage)
				usageTriggered = true
			}

			// 累积输出文本（用于 usage 缺失时的估算回退）
			if cfg.EstimateOnMissingUsage && !usageTriggered {
				if delta, ok := event.Delta.(*bamboo.StreamDelta); ok {
					switch delta.Type {
					case bamboo.DeltaTextDelta:
						accumulatedOutput.WriteString(delta.Text)
					case bamboo.DeltaThinkingDelta:
						accumulatedOutput.WriteString(delta.Thinking)
					case bamboo.DeltaInputJSON:
						accumulatedOutput.WriteString(delta.PartialJSON)
					}
				}
			}

			// 序列化事件
			data, sErr := serializer.Serialize(event)
			if sErr != nil {
				cfg.triggerError(toBambooError(sErr))
				continue
			}
			if data != nil {
				debugRelayResponseFrame("RelayStream", inFormat, outFormat, data)
				// ping 保活帧绕过 SmoothPacer，直接写入 out channel。
				// 原因：ping 的作用是对抗反向代理 idle timeout，如果经过 pacer 队列
				// 排队，在队列积压时会失去实时保活意义，导致 nginx/ALB 断连。
				if event.Type == bamboo.EventPing {
					select {
					case out <- data:
					case <-ctx.Done():
						return
					}
				} else if pacer != nil {
					pacer.Push(data)
				} else {
					select {
					case out <- data:
					case <-ctx.Done():
						return
					}
				}
			}

			// 注意：不在 error 事件处 break。
			// StreamConverter.handleError 会自动补发完整的终止序列
			//（block_stop + message_delta + message_stop），
			// 这些事件必须被正常序列化输出，否则下游客户端收不到 finish_reason，
			// 导致流被判定为异常中断。
		}

		// ── Flush 终止标记 ──
		flushData, fErr := serializer.Flush()
		if fErr != nil {
			cfg.triggerError(toBambooError(fErr))
			return
		}
		if flushData != nil {
			debugRelayResponseFrame("RelayStream", inFormat, outFormat, flushData)
			if pacer != nil {
				pacer.Push(flushData)
			} else {
				select {
				case out <- flushData:
				case <-ctx.Done():
				}
			}
		}
		// pacer.SignalEnd + pacer.Wait 由 defer 统一处理
	}()

	return out, nil
}

// estimateTokenCount 基于 CJK 1:1 / Latin 4:1 / Other 2:1 规则估算 token 数。
//
// 与 provider.charCounter.estimateTokens() 逻辑一致，
// 在 relay 层独立实现以避免跨包依赖。
func estimateTokenCount(text string) int64 {
	var cjk, latin, other int64
	for _, r := range text {
		switch {
		case isCJKRune(r):
			cjk++
		case isLatinAlnumRune(r):
			latin++
		default:
			other++
		}
	}
	return cjk + latin/4 + other/2
}

// isCJKRune 判断是否为 CJK 字符（汉字/平假名/片假名/谚文）。
func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// isLatinAlnumRune 判断是否为 ASCII Latin 字母或数字。
func isLatinAlnumRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

// estimateUsage 在上游 usage 缺失时，基于请求内容和输出内容估算 token 用量。
//
// input_tokens: 请求 messages 中的文本内容 + system prompt 的 token 估算。
// output_tokens: 流式过程中累积的输出文本（text/thinking/tool_call JSON）的 token 估算。
// cache 字段无法估算，设为 0。
func estimateUsage(messages []bamboo.BambooMessage, system string, output strings.Builder) bamboo.Usage {
	var inputText strings.Builder
	inputText.WriteString(system)
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch b := block.(type) {
			case *bamboo.TextBlock:
				inputText.WriteString(b.Text)
			case *bamboo.ToolUseBlock:
				inputText.WriteString(b.Name)
			case *bamboo.ToolResultBlock:
				inputText.WriteString(b.Content)
			}
		}
	}

	return bamboo.Usage{
		InputTokens:  estimateTokenCount(inputText.String()),
		OutputTokens: estimateTokenCount(output.String()),
	}
}

func toBambooError(err error) error {
	if err == nil {
		return nil
	}
	var be *pkgErrors.BambooError
	if errors.As(err, &be) {
		return err
	}
	return pkgErrors.NewBambooError("下游", err.Error(), 0)
}

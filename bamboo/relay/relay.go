package relay

import (
	"context"
	"fmt"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
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
		return nil, fmt.Errorf("relay: failed to get input codec: %w", err)
	}
	if inCodec == nil {
		err = fmt.Errorf("relay: input codec %q is not registered", inFormat)
		cfg.triggerError(err)
		return nil, err
	}

	outCodec, err := codec.Get(outFormat)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: failed to get output codec: %w", err)
	}
	if outCodec == nil {
		err = fmt.Errorf("relay: output codec %q is not registered", outFormat)
		cfg.triggerError(err)
		return nil, err
	}

	// ── 解析请求 ──
	dbg := shouldDebug(cfg)
	debugRelayInput(dbg, "Relay", inFormat, outFormat, body)

	req, err := inCodec.ParseRequest(body)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: failed to parse request: %w", err)
	}

	debugRelayParsed(dbg, "Relay", inFormat, req)

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
		return errorBody, fmt.Errorf("relay: provider complete failed: %w", err)
	}

	// ── 触发 Usage 回调 ──
	cfg.triggerUsage(resp.Usage)

	// ── 序列化响应 ──
	data, err := outCodec.SerializeResponse(resp)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: failed to serialize response: %w", err)
	}

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
		return nil, fmt.Errorf("relay: failed to get input codec: %w", err)
	}
	if inCodec == nil {
		err = fmt.Errorf("relay: input codec %q is not registered", inFormat)
		cfg.triggerError(err)
		return nil, err
	}

	outCodec, err := codec.Get(outFormat)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: failed to get output codec: %w", err)
	}
	if outCodec == nil {
		err = fmt.Errorf("relay: output codec %q is not registered", outFormat)
		cfg.triggerError(err)
		return nil, err
	}

	// ── 解析请求 ──
	dbg := shouldDebug(cfg)
	debugRelayInput(dbg, "RelayStream", inFormat, outFormat, body)

	req, err := inCodec.ParseRequest(body)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: failed to parse request: %w", err)
	}

	debugRelayParsed(dbg, "RelayStream", inFormat, req)

	// ── 调用 Provider 获取流事件 channel ──
	client := bamboo.NewClient(p)
	eventCh, err := client.Chat(ctx, req.Messages, req.System, req.Config)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: provider chat failed: %w", err)
	}

	// ── 启动转换 goroutine ──
	serializer := outCodec.NewSerializer(req.Config.Model)
	out := make(chan []byte, 64)

	go func() {
		var lastUsage *bamboo.Usage
		usageTriggered := false

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
				cfg.triggerError(fmt.Errorf("relay: goroutine panic: %v", r))
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
			if !usageTriggered && lastUsage != nil {
				cfg.triggerUsage(*lastUsage)
			}
		}()

		for event := range eventCh {
			// 处理错误事件
			if event.Type == bamboo.EventError && event.Error != nil {
				cfg.triggerError(event.Error)
			}

			// 处理 Usage 事件
			if event.Usage != nil {
				lastUsage = event.Usage
				cfg.triggerUsage(*event.Usage)
				usageTriggered = true
			}

			// 序列化事件
			data, sErr := serializer.Serialize(event)
			if sErr != nil {
				cfg.triggerError(fmt.Errorf("relay: serialize error: %w", sErr))
				continue
			}
			if data != nil {
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
			cfg.triggerError(fmt.Errorf("relay: flush error: %w", fErr))
			return
		}
		if flushData != nil {
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

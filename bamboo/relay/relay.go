package relay

import (
	"context"
	"fmt"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	"github.com/bamboo-services/bamboo-messages/internal/provider"
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
	req, err := inCodec.ParseRequest(body)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: failed to parse request: %w", err)
	}

	// ── 调用 Provider（通过 bamboo client 包装） ──
	client := bamboo.NewClient(p)
	resp, err := client.Complete(ctx, req.Messages, req.System, req.Config)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: provider complete failed: %w", err)
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
	req, err := inCodec.ParseRequest(body)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: failed to parse request: %w", err)
	}

	// ── 调用 Provider 获取流事件 channel ──
	client := bamboo.NewClient(p)
	eventCh, err := client.Chat(ctx, req.Messages, req.System, req.Config)
	if err != nil {
		cfg.triggerError(err)
		return nil, fmt.Errorf("relay: provider chat failed: %w", err)
	}

	// ── 启动转换 goroutine ──
	serializer := outCodec.NewSerializer()
	out := make(chan []byte, 64)

	go func() {
		defer close(out)

		for event := range eventCh {
			// 处理错误事件
			if event.Type == bamboo.EventError && event.Error != nil {
				cfg.triggerError(event.Error)
			}

			// 处理 Usage 事件
			if event.Usage != nil {
				cfg.triggerUsage(*event.Usage)
			}

			// 序列化事件
			data, sErr := serializer.Serialize(event)
			if sErr != nil {
				cfg.triggerError(fmt.Errorf("relay: serialize error: %w", sErr))
				continue
			}
			if data != nil {
				select {
				case out <- data:
				case <-ctx.Done():
					return
				}
			}
		}

		// ── Flush 终止标记 ──
		flushData, fErr := serializer.Flush()
		if fErr != nil {
			cfg.triggerError(fmt.Errorf("relay: flush error: %w", fErr))
			return
		}
		if flushData != nil {
			select {
			case out <- flushData:
			case <-ctx.Done():
			}
		}
	}()

	return out, nil
}

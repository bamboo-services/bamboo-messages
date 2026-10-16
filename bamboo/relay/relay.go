package relay

import (
	"bytes"
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

// Relay 执行非流式跨协议请求转换。
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
		bErr := toBambooError(err)
		var be *pkgErrors.BambooError
		if errors.As(bErr, &be) && be.StatusCode == 0 {
			be.StatusCode = 400
		}
		cfg.triggerError(bErr)
		errorBody := outCodec.SerializeError(bErr)
		debugRelayResponse("Relay", inFormat, outFormat, errorBody)
		return errorBody, bErr
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

// RelayStream 执行流式跨协议请求转换并返回只读 SSE 字节流 channel。
//
// 启动异步转换 goroutine，持续消费底层 Provider 的事件流并逐步序列化为 outFormat 协议的 SSE 数据帧。
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

		// defer LIFO 顺序: panicRecovery → close(out) → usageFallback
		defer func() {
			if r := recover(); r != nil {
				cfg.triggerError(pkgErrors.NewBambooError("下游", fmt.Sprintf("goroutine panic: %v", r), 0))
			}
		}()
		defer close(out)
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
				// 按 SSE 事件边界拆分：某些 codec 序列化器（如 OpenAI handleMessageDelta）
				// 会将 finish_reason + usage 两个 data 帧合并为单个 []byte。
				// 拆分后逐帧发送，确保上游调用方每个 []byte 收到的是单个 SSE 事件。
				frames := SplitSSEFrames(data)
				for _, frame := range frames {
					select {
					case out <- frame:
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
			frames := SplitSSEFrames(flushData)
			for _, frame := range frames {
				select {
				case out <- frame:
				case <-ctx.Done():
					return
				}
			}
		}
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

// SplitSSEFrames 按 SSE 事件边界（\n\n）拆分为独立帧。
//
// 某些 codec 序列化器（如 OpenAI handleMessageDelta）将多个 SSE 事件
// 合并为单个 []byte（finish_reason + usage 两个 data 帧连接在一起）。
// 此函数确保上游调用方每个 []byte 收到的是单个完整 SSE 事件。
//
// 在 SSE 规范中，\n\n 是事件边界，不会出现在单个事件的 data 行内
// （多行 data 按 \n 拼接，最多产生单个 \n，不会产生 \n\n）。
//
// 导出供上层业务（如 newapi bridge）在直接调用 codec serializer 时使用，
// 确保 marshalChunks 产生的合并帧被正确拆分为独立 SSE 事件。
func SplitSSEFrames(data []byte) [][]byte {
	if !bytes.Contains(data, []byte("\n\n")) {
		return [][]byte{data}
	}

	var frames [][]byte
	parts := bytes.Split(data, []byte("\n\n"))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		frame := make([]byte, len(part)+2)
		copy(frame, part)
		frame[len(part)] = '\n'
		frame[len(part)+1] = '\n'
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		return [][]byte{data}
	}
	return frames
}

// ExtractSSEDataPayloads 从单个或合并的 SSE 字节流中提取所有有效 JSON data 负载。
//
// 用于将 RelayStream 产出的 SSE 帧（如 `event: response.created\ndata: {...}\n\n`）
// 无损转换为 WebSocket 文本帧所需的纯 JSON 负载列表：
//   - 自动跳过 `: keep-alive` 等 SSE 注释帧；
//   - 自动跳过 `data: [DONE]` 终止哨兵；
//   - 支持单次输入包含多个合并的 SSE 事件。
func ExtractSSEDataPayloads(sseData []byte) [][]byte {
	frames := SplitSSEFrames(sseData)
	var payloads [][]byte
	for _, frame := range frames {
		trimmed := bytes.TrimSpace(frame)
		if len(trimmed) == 0 || bytes.HasPrefix(trimmed, []byte(":")) {
			continue
		}
		lines := bytes.Split(trimmed, []byte("\n"))
		var dataParts [][]byte
		for _, line := range lines {
			line = bytes.TrimRight(line, "\r")
			if bytes.HasPrefix(line, []byte("data:")) {
				val := bytes.TrimPrefix(line, []byte("data:"))
				if len(val) > 0 && val[0] == ' ' {
					val = val[1:]
				}
				dataParts = append(dataParts, val)
			}
		}
		if len(dataParts) == 0 {
			continue
		}
		joined := bytes.Join(dataParts, []byte("\n"))
		if bytes.Equal(bytes.TrimSpace(joined), []byte("[DONE]")) {
			continue
		}
		payloads = append(payloads, joined)
	}
	return payloads
}

// RelayWebSocketMessages 将流式协议互转输出转换为逐条纯 JSON 消息流（适用于 Responses WebSocket 等下行通道）。
//
// 内部复用 RelayStream，自动剥离 SSE `event:` / `data:` 包装并过滤心跳注释与 `[DONE]` 哨兵，
// 每条发往返回 channel 的 []byte 均为可直接通过 WebSocket WriteMessage(TextMessage, ...) 发送的完整 JSON 事件。
func RelayWebSocketMessages(
	ctx context.Context,
	p provider.Provider,
	body []byte,
	inFormat codec.FormatType,
	outFormat codec.FormatType,
	opts ...Option,
) (<-chan []byte, error) {
	sseCh, err := RelayStream(ctx, p, body, inFormat, outFormat, opts...)
	if err != nil {
		return nil, err
	}
	wsOut := make(chan []byte, 64)
	go func() {
		defer close(wsOut)
		for sseFrame := range sseCh {
			for _, payload := range ExtractSSEDataPayloads(sseFrame) {
				select {
				case wsOut <- payload:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return wsOut, nil
}

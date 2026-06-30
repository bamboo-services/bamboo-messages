package completions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/openai/openai-go/v3/packages/ssestream"
)

// resilientDecoder 是对标准 eventStreamDecoder 的容错包装。
//
// 智谱 GLM Issue #66（zai-org/GLM-5#66）存在服务端 SSE 帧截断 Bug：
// 两个 data: 帧在 JSON 对象中间粘连，导致第一个 JSON 不完整。
// openai-go SDK 的 eventStreamDecoder 将 data 行拼接后交给 json.Unmarshal，
// 解析失败后 stream.Next() 返回 false，整个流永久中断。
//
// resilientDecoder 在 SDK 的 json.Unmarshal 之前做 JSON 完整性校验：
//   - 完整 JSON → 正常传递
//   - 截断/不完整 JSON → 丢弃当前帧，内部 retry 读取下一帧
//   - [DONE] 哨兵 → 正常传递
//   - 空事件 → 跳过
//
// 这样 SDK 读到的每个 data 都是完整 JSON，永不触发 json.Unmarshal 失败，
// 流在遇到损坏帧后自动跳过并继续读取后续完整帧，实现"兜底继续"。
//
// 注册方式：通过 ssestream.RegisterDecoder 覆盖 text/event-stream 的默认 Decoder。
// 当 contentType 不是 text/event-stream 时，SDK 回退到默认行为。
type resilientDecoder struct {
	inner ssestream.Decoder
}

func newResilientDecoder(rc io.ReadCloser) ssestream.Decoder {
	scn := bufio.NewScanner(rc)
	scn.Buffer(nil, bufio.MaxScanTokenSize<<9)
	return &resilientDecoder{
		inner: &eventStreamDecoderShim{rc: rc, scn: scn},
	}
}

func (d *resilientDecoder) Event() ssestream.Event {
	return d.inner.Event()
}

func (d *resilientDecoder) Next() bool {
	for d.inner.Next() {
		evt := d.inner.Event()
		data := evt.Data

		// [DONE] 哨兵 — 正常传递
		if bytes.HasPrefix(data, []byte("[DONE]")) {
			return true
		}

		// 空事件 — 跳过
		if len(bytes.TrimSpace(data)) == 0 {
			continue
		}

		// JSON 完整性校验
		if json.Valid(data) {
			return true
		}

		// 损坏帧 — 跳过并继续读取下一帧
		// 此处不 log，避免在高频场景下刷屏。可通过 BAMBOO_DEBUG 观察。
		continue
	}
	return false
}

func (d *resilientDecoder) Close() error {
	return d.inner.Close()
}

func (d *resilientDecoder) Err() error {
	return d.inner.Err()
}

// eventStreamDecoderShim 复制自 openai-go SDK 的 eventStreamDecoder，
// 因为原始类型未导出，无法直接引用。
// 逻辑与 ssestream.go:65-135 完全一致。
type eventStreamDecoderShim struct {
	evt ssestream.Event
	rc  io.ReadCloser
	scn *bufio.Scanner
	err error
}

func (s *eventStreamDecoderShim) Next() bool {
	if s.err != nil {
		return false
	}

	event := ""
	data := bytes.NewBuffer(nil)

	for s.scn.Scan() {
		txt := s.scn.Bytes()

		if len(txt) == 0 {
			s.evt = ssestream.Event{
				Type: event,
				Data: data.Bytes(),
			}
			return true
		}

		name, value, _ := bytes.Cut(txt, []byte(":"))

		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		switch string(name) {
		case "":
			continue
		case "event":
			event = string(value)
		case "data":
			_, s.err = data.Write(value)
			if s.err != nil {
				break
			}
			_, s.err = data.WriteRune('\n')
			if s.err != nil {
				break
			}
		}
	}

	if s.scn.Err() != nil {
		s.err = s.scn.Err()
	}

	return false
}

func (s *eventStreamDecoderShim) Event() ssestream.Event {
	return s.evt
}

func (s *eventStreamDecoderShim) Close() error {
	return s.rc.Close()
}

func (s *eventStreamDecoderShim) Err() error {
	return s.err
}

// registerResilientDecoder 注册容错 SSE Decoder。
//
// 在 init() 中调用 ssestream.RegisterDecoder 覆盖 text/event-stream 的默认 Decoder。
// 注册后，所有经过 OpenAI Completions 适配器的流式请求都使用 resilientDecoder，
// 自动跳过 GLM Issue #66 导致的截断/粘连帧，流不会因损坏帧而中断。
//
// 注意：这是进程级全局注册。如果其他 Provider（Responses / Anthropic 等）也使用
// text/event-stream，它们也会经过 resilientDecoder。但由于 resilientDecoder 对完整
// JSON 透传不修改，对正常流式无副作用。
func init() {
	ssestream.RegisterDecoder("text/event-stream", func(rc io.ReadCloser) ssestream.Decoder {
		return newResilientDecoder(rc)
	})
}

// 确保在未被任何测试引用时不产生 unused 警告
var _ = http.Header{}

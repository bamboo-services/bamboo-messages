package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
)

// sseScannerBufferCapacity 初始缓冲容量。
const sseScannerBufferCapacity = 64 * 1024

// sseScannerMaxBufferSize 最大缓冲容量（1MB），防止单帧过大导致 OOM。
const sseScannerMaxBufferSize = 1 << 20

// SSEScanner 共享 SSE 帧解析器，内置 json.Valid 容错能力。
//
// 针对智谱 GLM Issue #66（zai-org/GLM-5#66）服务端 SSE 帧截断 Bug：
// 两个 data: 帧在 JSON 对象中间粘连，导致第一个 JSON 不完整。
// openai-go SDK 将 data 行拼接后直接 json.Unmarshal，解析失败导致整个流中断。
//
// SSEScanner 将"帧切分"与"JSON 反序列化"解耦：
//   - 按 SSE 规范（https://html.spec.whatwg.org/multipage/server-sent-events.html）切分帧
//   - 对每个帧仅做 json.Valid() 完整性校验（仅当数据看起来像 JSON 时）
//   - 完整 JSON → 正常返回给上层
//   - 截断/不完整 JSON → 跳过该帧，继续读取下一帧（GLM 容错）
//   - [DONE] 哨兵 → 返回 done=true
//   - 注释行（: keep-alive）→ 跳过
//   - 非 JSON 文本数据（不以 { 或 [ 开头）→ 原样返回
//
// 上层调用方拿到 json.RawMessage 后自行 Unmarshal 为具体类型，
// 解析失败是上层自己的业务逻辑问题，不影响 SSEScanner 继续读流。
type SSEScanner struct {
	rc        io.ReadCloser  // 底层数据源（用于 Close 释放连接）
	scanner   *bufio.Scanner // 底层行扫描器
	dataBuf   bytes.Buffer   // 当前事件的 data 累积缓冲（多行 data 按 \n 拼接）
	eventType string         // 当前事件的 event: 类型（无 event: 行时为空）
	err       error          // 终端错误状态（scanner.Err 或 io.EOF），设置后 Next() 永远返回同一状态
	done      bool           // 是否收到 [DONE] 哨兵
}

// NewSSEScanner 从 io.ReadCloser 创建 SSE 帧解析器。
//
// 内部创建 bufio.Scanner 并设置 1MB 最大缓冲，防止超大帧导致 bufio.ErrTooLong。
// 调用方负责在读取完成后 Close 底层的 ReadCloser（通过 [SSEScanner.Close]）。
func NewSSEScanner(rc io.ReadCloser) *SSEScanner {
	scn := bufio.NewScanner(rc)
	scn.Buffer(make([]byte, 0, sseScannerBufferCapacity), sseScannerMaxBufferSize)
	return &SSEScanner{
		rc:      rc,
		scanner: scn,
	}
}

// Next 读取下一个 SSE 事件。
//
// 返回值：
//   - eventType: event: 行的事件类型（无 event: 行时为空字符串）
//   - data: data: 行累积的原始 JSON（json.RawMessage），已通过 json.Valid 校验
//   - done: 收到 [DONE] 哨兵时为 true，表示流正常结束
//   - err: 底层读取错误（如 io.EOF 表示流耗尽）
//
// 处理规则（SSE 规范）：
//   - 空行（""）→ 事件边界，分派当前累积的 data/eventType
//   - data: 前缀 → 追加到 dataBuf（多行 data 按 \n 拼接）
//   - event: 前缀 → 设置当前事件类型
//   - : 前缀（含 ": keep-alive"）→ SSE 注释，跳过
//   - 其他行（id: / retry: 等未知字段）→ 跳过
//
// 容错规则：
//   - dataBuf 内容为 [DONE] → 返回 done=true
//   - json.Valid(dataBuf) 通过 → 返回给上层
//   - json.Valid(dataBuf) 失败 → 跳过该帧（log 记录），重置缓冲，继续读取
//
// 终态保护：一旦进入 done 或 err 状态，后续 Next() 调用返回相同结果，不会重复读取。
func (s *SSEScanner) Next() (eventType string, data json.RawMessage, done bool, err error) {
	// 终态保护：已结束的 scanner 返回上一次的状态
	if s.done {
		return "", nil, true, nil
	}
	if s.err != nil {
		return "", nil, false, s.err
	}

	for s.scanner.Scan() {
		line := s.scanner.Bytes()

		// ── 空行：事件边界 ──
		if len(line) == 0 {
			// 无累积数据 → 跳过（连续空行或开头空行）
			if s.dataBuf.Len() == 0 {
				s.eventType = ""
				continue
			}

			evType, result, isDone := s.dispatch()
			if isDone {
				// [DONE] 哨兵
				return "", nil, true, nil
			}
			if result != nil {
				// 正常事件
				return evType, result, false, nil
			}
			// result == nil：JSON 校验失败，已跳过，继续循环读取下一帧
			continue
		}

		// ── SSE 注释行（: keep-alive / : 开始的行）──
		if line[0] == ':' {
			continue
		}

		// ── 字段行（field: value）──
		name, value, found := bytes.Cut(line, []byte(":"))
		if !found {
			// 无冒号的行不符合 SSE 规范，跳过
			continue
		}

		// SSE 规范：冒号后如有一个前导空格，需移除
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		switch string(name) {
		case "event":
			s.eventType = string(value)

		case "data":
			// 多行 data 按 SSE 规范用 \n 拼接
			if s.dataBuf.Len() > 0 {
				s.dataBuf.WriteByte('\n')
			}
			s.dataBuf.Write(value)

		default:
			// id: / retry: 等未知字段 → 跳过（不做 SSE 重连）
		}
	}

	// ── scanner.Scan() 返回 false：流耗尽或出错 ──
	if scanErr := s.scanner.Err(); scanErr != nil {
		s.err = scanErr
		return "", nil, false, s.err
	}

	// EOF：检查是否有残余数据（流末尾可能没有 trailing 空行）
	if s.dataBuf.Len() > 0 {
		evType, result, isDone := s.dispatch()
		if isDone {
			return "", nil, true, nil
		}
		if result != nil {
			// 成功分派最后一帧
			s.err = io.EOF
			return evType, result, false, nil
		}
	}

	// 无残余或残余无效 → 正常 EOF
	s.err = io.EOF
	return "", nil, false, io.EOF
}

// dispatch 分派当前缓冲区中累积的事件数据。
//
// 返回值：
//   - eventType: 当前事件类型（从 event: 行获取）
//   - data: 有效数据（json.RawMessage 副本）；nil 表示跳过
//   - done: [DONE] 哨兵检测到时为 true
//
// 分派后重置 dataBuf 和 eventType。
func (s *SSEScanner) dispatch() (eventType string, data json.RawMessage, done bool) {
	evType := s.eventType
	content := s.dataBuf.Bytes()

	// 重置缓冲（无论结果如何，当前帧已经处理完毕）
	defer func() {
		s.dataBuf.Reset()
		s.eventType = ""
	}()

	// [DONE] 哨兵检测
	if bytes.Equal(bytes.TrimSpace(content), []byte("[DONE]")) {
		s.done = true
		return "", nil, true
	}

	// 判断数据是否看起来像 JSON（以 { 或 [ 开头）
	trimmed := bytes.TrimLeft(content, " \t\r\n")
	looksLikeJSON := len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')

	if looksLikeJSON {
		// JSON 完整性校验
		if !json.Valid(content) {
			// GLM Issue #66 容错：截断/粘连帧，跳过并继续
			log.Printf("[SSEScanner] 跳过无效 JSON 帧（长度 %d）: %s", len(content), truncateForLog(content, 200))
			return "", nil, false
		}
	}

	// 有效数据：拷贝副本返回（dataBuf.Reset 后原底层数组会被复用）
	result := make(json.RawMessage, len(content))
	copy(result, content)
	return evType, result, false
}

// Close 关闭底层 ReadCloser，释放连接资源。
//
// 应在 Next() 返回 done=true 或 err 非 nil 后调用。
// 重复调用安全（底层 rc 可能已关闭，错误被忽略）。
func (s *SSEScanner) Close() error {
	if s.rc != nil {
		return s.rc.Close()
	}
	return nil
}

// Err 返回终态错误。
//
// io.EOF 表示流正常耗尽（调用方应将其视为正常结束，而非错误）。
// 其他非 nil 错误表示底层读取异常。
func (s *SSEScanner) Err() error {
	return s.err
}

// truncateForLog 截断日志输出，防止超长无效 JSON 刷屏。
func truncateForLog(data []byte, maxLen int) string {
	if len(data) <= maxLen {
		return string(data)
	}
	return string(data[:maxLen]) + "...(truncated)"
}

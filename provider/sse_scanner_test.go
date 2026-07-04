package provider

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// newTestScanner 从字符串创建 SSEScanner（测试辅助）。
// 使用 io.NopCloser 包装 strings.Reader 模拟 ReadCloser。
func newTestScanner(input string) *SSEScanner {
	return NewSSEScanner(io.NopCloser(strings.NewReader(input)))
}

// readAll 读取 SSEScanner 直到终态，收集所有事件。
// 返回事件列表、是否收到 [DONE]、终态错误。
func readAll(t *testing.T, s *SSEScanner) (events []struct {
	EventType string
	Data      json.RawMessage
}, done bool, err error) {
	t.Helper()
	for {
		evType, data, isDone, e := s.Next()
		if isDone {
			done = true
			return
		}
		if e != nil {
			err = e
			return
		}
		events = append(events, struct {
			EventType string
			Data      json.RawMessage
		}{evType, data})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 测试用例
// ──────────────────────────────────────────────────────────────────────────

// TestSSEScanner_NormalFrames 正常 SSE 帧解析。
func TestSSEScanner_NormalFrames(t *testing.T) {
	input := "data: {\"a\":1}\n\ndata: {\"b\":2}\n\n"
	s := newTestScanner(input)

	events, done, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}
	if !done {
		// 正常耗尽为 EOF，非 [DONE]
	}

	if len(events) != 2 {
		t.Fatalf("期望 2 个事件，实际 %d", len(events))
	}

	// 验证第一帧
	var v1 map[string]int
	if err := json.Unmarshal(events[0].Data, &v1); err != nil {
		t.Fatalf("第一帧 JSON 反序列化失败: %v", err)
	}
	if v1["a"] != 1 {
		t.Errorf("第一帧: 期望 a=1，实际 %d", v1["a"])
	}

	// 验证第二帧
	var v2 map[string]int
	if err := json.Unmarshal(events[1].Data, &v2); err != nil {
		t.Fatalf("第二帧 JSON 反序列化失败: %v", err)
	}
	if v2["b"] != 2 {
		t.Errorf("第二帧: 期望 b=2，实际 %d", v2["b"])
	}
}

// TestSSEScanner_BrokenFrameSkip 损坏 JSON 帧跳过（GLM Issue #66 核心场景）。
func TestSSEScanner_BrokenFrameSkip(t *testing.T) {
	// 第一帧 JSON 被截断（不完整），第二帧完整
	input := "data: {\"id\":\"123\",\"object\":\"chat.c\n\ndata: {\"id\":\"456\",\"content\":\"hi\"}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("期望 1 个事件（损坏帧应被跳过），实际 %d", len(events))
	}

	// 验证唯一返回的帧是完整的第二帧
	var v map[string]string
	if err := json.Unmarshal(events[0].Data, &v); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if v["id"] != "456" {
		t.Errorf("期望 id=456，实际 %s", v["id"])
	}
	if v["content"] != "hi" {
		t.Errorf("期望 content=hi，实际 %s", v["content"])
	}
}

// TestSSEScanner_EventType event: 行的事件类型传递。
func TestSSEScanner_EventType(t *testing.T) {
	input := "event: content_block_delta\ndata: {\"type\":\"text_delta\"}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("期望 1 个事件，实际 %d", len(events))
	}

	if events[0].EventType != "content_block_delta" {
		t.Errorf("期望 eventType=content_block_delta，实际 %q", events[0].EventType)
	}

	var v map[string]string
	if err := json.Unmarshal(events[0].Data, &v); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if v["type"] != "text_delta" {
		t.Errorf("期望 type=text_delta，实际 %s", v["type"])
	}
}

// TestSSEScanner_DoneSentinel [DONE] 哨兵检测。
func TestSSEScanner_DoneSentinel(t *testing.T) {
	input := "data: {\"a\":1}\n\ndata: [DONE]\n\n"
	s := newTestScanner(input)

	events, done, err := readAll(t, s)
	// done=true 时不应有错误
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("期望 1 个正常事件，实际 %d", len(events))
	}

	var v map[string]int
	if err := json.Unmarshal(events[0].Data, &v); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if v["a"] != 1 {
		t.Errorf("期望 a=1，实际 %d", v["a"])
	}

	if !done {
		t.Error("期望收到 [DONE] 哨兵（done=true），实际 done=false")
	}
}

// TestSSEScanner_MultiLineData 多行 data: 按 \n 拼接。
func TestSSEScanner_MultiLineData(t *testing.T) {
	input := "data: line1\ndata: line2\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("期望 1 个事件，实际 %d", len(events))
	}

	// 多行 data 按 \n 拼接后应等于 "line1\nline2"
	expected := "line1\nline2"
	if string(events[0].Data) != expected {
		t.Errorf("期望拼接结果 %q，实际 %q", expected, string(events[0].Data))
	}
}

// TestSSEScanner_CommentSkip SSE 注释行（: keep-alive）跳过。
func TestSSEScanner_CommentSkip(t *testing.T) {
	input := ": keep-alive\ndata: {\"ok\":true}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("期望 1 个事件（注释应被跳过），实际 %d", len(events))
	}

	var v map[string]bool
	if err := json.Unmarshal(events[0].Data, &v); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if !v["ok"] {
		t.Error("期望 ok=true，实际 false")
	}
}

// TestSSEScanner_EmptyInput 空输入直接 EOF。
func TestSSEScanner_EmptyInput(t *testing.T) {
	s := newTestScanner("")

	events, done, err := readAll(t, s)

	if len(events) != 0 {
		t.Errorf("期望 0 个事件，实际 %d", len(events))
	}
	if done {
		t.Error("空输入不应收到 [DONE]")
	}
	if err == nil {
		t.Error("期望返回 io.EOF")
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("期望 io.EOF，实际 %v", err)
	}
}

// TestSSEScanner_AnthropicSequence 完整 Anthropic 风格事件序列。
func TestSSEScanner_AnthropicSequence(t *testing.T) {
	input := strings.Join([]string{
		"event: message_start",
		"data: {\"type\":\"message_start\"}",
		"",
		"event: content_block_delta",
		"data: {\"type\":\"text_delta\",\"text\":\"hello\"}",
		"",
		"event: message_stop",
		"data: {\"type\":\"message_stop\"}",
		"",
		"",
	}, "\n")
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("期望 3 个事件，实际 %d", len(events))
	}

	// 验证第一个事件
	if events[0].EventType != "message_start" {
		t.Errorf("事件 1: 期望 eventType=message_start，实际 %q", events[0].EventType)
	}
	var v1 map[string]string
	if err := json.Unmarshal(events[0].Data, &v1); err != nil {
		t.Fatalf("事件 1 JSON 反序列化失败: %v", err)
	}
	if v1["type"] != "message_start" {
		t.Errorf("事件 1: 期望 type=message_start，实际 %s", v1["type"])
	}

	// 验证第二个事件
	if events[1].EventType != "content_block_delta" {
		t.Errorf("事件 2: 期望 eventType=content_block_delta，实际 %q", events[1].EventType)
	}
	var v2 map[string]string
	if err := json.Unmarshal(events[1].Data, &v2); err != nil {
		t.Fatalf("事件 2 JSON 反序列化失败: %v", err)
	}
	if v2["type"] != "text_delta" {
		t.Errorf("事件 2: 期望 type=text_delta，实际 %s", v2["type"])
	}
	if v2["text"] != "hello" {
		t.Errorf("事件 2: 期望 text=hello，实际 %s", v2["text"])
	}

	// 验证第三个事件
	if events[2].EventType != "message_stop" {
		t.Errorf("事件 3: 期望 eventType=message_stop，实际 %q", events[2].EventType)
	}
	var v3 map[string]string
	if err := json.Unmarshal(events[2].Data, &v3); err != nil {
		t.Fatalf("事件 3 JSON 反序列化失败: %v", err)
	}
	if v3["type"] != "message_stop" {
		t.Errorf("事件 3: 期望 type=message_stop，实际 %s", v3["type"])
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 补充边界测试
// ──────────────────────────────────────────────────────────────────────────

// TestSSEScanner_DoneAfterDone 终态保护：收到 [DONE] 后再调 Next 返回相同状态。
func TestSSEScanner_DoneAfterDone(t *testing.T) {
	input := "data: [DONE]\n\n"
	s := newTestScanner(input)

	// 第一次：done
	_, _, done1, err1 := s.Next()
	if err1 != nil {
		t.Fatalf("第一次 Next 错误: %v", err1)
	}
	if !done1 {
		t.Fatal("期望第一次 done=true")
	}

	// 第二次：应返回相同终态（done=true，不 panic）
	_, _, done2, err2 := s.Next()
	if err2 != nil {
		t.Fatalf("第二次 Next 错误: %v", err2)
	}
	if !done2 {
		t.Fatal("期望第二次 done=true（终态保护）")
	}
}

// TestSSEScanner_UnknownFieldsSkip id: 和 retry: 等未知字段跳过。
func TestSSEScanner_UnknownFieldsSkip(t *testing.T) {
	input := "id: 123\nretry: 5000\ndata: {\"ok\":true}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("期望 1 个事件（id/retry 应跳过），实际 %d", len(events))
	}
}

// TestSSEScanner_DataLeadingSpace data: 后前导空格正确移除。
func TestSSEScanner_DataLeadingSpace(t *testing.T) {
	input := "data: {\"a\":1}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("期望 1 个事件，实际 %d", len(events))
	}
	if string(events[0].Data) != "{\"a\":1}" {
		t.Errorf("前导空格未正确移除，实际 %q", string(events[0].Data))
	}
}

// TestSSEScanner_EOFWithoutTrailingNewline 流末尾无 trailing 空行，残余数据仍被分派。
func TestSSEScanner_EOFWithoutTrailingNewline(t *testing.T) {
	// 最后一个 data 帧后没有 \n\n 结束
	input := "data: {\"a\":1}\n\ndata: {\"b\":2}"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("期望 2 个事件（含末尾无空行残余），实际 %d", len(events))
	}
}

// ──────────────────────────────────────────────────────────────────────────
// GLM Issue #66 帧粘连容错测试
// ──────────────────────────────────────────────────────────────────────────

// TestSSEScanner_FrameConcatenationRecovery GLM Issue #66 核心场景：
// 两个 data: 行粘连（无空行分隔），合并后 JSON 无效，应拆分恢复两个有效帧。
func TestSSEScanner_FrameConcatenationRecovery(t *testing.T) {
	// 模拟 GLM 帧粘连：两个 data: 行之间没有空行
	input := "data: {\"id\":\"1\",\"content\":\"hello\"}\ndata: {\"id\":\"2\",\"content\":\"world\"}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("期望 2 个事件（粘连帧拆分恢复），实际 %d", len(events))
	}

	var v1, v2 map[string]string
	if err := json.Unmarshal(events[0].Data, &v1); err != nil {
		t.Fatalf("第一帧 JSON 反序列化失败: %v", err)
	}
	if v1["id"] != "1" || v1["content"] != "hello" {
		t.Errorf("第一帧: 期望 id=1 content=hello，实际 id=%s content=%s", v1["id"], v1["content"])
	}

	if err := json.Unmarshal(events[1].Data, &v2); err != nil {
		t.Fatalf("第二帧 JSON 反序列化失败: %v", err)
	}
	if v2["id"] != "2" || v2["content"] != "world" {
		t.Errorf("第二帧: 期望 id=2 content=world，实际 id=%s content=%s", v2["id"], v2["content"])
	}
}

// TestSSEScanner_FrameConcatenationThreeFrames 三帧粘连场景。
func TestSSEScanner_FrameConcatenationThreeFrames(t *testing.T) {
	input := "data: {\"i\":1}\ndata: {\"i\":2}\ndata: {\"i\":3}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("期望 3 个事件（三帧粘连拆分），实际 %d", len(events))
	}

	for i, ev := range events {
		var v map[string]int
		if err := json.Unmarshal(ev.Data, &v); err != nil {
			t.Fatalf("帧 %d JSON 反序列化失败: %v", i, err)
		}
		if v["i"] != i+1 {
			t.Errorf("帧 %d: 期望 i=%d，实际 i=%d", i, i+1, v["i"])
		}
	}
}

// TestSSEScanner_FrameConcatenationWithTruncatedFirst 首帧截断 + 次帧完整。
// GLM Issue #66 原始场景：第一个 JSON 在字段中间被截断。
func TestSSEScanner_FrameConcatenationWithTruncatedFirst(t *testing.T) {
	// 第一个 data: 行的 JSON 被截断（不完整），第二个 data: 行完整
	input := "data: {\"id\":\"123\",\"object\":\"chat.c\ndata: {\"id\":\"456\",\"content\":\"hi\"}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	// 截断的第一帧无法恢复，但完整的第二帧应该被恢复
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件（截断帧跳过，完整帧恢复），实际 %d", len(events))
	}

	var v map[string]string
	if err := json.Unmarshal(events[0].Data, &v); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}
	if v["id"] != "456" {
		t.Errorf("期望 id=456，实际 %s", v["id"])
	}
}

// TestSSEScanner_FrameConcatenationBeforeNormalFrame 粘连帧后跟正常帧。
func TestSSEScanner_FrameConcatenationBeforeNormalFrame(t *testing.T) {
	input := "data: {\"a\":1}\ndata: {\"b\":2}\n\ndata: {\"c\":3}\n\n"
	s := newTestScanner(input)

	events, _, err := readAll(t, s)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("意外错误: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("期望 3 个事件（粘连 2 + 正常 1），实际 %d", len(events))
	}

	var v1, v2, v3 map[string]int
	json.Unmarshal(events[0].Data, &v1)
	json.Unmarshal(events[1].Data, &v2)
	json.Unmarshal(events[2].Data, &v3)

	if v1["a"] != 1 || v2["b"] != 2 || v3["c"] != 3 {
		t.Errorf("帧顺序或内容错误: v1=%v v2=%v v3=%v", v1, v2, v3)
	}
}

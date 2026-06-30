package completions

import (
	"errors"
	"fmt"
	"testing"
)

// TestClassifyStreamError_JSONParseMidStream 验证流中途收到畸形 JSON 时，
// classifyStreamError 将其归类为 errKindJSONParse（可降级处理），
// 而非 errKindFatal（直接报错终止）。
//
// 场景：GLM Coding-MAX Issue #66 — 两帧粘连导致 JSON 截断。
// 此时 startSent=true（已输出内容），stopSent=false（finish_reason 未到达）。
// 期望：归类为 JSON 解析错误，调用方可合成 Stop 事件优雅降级。
func TestClassifyStreamError_JSONParseMidStream(t *testing.T) {
	// 模拟 openai-go SDK json.Unmarshal 失败的真实错误
	jsonErr := fmt.Errorf("invalid character 'd' looking for beginning of value")

	kind := classifyStreamError(jsonErr)

	if kind != errKindJSONParse {
		t.Errorf("classifyStreamError(json error) = %v, want errKindJSONParse", kind)
	}
}

// TestClassifyStreamError_JSONParseUnexpectedEnd 验证 "unexpected end of JSON input"
// 也被正确归类为 JSON 解析错误。
//
// 场景：openai-go SDK Issue #556/#618 — 空 Data 事件触发。
func TestClassifyStreamError_JSONParseUnexpectedEnd(t *testing.T) {
	jsonErr := errors.New("unexpected end of JSON input")

	kind := classifyStreamError(jsonErr)

	if kind != errKindJSONParse {
		t.Errorf("classifyStreamError(unexpected end) = %v, want errKindJSONParse", kind)
	}
}

// TestClassifyStreamError_NetworkError 验证网络层错误（非 JSON 解析）
// 被归类为 errKindFatal，不做降级。
//
// 场景：TCP 连接重置、io.EOF 异常等真正的网络故障。
func TestClassifyStreamError_NetworkError(t *testing.T) {
	netErr := errors.New("read tcp 127.0.0.1:1234->10.0.0.1:443: read: connection reset by peer")

	kind := classifyStreamError(netErr)

	if kind != errKindFatal {
		t.Errorf("classifyStreamError(network error) = %v, want errKindFatal", kind)
	}
}

// TestClassifyStreamError_StreamError 验证 openai-go SDK 的 *ssestream.StreamError
// （含 "received error while streaming" 前缀）被归类为 errKindFatal。
//
// 场景：上游通过 SSE error 事件主动报错。
func TestClassifyStreamError_StreamError(t *testing.T) {
	streamErr := fmt.Errorf("received error while streaming: internal server error")

	kind := classifyStreamError(streamErr)

	if kind != errKindFatal {
		t.Errorf("classifyStreamError(stream error) = %v, want errKindFatal", kind)
	}
}

// TestClassifyStreamError_NilError 验证 nil 错误归类为 errKindNone。
func TestClassifyStreamError_NilError(t *testing.T) {
	kind := classifyStreamError(nil)

	if kind != errKindNone {
		t.Errorf("classifyStreamError(nil) = %v, want errKindNone", kind)
	}
}

// TestClassifyStreamError_ContextCanceled 验证 context.Canceled 归类为 errKindFatal。
//
// 场景：客户端主动取消请求（非 JSON 解析问题）。
func TestClassifyStreamError_ContextCanceled(t *testing.T) {
	// 模拟 context.Canceled 的错误消息
	cancelErr := errors.New("context canceled")

	kind := classifyStreamError(cancelErr)

	if kind != errKindFatal {
		t.Errorf("classifyStreamError(context canceled) = %v, want errKindFatal", kind)
	}
}

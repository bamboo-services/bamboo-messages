package relay

import (
	"strings"
	"testing"
)

// ════════════════════════════════════════════════════════════
// FormatRelayResponse 测试
// ════════════════════════════════════════════════════════════

// TestFormatRelayResponse_NonStream 验证非流式响应格式化包含关键信息。
func TestFormatRelayResponse_NonStream(t *testing.T) {
	got := FormatRelayResponse("Relay", "openai", "anthropic", []byte(`{"id":"msg_001","content":"hello"}`))
	for _, want := range []string{"Relay", "openai", "anthropic", "msg_001"} {
		if !strings.Contains(got, want) {
			t.Errorf("FormatRelayResponse() = %q, want contains %q", got, want)
		}
	}
}

// TestFormatRelayResponse_EmptyBody 验证空 body 不会 panic。
func TestFormatRelayResponse_EmptyBody(t *testing.T) {
	got := FormatRelayResponse("Relay", "openai", "anthropic", []byte{})
	if got == "" {
		t.Error("FormatRelayResponse() with empty body returned empty string")
	}
}

// TestFormatRelayResponse_BodyTruncation 验证超长 body 会被截断。
func TestFormatRelayResponse_BodyTruncation(t *testing.T) {
	longBody := []byte(strings.Repeat("a", maxDebugBodyLen+100))
	got := FormatRelayResponse("Relay", "openai", "anthropic", longBody)
	if !strings.Contains(got, "...(truncated)") {
		t.Errorf("FormatRelayResponse() = %q, want truncation marker", got)
	}
}

// ════════════════════════════════════════════════════════════
// FormatRelayResponseFrame 测试
// ════════════════════════════════════════════════════════════

// TestFormatRelayResponseFrame_Stream 验证流式逐帧响应格式化包含关键信息。
func TestFormatRelayResponseFrame_Stream(t *testing.T) {
	data := []byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\"}")
	got := FormatRelayResponseFrame("RelayStream", "openai", "anthropic", data)
	for _, want := range []string{"RelayStream", "openai", "anthropic", "content_block_delta"} {
		if !strings.Contains(got, want) {
			t.Errorf("FormatRelayResponseFrame() = %q, want contains %q", got, want)
		}
	}
}

// TestFormatRelayResponseFrame_EmptyData 验证空 data 不会 panic。
func TestFormatRelayResponseFrame_EmptyData(t *testing.T) {
	got := FormatRelayResponseFrame("RelayStream", "openai", "anthropic", []byte{})
	if got == "" {
		t.Error("FormatRelayResponseFrame() with empty data returned empty string")
	}
}

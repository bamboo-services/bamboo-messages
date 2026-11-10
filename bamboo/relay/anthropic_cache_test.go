package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo"
	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	"github.com/bamboo-services/bamboo-messages/provider/anthropic"
)

// anthropicCacheSSE 模拟 Anthropic 官方流式响应。
//
// 关键点：Anthropic 把 usage 拆成两处——message_start 携带 input_tokens 与缓存统计，
// message_delta 顶层携带最终 output_tokens。
const anthropicCacheSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-sonnet-4","content":[],"stop_reason":null,"usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":12}}

event: message_stop
data: {"type":"message_stop"}

`

// relayFrame 表示一个解析后的 Anthropic SSE 输出帧。
type relayFrame struct {
	eventType string
	payload   map[string]any
}

// parseRelayFrames 解析 relay 输出的 SSE 帧序列。
func parseRelayFrames(t *testing.T, chunks []string) []relayFrame {
	t.Helper()
	var frames []relayFrame
	for _, chunk := range chunks {
		for _, block := range strings.Split(strings.TrimRight(chunk, "\n"), "\n\n") {
			if strings.TrimSpace(block) == "" {
				continue
			}
			var frame relayFrame
			for _, line := range strings.Split(block, "\n") {
				switch {
				case strings.HasPrefix(line, "event: "):
					frame.eventType = strings.TrimPrefix(line, "event: ")
				case strings.HasPrefix(line, "data: "):
					data := strings.TrimPrefix(line, "data: ")
					if err := json.Unmarshal([]byte(data), &frame.payload); err != nil {
						t.Fatalf("failed to unmarshal SSE data: %v\nraw: %s", err, data)
					}
				}
			}
			if frame.eventType == "" {
				t.Fatalf("missing event: line in SSE: %q", block)
			}
			frames = append(frames, frame)
		}
	}
	return frames
}

// findFrame 查找指定类型的首个帧。
func findFrame(frames []relayFrame, eventType string) (relayFrame, bool) {
	for _, frame := range frames {
		if frame.eventType == eventType {
			return frame, true
		}
	}
	return relayFrame{}, false
}

// TestRelayStream_AnthropicCacheUsageInWireFrames 验证 relay 输出的 Anthropic 帧携带缓存统计。
//
// 回归场景：
//   - message_start 帧此前 usage 为空，下游按 Anthropic 惯例从 message_start 读取
//     输入与缓存统计时只能拿到 0，表现为「缓存无法记录」。
//   - message_delta 帧此前丢失最终 output_tokens。
func TestRelayStream_AnthropicCacheUsageInWireFrames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(anthropicCacheSSE))
	}))
	defer srv.Close()

	p := anthropic.NewProviderWithOptions(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(srv.URL),
	)

	var usages []bamboo.Usage
	body := `{"model":"claude-sonnet-4","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`

	ch, err := RelayStream(context.Background(), p, []byte(body),
		codec.FormatAnthropic, codec.FormatAnthropic,
		WithUsageCallback(func(u bamboo.Usage) {
			usages = append(usages, u)
		}))
	if err != nil {
		t.Fatalf("RelayStream() error = %v", err)
	}

	var chunks []string
	for chunk := range ch {
		chunks = append(chunks, string(chunk))
	}
	frames := parseRelayFrames(t, chunks)

	if len(frames) == 0 {
		t.Fatal("expected at least one output frame")
	}
	if frames[0].eventType != "message_start" {
		t.Fatalf("first frame = %q, want %q", frames[0].eventType, "message_start")
	}

	// 1. message_start 帧必须携带输入与缓存统计
	msg, ok := frames[0].payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message field should be object")
	}
	startUsage, ok := msg["usage"].(map[string]any)
	if !ok {
		t.Fatalf("message.usage should be object")
	}
	if in, ok := startUsage["input_tokens"].(float64); !ok || in != 100 {
		t.Errorf("message_start input_tokens = %v, want 100", startUsage["input_tokens"])
	}
	if cc, ok := startUsage["cache_creation_input_tokens"].(float64); !ok || cc != 50 {
		t.Errorf("message_start cache_creation_input_tokens = %v, want 50", startUsage["cache_creation_input_tokens"])
	}
	if cr, ok := startUsage["cache_read_input_tokens"].(float64); !ok || cr != 25 {
		t.Errorf("message_start cache_read_input_tokens = %v, want 25", startUsage["cache_read_input_tokens"])
	}

	// 2. message_delta 帧必须携带最终 output_tokens 与缓存统计
	deltaFrame, ok := findFrame(frames, "message_delta")
	if !ok {
		t.Fatal("expected message_delta frame")
	}
	deltaUsage, ok := deltaFrame.payload["usage"].(map[string]any)
	if !ok {
		t.Fatalf("message_delta usage should be object")
	}
	if out, ok := deltaUsage["output_tokens"].(float64); !ok || out != 12 {
		t.Errorf("message_delta output_tokens = %v, want 12", deltaUsage["output_tokens"])
	}
	if in, ok := deltaUsage["input_tokens"].(float64); !ok || in != 100 {
		t.Errorf("message_delta input_tokens = %v, want 100", deltaUsage["input_tokens"])
	}
	if cc, ok := deltaUsage["cache_creation_input_tokens"].(float64); !ok || cc != 50 {
		t.Errorf("message_delta cache_creation_input_tokens = %v, want 50", deltaUsage["cache_creation_input_tokens"])
	}
	if cr, ok := deltaUsage["cache_read_input_tokens"].(float64); !ok || cr != 25 {
		t.Errorf("message_delta cache_read_input_tokens = %v, want 25", deltaUsage["cache_read_input_tokens"])
	}

	// 3. usage 回调必须收到完整的累计用量
	if len(usages) == 0 {
		t.Fatal("expected usage callback")
	}
	last := usages[len(usages)-1]
	if last.InputTokens != 175 {
		t.Errorf("usage InputTokens = %d, want 175", last.InputTokens)
	}
	if last.OutputTokens != 12 {
		t.Errorf("usage OutputTokens = %d, want 12", last.OutputTokens)
	}
	if last.CacheCreationInputTokens != 50 {
		t.Errorf("usage CacheCreationInputTokens = %d, want 50", last.CacheCreationInputTokens)
	}
	if last.CacheReadInputTokens != 25 {
		t.Errorf("usage CacheReadInputTokens = %d, want 25", last.CacheReadInputTokens)
	}
}

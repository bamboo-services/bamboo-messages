package relay

import (
	"context"
	"strings"
	"testing"

	"github.com/bamboo-services/bamboo-messages/bamboo/codec"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/anthropic"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/gemini"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/openai"
	_ "github.com/bamboo-services/bamboo-messages/bamboo/codec/responses"
)

// ════════════════════════════════════════════════════════════
// 4×4 交叉互转测试矩阵
// ════════════════════════════════════════════════════════════
//
// 该测试验证所有入站格式 → 所有出站格式的端到端管道通畅性，
// 不做精细的 JSON 结构断言（那由各 codec 自身的测试覆盖）。
//
// 矩阵：4 入站格式 × 4 出站格式 = 16 条路径
// 每条路径覆盖流式 + 非流式两种调用方式 = 32 个测试
// 另加 4 条工具调用跨格式路径 = 36 个测试总计

// ── 各格式最小合法请求 JSON ──

var (
	// openaiBody OpenAI Chat Completions 最小请求
	openaiBody = []byte(`{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`)

	// anthropicBody Anthropic Messages 最小请求（max_tokens 必填）
	anthropicBody = []byte(`{"model":"test-model","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}`)

	// responsesBody OpenAI Responses 最小请求（input 为 string 简化形式）
	responsesBody = []byte(`{"model":"test-model","input":"hello"}`)

	// geminiBody Google Gemini GenerateContent 最小请求
	geminiBody = []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)

	// openaiStreamBody 流式变体
	openaiStreamBody    = []byte(`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	anthropicStreamBody = []byte(`{"model":"test-model","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	responsesStreamBody = []byte(`{"model":"test-model","stream":true,"input":"hello"}`)
	geminiStreamBody    = geminiBody // Gemini 流由 URL 参数决定，body 一致
)

// ── 各格式带工具的请求 JSON（用于工具调用跨格式测试）──

var (
	openaiToolsBody = []byte(`{"model":"test-model","messages":[{"role":"user","content":"what's the weather?"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]}`)

	anthropicToolsBody = []byte(`{"model":"test-model","max_tokens":1024,"messages":[{"role":"user","content":"what's the weather?"}],"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`)

	responsesToolsBody = []byte(`{"model":"test-model","input":"what's the weather?","tools":[{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`)

	geminiToolsBody = []byte(`{"contents":[{"role":"user","parts":[{"text":"what's the weather?"}]}],"tools":[{"functionDeclarations":[{"name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}]}`)
)

// crossFormatCase 单条交叉测试路径定义。
type crossFormatCase struct {
	name      string
	inFormat  codec.FormatType
	outFormat codec.FormatType
	body      []byte
	stream    bool
}

// buildMatrix 构建 4×4 = 16 条路径的完整矩阵，每条路径含流式 + 非流式。
func buildMatrix() []crossFormatCase {
	// 入站格式 → (非流式 body, 流式 body)
	type formatBodies struct {
		fmt     codec.FormatType
		regular []byte
		stream  []byte
	}

	all := []formatBodies{
		{codec.FormatOpenAI, openaiBody, openaiStreamBody},
		{codec.FormatAnthropic, anthropicBody, anthropicStreamBody},
		{codec.FormatResponses, responsesBody, responsesStreamBody},
		{codec.FormatGemini, geminiBody, geminiStreamBody},
	}

	// 出站格式简称映射（仅用于测试名）
	outNames := map[codec.FormatType]string{
		codec.FormatOpenAI:    "OpenAI",
		codec.FormatAnthropic: "Anthropic",
		codec.FormatResponses: "Responses",
		codec.FormatGemini:    "Gemini",
	}
	inNames := map[codec.FormatType]string{
		codec.FormatOpenAI:    "OpenAI",
		codec.FormatAnthropic: "Anthropic",
		codec.FormatResponses: "Responses",
		codec.FormatGemini:    "Gemini",
	}

	var cases []crossFormatCase
	for _, in := range all {
		for _, out := range all {
			// 非流式
			cases = append(cases, crossFormatCase{
				name:      inNames[in.fmt] + "_to_" + outNames[out.fmt] + "_NonStream",
				inFormat:  in.fmt,
				outFormat: out.fmt,
				body:      in.regular,
				stream:    false,
			})
			// 流式
			cases = append(cases, crossFormatCase{
				name:      inNames[in.fmt] + "_to_" + outNames[out.fmt] + "_Stream",
				inFormat:  in.fmt,
				outFormat: out.fmt,
				body:      in.stream,
				stream:    true,
			})
		}
	}
	return cases
}

// TestCrossFormat_AllPaths 验证 4×4 = 16 条路径 × 2（流式+非流式）= 32 个测试。
//
// 断言策略：
//   - 非流式：result 非空、无错误、可被 JSON 解析（管道通畅）
//   - 流式：channel 正常关闭、收集到至少一个非空数据帧（管道通畅）
func TestCrossFormat_AllPaths(t *testing.T) {
	cases := buildMatrix()

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.stream {
				testCrossFormatStream(t, tt)
			} else {
				testCrossFormatNonStream(t, tt)
			}
		})
	}
}

// testCrossFormatNonStream 验证非流式路径：Relay() 管道通畅。
func testCrossFormatNonStream(t *testing.T, tt crossFormatCase) {
	t.Helper()
	mp := newMockCompleteProvider("Hello from mock", 10, 5)

	result, err := Relay(context.Background(), mp, tt.body, tt.inFormat, tt.outFormat)
	if err != nil {
		t.Fatalf("Relay(%s→%s) error: %v", tt.inFormat, tt.outFormat, err)
	}
	if len(result) == 0 {
		t.Fatalf("Relay(%s→%s) returned empty result", tt.inFormat, tt.outFormat)
	}

	// 通用断言：输出应是合法 JSON（以 { 开头）
	outStr := string(result)
	if !strings.HasPrefix(strings.TrimSpace(outStr), "{") {
		t.Errorf("Relay(%s→%s) output is not valid JSON, got: %.200s", tt.inFormat, tt.outFormat, outStr)
	}

	// 通用断言：响应文本应在输出中出现
	if !strings.Contains(outStr, "Hello from mock") {
		t.Errorf("Relay(%s→%s) output missing response text, got: %.200s", tt.inFormat, tt.outFormat, outStr)
	}
}

// testCrossFormatStream 验证流式路径：RelayStream() 管道通畅。
func testCrossFormatStream(t *testing.T, tt crossFormatCase) {
	t.Helper()
	mp := newMockStreamProvider([]string{"Hello", " from", " mock"}, 10, 5)

	ch, err := RelayStream(context.Background(), mp, tt.body, tt.inFormat, tt.outFormat)
	if err != nil {
		t.Fatalf("RelayStream(%s→%s) error: %v", tt.inFormat, tt.outFormat, err)
	}

	var output strings.Builder
	frameCount := 0
	for data := range ch {
		if len(data) > 0 {
			frameCount++
		}
		output.Write(data)
	}

	if frameCount == 0 {
		t.Errorf("RelayStream(%s→%s) produced no data frames", tt.inFormat, tt.outFormat)
	}

	// 通用断言：流输出中应包含 mock 文本内容
	result := output.String()
	if !strings.Contains(result, "Hello") && !strings.Contains(result, "mock") {
		t.Errorf("RelayStream(%s→%s) output missing text content, got: %.200s", tt.inFormat, tt.outFormat, result)
	}
}

// ════════════════════════════════════════════════════════════
// 工具调用跨格式转换测试（4 条额外路径）
// ════════════════════════════════════════════════════════════

// TestCrossFormat_ToolCalls 验证携带工具定义的请求能跨格式正常解析和转发。
//
// 仅验证非流式管道通畅（工具调用参数的精确转换由各 codec 测试覆盖）。
// 测试 mock provider 收到的 config.Tools 不为空。
func TestCrossFormat_ToolCalls(t *testing.T) {
	toolCases := []struct {
		name      string
		inFormat  codec.FormatType
		outFormat codec.FormatType
		body      []byte
	}{
		{"OpenAI_to_Anthropic_Tools", codec.FormatOpenAI, codec.FormatAnthropic, openaiToolsBody},
		{"Anthropic_to_OpenAI_Tools", codec.FormatAnthropic, codec.FormatOpenAI, anthropicToolsBody},
		{"Responses_to_Anthropic_Tools", codec.FormatResponses, codec.FormatAnthropic, responsesToolsBody},
		{"Gemini_to_OpenAI_Tools", codec.FormatGemini, codec.FormatOpenAI, geminiToolsBody},
	}

	for _, tt := range toolCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mp := newMockCompleteProvider("tool result", 10, 5)

			result, err := Relay(context.Background(), mp, tt.body, tt.inFormat, tt.outFormat)
			if err != nil {
				t.Fatalf("Relay() error: %v", err)
			}
			if len(result) == 0 {
				t.Fatal("Relay() returned empty result")
			}

			// 验证工具定义被正确解析并传递给 provider
			if mp.lastConfig == nil {
				t.Fatal("expected non-nil lastConfig on mock provider")
			}
			if len(mp.lastConfig.Tools) == 0 {
				t.Errorf("expected tools to be parsed from %s request, got 0 tools", tt.inFormat)
			}
		})
	}
}

// ════════════════════════════════════════════════════════════
// 透传验证（同格式入出）
// ════════════════════════════════════════════════════════════

// TestCrossFormat_PassthroughIntegrity 同格式入出时，关键字段应保持一致。
//
// 这是对矩阵中 4 条透传路径（OAI→OAI / ANT→ANT / RESP→RESP / GEM→GEM）
// 的增强验证：确保 relay 层没有意外丢失或篡改字段。
func TestCrossFormat_PassthroughIntegrity(t *testing.T) {
	t.Run("OpenAI_Passthrough", func(t *testing.T) {
		mp := newMockCompleteProvider("passthrough test", 10, 20)
		out, err := Relay(context.Background(), mp, openaiBody, codec.FormatOpenAI, codec.FormatOpenAI)
		if err != nil {
			t.Fatalf("Relay error: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, `"object":"chat.completion"`) {
			t.Errorf("OpenAI passthrough missing chat.completion object, got: %s", s)
		}
		if !strings.Contains(s, "passthrough test") {
			t.Errorf("OpenAI passthrough missing response text, got: %s", s)
		}
	})

	t.Run("Anthropic_Passthrough", func(t *testing.T) {
		mp := newMockCompleteProvider("passthrough test", 10, 20)
		out, err := Relay(context.Background(), mp, anthropicBody, codec.FormatAnthropic, codec.FormatAnthropic)
		if err != nil {
			t.Fatalf("Relay error: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, `"type":"message"`) {
			t.Errorf("Anthropic passthrough missing message type, got: %s", s)
		}
		if !strings.Contains(s, "passthrough test") {
			t.Errorf("Anthropic passthrough missing response text, got: %s", s)
		}
	})

	t.Run("Responses_Passthrough", func(t *testing.T) {
		mp := newMockCompleteProvider("passthrough test", 10, 20)
		out, err := Relay(context.Background(), mp, responsesBody, codec.FormatResponses, codec.FormatResponses)
		if err != nil {
			t.Fatalf("Relay error: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "passthrough test") {
			t.Errorf("Responses passthrough missing response text, got: %s", s)
		}
	})

	t.Run("Gemini_Passthrough", func(t *testing.T) {
		mp := newMockCompleteProvider("passthrough test", 10, 20)
		out, err := Relay(context.Background(), mp, geminiBody, codec.FormatGemini, codec.FormatGemini)
		if err != nil {
			t.Fatalf("Relay error: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "passthrough test") {
			t.Errorf("Gemini passthrough missing response text, got: %s", s)
		}
	})
}

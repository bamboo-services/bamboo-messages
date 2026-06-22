package provider

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// ────────────────────────────────────────────────────────
// summarizeTools 测试
// ────────────────────────────────────────────────────────

// TestSummarizeTools_AnthropicFlat 测试 Anthropic 扁平格式 tools 的简化行为。
//
// 第一个 tool 完整保留，后续 tool 的 description/input_schema 被摘要。
func TestSummarizeTools_AnthropicFlat(t *testing.T) {
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(`{"tools":[{"name":"get_time","description":"获取当前时间","input_schema":{"type":"object","properties":{}}},{"name":"calc","description":"计算器","input_schema":{"type":"object","properties":{}}},{"name":"read_file","description":"读取文件","input_schema":{"type":"object","properties":{}}}]}`), &raw); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	summarizeTools(raw)

	tools, ok := raw["tools"].([]any)
	if !ok {
		t.Fatal("tools 应为 []any 类型")
	}
	if len(tools) != 3 {
		t.Fatalf("tools 长度 = %d, want 3", len(tools))
	}

	// ── tools[0] 完整保留 ──
	tool0, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatal("tools[0] 应为 map[string]any")
	}
	want0 := map[string]any{
		"name":        "get_time",
		"description": "获取当前时间",
		"input_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
	if !reflect.DeepEqual(tool0, want0) {
		t.Errorf("tools[0] 未完整保留\ngot:  %v\nwant: %v", tool0, want0)
	}

	// ── tools[1] description 被摘要 ──
	tool1, ok := tools[1].(map[string]any)
	if !ok {
		t.Fatal("tools[1] 应为 map[string]any")
	}
	if tool1["name"] != "calc" {
		t.Errorf("tools[1].name = %v, want calc", tool1["name"])
	}
	if tool1["description"] != "(9 chars)" {
		t.Errorf("tools[1].description = %v, want (9 chars)", tool1["description"])
	}
	if tool1["input_schema"] != "(2 keys)" {
		t.Errorf("tools[1].input_schema = %v, want (2 keys)", tool1["input_schema"])
	}

	// ── tools[2] description 被摘要 ──
	tool2, ok := tools[2].(map[string]any)
	if !ok {
		t.Fatal("tools[2] 应为 map[string]any")
	}
	if tool2["name"] != "read_file" {
		t.Errorf("tools[2].name = %v, want read_file", tool2["name"])
	}
	if tool2["description"] != "(12 chars)" {
		t.Errorf("tools[2].description = %v, want (12 chars)", tool2["description"])
	}
}

// TestSummarizeTools_OpenAINested 测试 OpenAI 嵌套格式 tools 的简化行为。
//
// function 子对象内的 description/parameters 被摘要，标识字段（name/type）保留。
func TestSummarizeTools_OpenAINested(t *testing.T) {
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(`{"tools":[{"type":"function","function":{"name":"get_time","description":"Get the current time","parameters":{"type":"object","properties":{}}}},{"type":"function","function":{"name":"calc","description":"Calculator tool","parameters":{"type":"object"}}}]}`), &raw); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	summarizeTools(raw)

	tools, ok := raw["tools"].([]any)
	if !ok {
		t.Fatal("tools 应为 []any 类型")
	}
	if len(tools) != 2 {
		t.Fatalf("tools 长度 = %d, want 2", len(tools))
	}

	// ── tools[0] 完整保留 ──
	tool0, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatal("tools[0] 应为 map[string]any")
	}
	fn0, ok := tool0["function"].(map[string]any)
	if !ok {
		t.Fatal("tools[0].function 应为 map[string]any")
	}
	if fn0["name"] != "get_time" {
		t.Errorf("tools[0].function.name = %v, want get_time", fn0["name"])
	}
	if fn0["description"] != "Get the current time" {
		t.Errorf("tools[0].function.description 应完整保留, got %v", fn0["description"])
	}

	// ── tools[1] 外层标识保留 ──
	tool1, ok := tools[1].(map[string]any)
	if !ok {
		t.Fatal("tools[1] 应为 map[string]any")
	}
	if tool1["type"] != "function" {
		t.Errorf("tools[1].type = %v, want function", tool1["type"])
	}

	// ── tools[1].function 内部简化 ──
	fn1, ok := tool1["function"].(map[string]any)
	if !ok {
		t.Fatal("tools[1].function 应为 map[string]any")
	}
	if fn1["name"] != "calc" {
		t.Errorf("tools[1].function.name = %v, want calc", fn1["name"])
	}
	if fn1["description"] != "(15 chars)" {
		t.Errorf("tools[1].function.description = %v, want (15 chars)", fn1["description"])
	}
	if fn1["parameters"] != "(1 keys)" {
		t.Errorf("tools[1].function.parameters = %v, want (1 keys)", fn1["parameters"])
	}
}

// TestSummarizeTools_SingleTool 测试仅有一个 tool 时不简化。
func TestSummarizeTools_SingleTool(t *testing.T) {
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(`{"tools":[{"name":"get_time","description":"获取当前时间","input_schema":{"type":"object","properties":{}}}]}`), &raw); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	// 记录简化前的快照
	toolsBefore, _ := json.Marshal(raw["tools"])

	summarizeTools(raw)

	toolsAfter, _ := json.Marshal(raw["tools"])

	if !reflect.DeepEqual(toolsBefore, toolsAfter) {
		t.Errorf("单 tool 不应被简化\nbefore: %s\nafter:  %s", toolsBefore, toolsAfter)
	}
}

// TestSummarizeTools_NoTools 测试无 tools 键时不 panic 且 map 不变。
func TestSummarizeTools_NoTools(t *testing.T) {
	raw := map[string]any{"model": "gpt-4", "messages": []any{}}

	// 应安全返回，不 panic
	summarizeTools(raw)

	if raw["model"] != "gpt-4" {
		t.Errorf("model 字段不应被修改, got %v", raw["model"])
	}
}

// TestSummarizeTools_EmptyArray 测试 tools 为空数组时不 panic。
func TestSummarizeTools_EmptyArray(t *testing.T) {
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(`{"tools":[]}`), &raw); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	summarizeTools(raw)

	tools, ok := raw["tools"].([]any)
	if !ok {
		t.Fatal("tools 应仍为 []any 类型")
	}
	if len(tools) != 0 {
		t.Errorf("tools 应仍为空数组, 长度 = %d", len(tools))
	}
}

// TestSummarizeTools_NonArrayTools 测试 tools 非数组时不 panic 且不修改值。
func TestSummarizeTools_NonArrayTools(t *testing.T) {
	raw := map[string]any{"tools": "not-an-array"}

	summarizeTools(raw)

	if raw["tools"] != "not-an-array" {
		t.Errorf("tools 非数组时不应被修改, got %v", raw["tools"])
	}
}

// ────────────────────────────────────────────────────────
// summarizeValue 测试
// ────────────────────────────────────────────────────────

// TestSummarizeValue_String 测试字符串值被摘要为 "(N chars)"。
func TestSummarizeValue_String(t *testing.T) {
	got := summarizeValue("description", "hello world")
	want := "(11 chars)"
	if got != want {
		t.Errorf("summarizeValue(description, \"hello world\") = %v, want %v", got, want)
	}
}

// TestSummarizeValue_Map 测试 map 值被摘要为 "(N keys)"。
func TestSummarizeValue_Map(t *testing.T) {
	input := map[string]any{"type": "object", "properties": map[string]any{}}
	got := summarizeValue("input_schema", input)
	want := "(2 keys)"
	if got != want {
		t.Errorf("summarizeValue(input_schema, map) = %v, want %v", got, want)
	}
}

// TestSummarizeValue_Array 测试 array 值被摘要为 "(N items)"。
func TestSummarizeValue_Array(t *testing.T) {
	input := []any{"a", "b", "c"}
	got := summarizeValue("enum", input)
	want := "(3 items)"
	if got != want {
		t.Errorf("summarizeValue(enum, [a b c]) = %v, want %v", got, want)
	}
}

// TestSummarizeValue_Identifier 测试标识字段（name/type）返回原值。
func TestSummarizeValue_Identifier(t *testing.T) {
	tests := []struct {
		key  string
		val  any
		want any
	}{
		{"name", "calculator", "calculator"},
		{"type", "function", "function"},
		{"Name", "Calculator", "Calculator"}, // 大小写不敏感
		{"Type", "Function", "Function"},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+stringOrAny(tt.val), func(t *testing.T) {
			got := summarizeValue(tt.key, tt.val)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("summarizeValue(%q, %v) = %v, want %v", tt.key, tt.val, got, tt.want)
			}
		})
	}
}

// TestSummarizeValue_Primitives 测试原始类型（number/bool/nil）返回原值。
func TestSummarizeValue_Primitives(t *testing.T) {
	// float64（JSON number 解析后为 float64）
	got := summarizeValue("temperature", 0.7)
	if got != 0.7 {
		t.Errorf("summarizeValue(temperature, 0.7) = %v (%T), want 0.7", got, got)
	}

	// bool
	got = summarizeValue("enabled", true)
	if got != true {
		t.Errorf("summarizeValue(enabled, true) = %v, want true", got)
	}

	// nil
	got = summarizeValue("nothing", nil)
	if got != nil {
		t.Errorf("summarizeValue(nothing, nil) = %v, want nil", got)
	}
}

// ────────────────────────────────────────────────────────
// truncateContent 测试
// ────────────────────────────────────────────────────────

// TestTruncateContent_ToolsSimplified 测试端到端 truncateContent 对 tools 的简化。
//
// 构造包含 2+ tools 和普通字段的 JSON，验证 tools 被简化、其他字段不受影响。
func TestTruncateContent_ToolsSimplified(t *testing.T) {
	input := `{"model":"gpt-4","tools":[{"name":"get_time","description":"Get the current time","input_schema":{"type":"object","properties":{}}},{"name":"calc","description":"A calculator tool for math operations","input_schema":{"type":"object","properties":{"expr":{"type":"string"}}}}]}`

	result := truncateContent(input)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("truncateContent 输出不是合法 JSON: %v\n原始输出: %s", err, result)
	}

	// model 字段不受影响
	if parsed["model"] != "gpt-4" {
		t.Errorf("model = %v, want gpt-4", parsed["model"])
	}

	// tools 被解析为数组
	tools, ok := parsed["tools"].([]any)
	if !ok {
		t.Fatal("tools 应为 []any 类型")
	}
	if len(tools) != 2 {
		t.Fatalf("tools 长度 = %d, want 2", len(tools))
	}

	// tools[0] 完整保留
	tool0, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatal("tools[0] 应为 map[string]any")
	}
	if tool0["name"] != "get_time" {
		t.Errorf("tools[0].name = %v, want get_time", tool0["name"])
	}
	if tool0["description"] != "Get the current time" {
		t.Errorf("tools[0].description 应完整保留, got %v", tool0["description"])
	}

	// tools[1] description 被摘要
	tool1, ok := tools[1].(map[string]any)
	if !ok {
		t.Fatal("tools[1] 应为 map[string]any")
	}
	if tool1["name"] != "calc" {
		t.Errorf("tools[1].name = %v, want calc", tool1["name"])
	}
	// "A calculator tool for math operations" = 38 bytes
	desc, ok := tool1["description"].(string)
	if !ok {
		t.Fatalf("tools[1].description 应为 string, got %T", tool1["description"])
	}
	if !strings.HasPrefix(desc, "(") || !strings.HasSuffix(desc, " chars)") {
		t.Errorf("tools[1].description 应为摘要格式, got %v", desc)
	}
}

// TestTruncateContent_Regression 回归测试：验证 content 截断与 tools 简化共存。
//
// 构造含超长 content 和 2+ tools 的 JSON，验证：
//   - content 被截断到 MaxDebugBodyLen + "...(truncated)" 后缀
//   - tools 标识字段保留、非标识字段被摘要
func TestTruncateContent_Regression(t *testing.T) {
	longContent := strings.Repeat("x", 600)
	input := `{"content":"` + longContent + `","tools":[{"name":"t1","description":"tool one","input_schema":{"type":"object"}},{"name":"t2","description":"tool two","input_schema":{"type":"object"}}]}`

	result := truncateContent(input)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("truncateContent 输出不是合法 JSON: %v", err)
	}

	// ── content 被截断 ──
	content, ok := parsed["content"].(string)
	if !ok {
		t.Fatalf("content 应为 string, got %T", parsed["content"])
	}
	expectedLen := MaxDebugBodyLen + len("...(truncated)")
	if len(content) != expectedLen {
		t.Errorf("content 长度 = %d, want %d (500 + 13 后缀)", len(content), expectedLen)
	}
	if !strings.HasSuffix(content, "...(truncated)") {
		t.Errorf("content 应以 ...(truncated) 结尾, got suffix: %q", content[len(content)-20:])
	}

	// ── tools[1] 标识保留、description 被摘要 ──
	tools, ok := parsed["tools"].([]any)
	if !ok {
		t.Fatal("tools 应为 []any 类型")
	}
	if len(tools) != 2 {
		t.Fatalf("tools 长度 = %d, want 2", len(tools))
	}

	tool1, ok := tools[1].(map[string]any)
	if !ok {
		t.Fatal("tools[1] 应为 map[string]any")
	}
	if tool1["name"] != "t2" {
		t.Errorf("tools[1].name = %v, want t2", tool1["name"])
	}
	desc, ok := tool1["description"].(string)
	if !ok {
		t.Fatalf("tools[1].description 应为 string, got %T", tool1["description"])
	}
	if !strings.HasPrefix(desc, "(") || !strings.HasSuffix(desc, " chars)") {
		t.Errorf("tools[1].description 应为摘要格式, got %v", desc)
	}
}

// ────────────────────────────────────────────────────────
// 辅助函数
// ────────────────────────────────────────────────────────

// stringOrAny 辅助函数，将 any 转为字符串用于 t.Run 子测试名。
func stringOrAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return "<non-string>"
}

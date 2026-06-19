package bamboo

import (
	"encoding/json"
	"testing"
)

func TestTool_JSONUsesInputSchema(t *testing.T) {
	tool := Tool{
		Name:        "get_weather",
		Description: "获取指定城市的天气信息",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"city": {"type": "string", "description": "城市名称"},
				"unit": {"type": "string", "description": "温度单位", "enum": ["celsius", "fahrenheit"]}
			},
			"required": ["city"]
		}`),
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析为 map 失败: %v", err)
	}

	if _, ok := raw["input_schema"]; !ok {
		t.Error("期望 JSON 中包含 'input_schema' 字段")
	}
	if _, ok := raw["parameters"]; ok {
		t.Error("JSON 中不应包含 'parameters' 字段，应使用 'input_schema'")
	}
}

func TestTool_JSONRoundtrip(t *testing.T) {
	original := Tool{
		Name:        "search",
		Description: "搜索数据库",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "搜索关键词"},
				"limit": {"type": "number", "description": "返回结果数量上限"}
			},
			"required": ["query"]
		}`),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed Tool
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Name != "search" {
		t.Errorf("Name 不匹配: 期望 search，实际 %s", parsed.Name)
	}
	if parsed.Description != "搜索数据库" {
		t.Errorf("Description 不匹配")
	}

	// 验证 RawMessage 内容完整保留
	var schema map[string]any
	if err := json.Unmarshal(parsed.InputSchema, &schema); err != nil {
		t.Fatalf("解析 InputSchema 失败: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("InputSchema.type 不匹配: 期望 object，实际 %v", schema["type"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) != 2 {
		t.Errorf("Properties 长度不匹配: 期望 2，实际 %v", schema["properties"])
	}
	req, ok := schema["required"].([]any)
	if !ok || len(req) != 1 || req[0] != "query" {
		t.Errorf("Required 不匹配: %v", schema["required"])
	}
}

func TestTool_JSONRoundtrip_ComplexSchema(t *testing.T) {
	// 验证包含 additionalProperties、嵌套 properties 等 JSON Schema 高级字段的完整保留
	complexJSON := `{
		"type": "object",
		"properties": {
			"headers": {
				"type": "object",
				"properties": {
					"X-Project-Id": {"type": "string"}
				},
				"required": ["X-Project-Id"],
				"additionalProperties": {}
			}
		},
		"required": ["headers"],
		"additionalProperties": false
	}`

	original := Tool{
		Name:        "create_endpoint",
		Description: "创建 HTTP 接口",
		InputSchema: json.RawMessage(complexJSON),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed Tool
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal(parsed.InputSchema, &schema); err != nil {
		t.Fatalf("解析 InputSchema 失败: %v", err)
	}

	// 验证 additionalProperties 被完整保留（旧版 PropertyDef 会丢失此字段）
	if _, ok := schema["additionalProperties"]; !ok {
		t.Error("additionalProperties 应被保留")
	}
	headers, ok := schema["properties"].(map[string]any)["headers"].(map[string]any)
	if !ok {
		t.Fatal("headers 属性应存在")
	}
	if _, ok := headers["additionalProperties"]; !ok {
		t.Error("嵌套 additionalProperties 应被保留")
	}
	if _, ok := headers["properties"]; !ok {
		t.Error("嵌套 properties 应被保留")
	}
}

func TestTool_DescriptionOmitEmpty(t *testing.T) {
	tool := Tool{
		Name:        "minimal_tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析为 map 失败: %v", err)
	}

	if _, ok := raw["description"]; ok {
		t.Error("空 Description 应被 omitempty 忽略")
	}
}

func TestBuildParameters_NilSchema(t *testing.T) {
	result := buildParameters(nil)
	if result != nil {
		t.Errorf("nil schema 应返回 nil，实际 %v", result)
	}
}

func TestBuildParameters_ValidSchema(t *testing.T) {
	result := buildParameters(json.RawMessage(`{"type":"object","additionalProperties":false}`))
	if result == nil {
		t.Fatal("非空 schema 不应返回 nil")
	}
	if result["type"] != "object" {
		t.Errorf("type 不匹配: 期望 object，实际 %v", result["type"])
	}
	if _, ok := result["additionalProperties"]; !ok {
		t.Error("additionalProperties 应被保留")
	}
}

func TestBuildParameters_InvalidJSON(t *testing.T) {
	result := buildParameters(json.RawMessage(`{invalid`))
	if result == nil {
		t.Fatal("无效 JSON 不应返回 nil，应回退为最小 schema")
	}
	if result["type"] != "object" {
		t.Errorf("回退 schema 的 type 应为 object，实际 %v", result["type"])
	}
}

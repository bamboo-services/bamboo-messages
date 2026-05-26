package bamboo

import (
	"encoding/json"
	"testing"
)

func TestTool_JSONUsesInputSchema(t *testing.T) {
	tool := Tool{
		Name:        "get_weather",
		Description: "获取指定城市的天气信息",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]PropertyDef{
				"city": {
					Type:        "string",
					Description: "城市名称",
				},
				"unit": {
					Type:        "string",
					Description: "温度单位",
					Enum:        []string{"celsius", "fahrenheit"},
				},
			},
			Required: []string{"city"},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	// 验证使用 "input_schema" 而不是 "parameters"
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
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]PropertyDef{
				"query": {
					Type:        "string",
					Description: "搜索关键词",
				},
				"limit": {
					Type:        "number",
					Description: "返回结果数量上限",
				},
			},
			Required: []string{"query"},
		},
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
	if parsed.InputSchema.Type != "object" {
		t.Errorf("InputSchema.Type 不匹配: 期望 object，实际 %s", parsed.InputSchema.Type)
	}
	if len(parsed.InputSchema.Properties) != 2 {
		t.Errorf("Properties 长度不匹配: 期望 2，实际 %d", len(parsed.InputSchema.Properties))
	}
	if len(parsed.InputSchema.Required) != 1 || parsed.InputSchema.Required[0] != "query" {
		t.Errorf("Required 不匹配: %v", parsed.InputSchema.Required)
	}
}

func TestPropertyDef_WithEnum(t *testing.T) {
	prop := PropertyDef{
		Type:        "string",
		Description: "颜色选项",
		Enum:        []string{"red", "green", "blue"},
	}

	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed PropertyDef
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if len(parsed.Enum) != 3 {
		t.Errorf("Enum 长度不匹配: 期望 3，实际 %d", len(parsed.Enum))
	}
	if parsed.Enum[0] != "red" || parsed.Enum[2] != "blue" {
		t.Errorf("Enum 值不匹配: %v", parsed.Enum)
	}
}

func TestPropertyDef_WithItems(t *testing.T) {
	prop := PropertyDef{
		Type:        "array",
		Description: "字符串列表",
		Items: map[string]any{
			"type": "string",
		},
	}

	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var parsed PropertyDef
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if parsed.Type != "array" {
		t.Errorf("Type 不匹配: 期望 array，实际 %s", parsed.Type)
	}
	if parsed.Items == nil {
		t.Error("Items 不应为 nil")
	}
}

func TestInputSchema_EmptyProperties(t *testing.T) {
	schema := InputSchema{
		Type: "object",
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析为 map 失败: %v", err)
	}

	// omitempty：空 map 和空 slice 不应出现
	if _, ok := raw["properties"]; ok {
		t.Error("空 Properties 应被 omitempty 忽略")
	}
	if _, ok := raw["required"]; ok {
		t.Error("空 Required 应被 omitempty 忽略")
	}
}

func TestTool_DescriptionOmitEmpty(t *testing.T) {
	tool := Tool{
		Name: "minimal_tool",
		InputSchema: InputSchema{
			Type: "object",
		},
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

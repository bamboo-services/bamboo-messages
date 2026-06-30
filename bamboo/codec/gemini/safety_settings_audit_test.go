package gemini

import (
	"testing"
)

// TestAudit_SafetySettings_CodecToProviderTypeMatch 验证 safety_settings 在
// codec->relay->provider 路径上的类型匹配。
//
// Fix: codec 现在将 []geminiSafetySetting 转换为 []map[string]string，
// 避免引入 genai SDK 依赖，provider 适配器通过 GetExtraAny 原样透传。
func TestAudit_SafetySettings_CodecToProviderTypeMatch(t *testing.T) {
	body := []byte(`{
		"contents": [{"role":"user","parts":[{"text":"test"}]}],
		"safetySettings": [
			{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	if req.Config.ProviderExtra == nil {
		t.Fatal("ProviderExtra is nil, safety_settings not stored")
	}

	settings, ok := req.Config.ProviderExtra["safety_settings"]
	if !ok {
		t.Fatal("safety_settings not found in ProviderExtra")
	}

	// Fix: 现在存储的类型是 []map[string]string（通用 map 类型，无 SDK 依赖）
	safetySlice, ok := settings.([]map[string]string)
	if !ok {
		t.Fatalf("safety_settings type = %T, want []map[string]string", settings)
	}

	if len(safetySlice) != 1 {
		t.Fatalf("safety_settings count = %d, want 1", len(safetySlice))
	}

	if safetySlice[0]["category"] != "HARM_CATEGORY_DANGEROUS_CONTENT" {
		t.Errorf("Category = %v, want HARM_CATEGORY_DANGEROUS_CONTENT", safetySlice[0]["category"])
	}
	if safetySlice[0]["threshold"] != "BLOCK_NONE" {
		t.Errorf("Threshold = %v, want BLOCK_NONE", safetySlice[0]["threshold"])
	}
}

// TestAudit_SafetySettings_MultipleCategories 验证多个 safety settings 的转换。
func TestAudit_SafetySettings_MultipleCategories(t *testing.T) {
	body := []byte(`{
		"contents": [{"role":"user","parts":[{"text":"test"}]}],
		"safetySettings": [
			{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_MEDIUM_AND_ABOVE"},
			{"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "BLOCK_LOW_AND_ABOVE"}
		]
	}`)

	req, err := parseRequest(body)
	if err != nil {
		t.Fatalf("parseRequest() error = %v", err)
	}

	safetySlice := req.Config.ProviderExtra["safety_settings"].([]map[string]string)
	if len(safetySlice) != 2 {
		t.Fatalf("safety_settings count = %d, want 2", len(safetySlice))
	}
	if safetySlice[0]["category"] != "HARM_CATEGORY_HARASSMENT" {
		t.Errorf("safetySlice[0].category = %v, want HARM_CATEGORY_HARASSMENT", safetySlice[0]["category"])
	}
	if safetySlice[1]["category"] != "HARM_CATEGORY_HATE_SPEECH" {
		t.Errorf("safetySlice[1].category = %v, want HARM_CATEGORY_HATE_SPEECH", safetySlice[1]["category"])
	}
}

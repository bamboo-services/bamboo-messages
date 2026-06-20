package gemini

import (
	"testing"

	"google.golang.org/genai"
)

// TestAudit_SafetySettings_CodecToProviderTypeMatch 验证 safety_settings 在
// codec->relay->provider 路径上的类型匹配。
//
// Fix: codec 现在将 []geminiSafetySetting 转换为 []*genai.SafetySetting，
// 确保与 provider 期望的类型一致，safety_settings 不再被静默丢弃。
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

	// Fix: 现在存储的类型是 []*genai.SafetySetting（与 provider 期望一致）
	safetySlice, ok := settings.([]*genai.SafetySetting)
	if !ok {
		t.Fatalf("safety_settings type = %T, want []*genai.SafetySetting", settings)
	}

	if len(safetySlice) != 1 {
		t.Fatalf("safety_settings count = %d, want 1", len(safetySlice))
	}

	if safetySlice[0].Category != genai.HarmCategoryDangerousContent {
		t.Errorf("Category = %v, want HarmCategoryDangerousContent", safetySlice[0].Category)
	}
	if safetySlice[0].Threshold != genai.HarmBlockThresholdBlockNone {
		t.Errorf("Threshold = %v, want HarmBlockNone", safetySlice[0].Threshold)
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

	safetySlice := req.Config.ProviderExtra["safety_settings"].([]*genai.SafetySetting)
	if len(safetySlice) != 2 {
		t.Fatalf("safety_settings count = %d, want 2", len(safetySlice))
	}
	if safetySlice[0].Category != genai.HarmCategoryHarassment {
		t.Errorf("safetySlice[0].Category = %v, want Harassment", safetySlice[0].Category)
	}
	if safetySlice[1].Category != genai.HarmCategoryHateSpeech {
		t.Errorf("safetySlice[1].Category = %v, want HateSpeech", safetySlice[1].Category)
	}
}

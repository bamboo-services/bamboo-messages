package responses

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestBuildParams_MaxEffortNormalized 验证 Effort "max" 在 Responses 出口归一化为 xhigh，
// 且 summary 推导落入 detailed 档（与 high/xhigh 一致）。
func TestBuildParams_MaxEffortNormalized(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "max"},
	}
	params := testBuildParams(p, config)

	reasoning, ok := params["reasoning"].(map[string]any)
	if !ok {
		t.Fatal("reasoning should be set")
	}
	if reasoning["effort"] != "xhigh" {
		t.Errorf("reasoning.effort = %v, want %q", reasoning["effort"], "xhigh")
	}
	if _, ok := reasoning["summary"]; ok {
		t.Errorf("reasoning.summary = %v, want omitted (不自动推导，以免 Grok 拒请求)", reasoning["summary"])
	}
}

// TestBuildParams_HighEffortSummary 对照验证 high 档 summary 仍为 detailed。
func TestBuildParams_HighEffortSummary(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
	}
	params := testBuildParams(p, config)

	reasoning, ok := params["reasoning"].(map[string]any)
	if !ok {
		t.Fatal("reasoning should be set")
	}
	if reasoning["effort"] != "high" {
		t.Errorf("reasoning.effort = %v, want %q", reasoning["effort"], "high")
	}
	if _, ok := reasoning["summary"]; ok {
		t.Errorf("reasoning.summary = %v, want omitted (不自动推导)", reasoning["summary"])
	}
}

func TestBuildParams_PassthroughReasoningSummaryAndInclude(t *testing.T) {
	p := NewResponsesProvider("test-api-key")
	config := &provider.ChatConfig{
		ThinkingConfig: &provider.ThinkingConfig{Effort: "high"},
		ProviderExtra: map[string]any{
			"reasoning_summary": "auto",
			"include":           []any{"reasoning.encrypted_content"},
		},
	}
	params := testBuildParams(p, config)
	reasoning, ok := params["reasoning"].(map[string]any)
	if !ok {
		t.Fatal("reasoning should be set")
	}
	if reasoning["summary"] != "auto" {
		t.Errorf("summary = %v, want auto (client opt-in)", reasoning["summary"])
	}
	include, ok := params["include"].([]string)
	if !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Errorf("include = %v", params["include"])
	}
}

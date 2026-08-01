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
	if reasoning["summary"] != "detailed" {
		t.Errorf("reasoning.summary = %v, want %q", reasoning["summary"], "detailed")
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
	if reasoning["summary"] != "detailed" {
		t.Errorf("reasoning.summary = %v, want %q", reasoning["summary"], "detailed")
	}
}

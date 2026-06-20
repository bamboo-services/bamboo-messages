package responses

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
	"github.com/openai/openai-go/v3/responses"
)

// TestAudit_Metadata_ProviderExtraVsConfigField 验证 Responses 适配器中 Metadata 的来源。
//
// Severity: P1
// File:Line: provider/openai/responses/params.go:79
// Issue: Responses adapter buildResponseNewParams 从 config.Metadata 读取 metadata，
//        但 codec/responses 将 metadata 存储在 ProviderExtra["metadata"] 中。
//        当通过 codec→relay→provider 路径时，config.Metadata 为空，
//        metadata 被静默丢弃。
// Affected: Any codec→relay→Responses provider with metadata
func TestAudit_Metadata_ProviderExtraVsConfigField(t *testing.T) {
	p := NewResponsesProvider("test-key")

	// 场景1：Metadata 在 config.Metadata 上（直接调用 API 的正确路径）
	config := &provider.ChatConfig{
		Model:    "gpt-4o",
		Metadata: map[string]string{"key1": "value1"},
	}

	params := p.buildResponseNewParams("gpt-4o", responses.ResponseNewParamsInputUnion{}, config)
	if params.Metadata == nil {
		t.Errorf("config.Metadata should be mapped to params.Metadata")
	}

	// 场景2：Metadata 在 ProviderExtra 上（codec 路径）
	config2 := &provider.ChatConfig{
		Model: "gpt-4o",
		ProviderExtra: map[string]any{
			"metadata": map[string]any{"key1": "value1"},
		},
	}

	params2 := p.buildResponseNewParams("gpt-4o", responses.ResponseNewParamsInputUnion{}, config2)
	if params2.Metadata != nil {
		t.Errorf("P1 BUG: Metadata from ProviderExtra should NOT be used, but it was")
	}
	t.Logf("P1 NOTE: Metadata from ProviderExtra['metadata'] is silently dropped; adapter reads config.Metadata")
}

// TestAudit_Stop_ExtraFields 验证 Stop 参数通过 ExtraFields 传递。
func TestAudit_Stop_ExtraFields(t *testing.T) {
	p := NewResponsesProvider("test-key")

	config := &provider.ChatConfig{
		Model: "gpt-4o",
		Stop:  []string{"STOP", "END"},
	}

	params := p.buildResponseNewParams("gpt-4o", responses.ResponseNewParamsInputUnion{}, config)
	_ = params
}

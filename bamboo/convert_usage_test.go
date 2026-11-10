package bamboo

import (
	"testing"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// TestStreamConverter_UsageMergeKeepsCacheStats 验证 usage 增量采用合并语义。
//
// 回归场景：Anthropic 把输入与缓存统计放在 message_start、把最终 output_tokens 放在
// message_delta。若后到的 usage 增量整块覆盖前值，message_start 的缓存统计会被清零，
// 表现为「缓存无法记录」。
func TestStreamConverter_UsageMergeKeepsCacheStats(t *testing.T) {
	sc := NewStreamConverter()

	// 1. 起始事件
	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})

	// 2. message_start 阶段：输入与缓存统计
	events := sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDeltaWithCache(175, 0, 50, 25),
	})
	if len(events) != 1 || events[0].Type != EventPing {
		t.Fatalf("expected 1 ping event, got %+v", events)
	}
	if events[0].Usage == nil {
		t.Fatal("ping event should carry usage")
	}
	if events[0].Usage.CacheCreationInputTokens != 50 || events[0].Usage.CacheReadInputTokens != 25 {
		t.Errorf("ping usage = %+v, want cache_creation=50 cache_read=25", *events[0].Usage)
	}

	// 3. message_delta 阶段：仅携带最终 output_tokens，输入与缓存字段为零
	sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDeltaWithCache(0, 12, 0, 0),
	})

	// 4. 结束事件，产出最终 usage
	events = sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})
	if len(events) == 0 {
		t.Fatal("expected stop sequence events")
	}

	var final *Usage
	for _, ev := range events {
		if ev.Type == EventMessageDelta && ev.Usage != nil {
			usage := *ev.Usage
			final = &usage
		}
	}
	if final == nil {
		t.Fatal("expected final usage in message_delta event")
	}
	if final.InputTokens != 175 {
		t.Errorf("InputTokens = %d, want 175", final.InputTokens)
	}
	if final.OutputTokens != 12 {
		t.Errorf("OutputTokens = %d, want 12", final.OutputTokens)
	}
	if final.CacheCreationInputTokens != 50 {
		t.Errorf("CacheCreationInputTokens = %d, want 50", final.CacheCreationInputTokens)
	}
	if final.CacheReadInputTokens != 25 {
		t.Errorf("CacheReadInputTokens = %d, want 25", final.CacheReadInputTokens)
	}
}

// TestStreamConverter_UsageMergePrefersLatestNonZero 验证后到的非零 usage 维度会覆盖旧值。
func TestStreamConverter_UsageMergePrefersLatestNonZero(t *testing.T) {
	sc := NewStreamConverter()

	sc.Convert(provider.StreamEvent{Type: provider.StreamTypeStart})
	sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDeltaWithCache(100, 0, 0, 0),
	})
	sc.Convert(provider.StreamEvent{
		Type:  provider.StreamTypeDelta,
		Delta: provider.NewUsageDeltaWithCache(175, 12, 50, 25),
	})

	events := sc.Convert(provider.StreamEvent{Type: provider.StreamTypeDone})

	var final *Usage
	for _, ev := range events {
		if ev.Type == EventMessageDelta && ev.Usage != nil {
			usage := *ev.Usage
			final = &usage
		}
	}
	if final == nil {
		t.Fatal("expected final usage in message_delta event")
	}
	if final.InputTokens != 175 || final.OutputTokens != 12 {
		t.Errorf("final usage = %+v, want input=175 output=12", *final)
	}
	if final.CacheCreationInputTokens != 50 || final.CacheReadInputTokens != 25 {
		t.Errorf("final usage = %+v, want cache_creation=50 cache_read=25", *final)
	}
}

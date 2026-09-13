package gemini

import (
	"crypto/rand"
	"strconv"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// toolCallIDs 为单次响应分配调用 ID，历史和已发出的 ID 均不可用于合成。
type toolCallIDs struct {
	namespace string
	ordinal   uint64
	used      map[string]struct{}
}

func newToolCallIDs(messages []provider.Message) toolCallIDs {
	ids := toolCallIDs{used: make(map[string]struct{})}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			ids.used[call.ID] = struct{}{}
		}
		if message.ToolCallID != "" {
			ids.used[message.ToolCallID] = struct{}{}
		}
	}
	return ids
}

func (ids *toolCallIDs) next(explicit string) string {
	if ids.used == nil {
		ids.used = make(map[string]struct{})
	}
	if explicit != "" {
		ids.used[explicit] = struct{}{}
		return explicit
	}
	if ids.namespace == "" {
		ids.namespace = rand.Text()
	}
	for {
		ids.ordinal++
		id := "gemini_call_" + ids.namespace + "_" + strconv.FormatUint(ids.ordinal, 10)
		if _, occupied := ids.used[id]; occupied {
			continue
		}
		ids.used[id] = struct{}{}
		return id
	}
}

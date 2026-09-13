package gemini

import "fmt"

type geminiToolIdentity struct {
	id   string
	name string
}

type geminiHistoryCall struct {
	geminiToolIdentity
	consumed bool
}

type geminiToolHistory struct {
	calls   map[*geminiFunctionCall]string
	results map[*geminiFuncResponse]geminiToolIdentity
}

// correlateToolHistory 只在当前 model 轮次关联结果；预扫描仅用于避免合成 ID 碰撞。
func correlateToolHistory(contents []geminiContent) geminiToolHistory {
	history := geminiToolHistory{
		calls:   make(map[*geminiFunctionCall]string),
		results: make(map[*geminiFuncResponse]geminiToolIdentity),
	}
	reserved := make(map[string]bool)
	hasModel := false
	for _, content := range contents {
		if content.Role == "model" {
			hasModel = true
		}
		for _, part := range content.Parts {
			if call := part.FunctionCall; call != nil && call.ID != "" {
				reserved[call.ID] = true
			}
		}
	}
	ordinal := 0
	for _, content := range contents {
		for _, part := range content.Parts {
			call := part.FunctionCall
			if call == nil {
				continue
			}
			id := call.ID
			if id == "" {
				for {
					id = fmt.Sprintf("gemini_call_%s_%d", call.Name, ordinal)
					if !reserved[id] {
						break
					}
					ordinal++
				}
			}
			ordinal++
			reserved[id] = true
			history.calls[call] = id
		}
	}
	var active []geminiHistoryCall
	for i := 0; i < len(contents); {
		content := contents[i]
		if responseBearing(content) {
			end := i + 1
			for end < len(contents) && responseBearing(contents[end]) {
				end++
			}
			history.pairResults(contents[i:end], active, hasModel)
			i = end
			continue
		}
		active = nil
		seen := make(map[string]bool)
		for _, part := range content.Parts {
			call := part.FunctionCall
			if call == nil {
				continue
			}
			id := history.calls[call]
			if content.Role == "model" && !seen[id] {
				active = append(active, geminiHistoryCall{geminiToolIdentity: geminiToolIdentity{id: id, name: call.Name}})
				seen[id] = true
			}
		}
		i++
	}
	return history
}

func responseBearing(content geminiContent) bool {
	if content.Role != "user" && content.Role != "function" {
		return false
	}
	for _, part := range content.Parts {
		if part.FunctionResponse != nil {
			return true
		}
	}
	return false
}

// pairResults 先在整个连续结果段预留显式 ID，再按声明顺序关联缺省 ID，输出顺序不变。
func (h geminiToolHistory) pairResults(contents []geminiContent, active []geminiHistoryCall, hasModel bool) {
	var pending []*geminiFuncResponse
	for _, content := range contents {
		for _, part := range content.Parts {
			r := part.FunctionResponse
			if r == nil {
				continue
			}
			h.results[r] = geminiToolIdentity{name: r.Name}
			if r.ID == "" {
				pending = append(pending, r)
				continue
			}
			if !hasModel {
				h.results[r] = geminiToolIdentity{id: r.ID, name: r.Name}
				continue
			}
			for i := range active {
				call := &active[i]
				if call.id != r.ID {
					continue
				}
				if !call.consumed && (r.Name == "" || r.Name == call.name) {
					call.consumed = true
					h.results[r] = call.geminiToolIdentity
				}
				break
			}
		}
	}
	for _, r := range pending {
		for i := range active {
			call := &active[i]
			if !call.consumed && r.Name != "" && call.name == r.Name {
				call.consumed = true
				h.results[r] = call.geminiToolIdentity
				break
			}
		}
	}
}

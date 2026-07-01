package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	"github.com/bamboo-services/bamboo-messages/bamboo"
)

// ── Gemini 响应 JSON 结构体 ──

// geminiResponse Gemini GenerateContentResponse 完整结构。
type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsageMeta  `json:"usageMetadata,omitempty"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
	ResponseID    string            `json:"responseId,omitempty"`
}

// geminiCandidate Gemini 候选响应。
type geminiCandidate struct {
	Content       *geminiContentOut `json:"content,omitempty"`
	FinishReason  string            `json:"finishReason,omitempty"`
	Index         int               `json:"index"`
	SafetyRatings []json.RawMessage `json:"safetyRatings,omitempty"`
}

// geminiContentOut 候选内容（输出方向）。
type geminiContentOut struct {
	Parts []geminiPartOut `json:"parts"`
	Role  string          `json:"role"`
}

// geminiPartOut 输出方向的 Part（支持 text / functionCall / thought / inlineData / fileData）。
type geminiPartOut struct {
	Text         string              `json:"text,omitempty"`
	Thought      bool                `json:"thought,omitempty"`
	FunctionCall *geminiFuncCallOut  `json:"functionCall,omitempty"`
	InlineData   *geminiInlineData   `json:"inlineData,omitempty"`
	FileData     *geminiFileData     `json:"fileData,omitempty"`
}

// geminiFuncCallOut 输出方向的 functionCall。
type geminiFuncCallOut struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// geminiUsageMeta Token 使用量元数据。
type geminiUsageMeta struct {
	PromptTokenCount        int64 `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount    int64 `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount         int64 `json:"totalTokenCount,omitempty"`
	CachedContentTokenCount int64 `json:"cachedContentTokenCount,omitempty"`
}

// serializeResponse 将 Bamboo Response 序列化为 Gemini GenerateContentResponse JSON。
func serializeResponse(resp *bamboo.Response) ([]byte, error) {
	// 构建 candidate
	candidate := geminiCandidate{
		Index:        0,
		FinishReason: mapFinishReasonToGemini(resp.StopReason),
		Content: &geminiContentOut{
			Role:  "model",
			Parts: buildResponseParts(resp.Content),
		},
	}

	out := geminiResponse{
		Candidates: []geminiCandidate{candidate},
		UsageMetadata: &geminiUsageMeta{
			PromptTokenCount:        resp.Usage.InputTokens,
			CandidatesTokenCount:    resp.Usage.OutputTokens,
			TotalTokenCount:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
			CachedContentTokenCount: resp.Usage.CacheReadInputTokens,
		},
	}
	// Gemini 无原生 cache_creation_input_tokens 字段，仅映射 CacheReadInputTokens 到 cachedContentTokenCount。
	// CacheCreationInputTokens 在跨协议转换中会丢失，此为已知限制。

	if resp.Model != "" {
		out.ModelVersion = resp.Model
	}

	return json.Marshal(out)
}

// buildResponseParts 将 Bamboo ContentBlock 列表转换为 Gemini parts 数组。
//
// 映射规则:
//   - TextBlock      → {text: "..."}
//   - ThinkingBlock  → {text: "...", thought: true}
//   - ToolUseBlock   → {functionCall: {name, args}}
//   - ImageBlock     → {inlineData: {mimeType, data}} 或 {fileData: {fileUri}}
//   - DocumentBlock  → {inlineData: {mimeType, data}} 或 {fileData: {fileUri}}
//   - ToolResultBlock → 不应出现在 assistant 响应中，记录警告并跳过
func buildResponseParts(blocks []bamboo.ContentBlock) []geminiPartOut {
	parts := make([]geminiPartOut, 0, len(blocks))
	for _, block := range blocks {
		switch b := block.(type) {
		case *bamboo.TextBlock:
			if b.Text != "" {
				parts = append(parts, geminiPartOut{Text: b.Text})
			}
		case *bamboo.ToolUseBlock:
			args := b.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			part := geminiPartOut{
				FunctionCall: &geminiFuncCallOut{
					Name: b.Name,
					Args: args,
				},
			}
			if b.ID != "" {
				part.FunctionCall.ID = b.ID
			}
			parts = append(parts, part)
		case *bamboo.ThinkingBlock:
			if b.Thinking != "" {
				parts = append(parts, geminiPartOut{Text: b.Thinking, Thought: true})
			}
		case *bamboo.ImageBlock:
			// Gemini 原生支持 inlineData / fileData，映射为 inlineData part
			if b.Source == nil {
				continue
			}
			part := buildInlineDataPart(b.Source)
			if part != nil {
				parts = append(parts, *part)
			}
		case *bamboo.DocumentBlock:
			// Gemini 原生支持 inlineData / fileData，映射为 inlineData part
			if b.Source == nil {
				continue
			}
			part := buildInlineDataPart(b.Source)
			if part != nil {
				parts = append(parts, *part)
			}
		case *bamboo.ToolResultBlock:
			// ToolResultBlock 不应出现在 assistant 响应中，记录警告并跳过
			xLog.WithName("codec/gemini").SugarWarn(context.Background(),
				fmt.Sprintf("warning: ToolResultBlock should not appear in assistant response, skipped (tool_use_id=%s)", b.ToolUseID))
		}
	}
	return parts
}

// buildInlineDataPart 将 ContentSource 转换为 Gemini inlineData 或 fileData part。
//
// 映射规则:
//   - base64 类型 → {inlineData: {mimeType, data}}
//   - url 类型    → {fileData: {mimeType, fileUri}}
//   - 其他类型    → 返回 nil
func buildInlineDataPart(source *bamboo.ContentSource) *geminiPartOut {
	switch source.Type {
	case "base64":
		return &geminiPartOut{
			InlineData: &geminiInlineData{
				MimeType: source.MediaType,
				Data:     source.Data,
			},
		}
	case "url":
		return &geminiPartOut{
			FileData: &geminiFileData{
				MimeType: source.MediaType,
				FileURI:  source.URL,
			},
		}
	default:
		xLog.WithName("codec/gemini").SugarWarn(context.Background(),
			fmt.Sprintf("warning: unsupported ContentSource type %q for inline_data mapping, skipped", source.Type))
		return nil
	}
}

// mapFinishReasonToGemini 将 Bamboo FinishReason 映射为 Gemini finishReason。
//
// Gemini finishReason 取值:
//   - STOP — 正常结束
//   - MAX_TOKENS — 达到最大 token 数
//   - SAFETY — 安全拦截
//   - RECITATION — 引用拦截
//   - OTHER — 其他原因
func mapFinishReasonToGemini(reason bamboo.FinishReason) string {
	switch reason {
	case bamboo.FinishReasonEndTurn:
		return "STOP"
	case bamboo.FinishReasonMaxTokens:
		return "MAX_TOKENS"
	case bamboo.FinishReasonToolUse:
		return "STOP"
	case bamboo.FinishReasonStopSequence:
		return "STOP"
	default:
		return "STOP"
	}
}

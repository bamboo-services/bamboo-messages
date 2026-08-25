package bamboo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	xLog "github.com/bamboo-services/bamboo-base-go/common/log"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// finishReasonMap provider.FinishReason → bamboo.FinishReason 映射表。
//
// 用于将底层 provider 的完成原因映射为上层 bamboo 的标准完成原因。
var finishReasonMap = map[provider.FinishReason]FinishReason{
	provider.FinishReasonStop:      FinishReasonEndTurn,
	provider.FinishReasonLength:    FinishReasonMaxTokens,
	provider.FinishReasonToolCalls: FinishReasonToolUse,
}

// messagesToProvider 将 bamboo.BambooMessage 列表转换为 provider.Message 列表。
//
// tool_result 消息会被拆分为独立的 Role=tool 消息；
// image/document 类型会被静默丢弃并记录日志。
// thinking block 的内容会被保留到 provider.Message 的 ThinkingContent/ThinkingSignature 字段。
func messagesToProvider(msgs []BambooMessage) ([]provider.Message, error) {
	var result []provider.Message

	// 预先建立 tool_use_id -> tool_name 映射，供 ToolResultBlock 在缺少 ToolName 时反查
	toolNames := make(map[string]string)
	for _, msg := range msgs {
		for _, block := range msg.Content {
			if tu, ok := block.(*ToolUseBlock); ok && tu.ID != "" && tu.Name != "" {
				toolNames[tu.ID] = tu.Name
			}
		}
	}

	for _, msg := range msgs {
		var textBuilder strings.Builder
		var toolCalls []provider.ToolCall
		var toolResults []provider.Message
		var contentBlocks []provider.ContentBlock
		var msgCacheControl *provider.CacheControl
		var cacheControlBlockType string
		var ccCount int
		var thinkingContent string
		var thinkingSignature string
		var thinkingSignatureProvider string
		var redactedThinkingData string

		// Content 为空数组时允许透传，生成 Content="" 的空消息，
		// 由下游适配器内部处理（补空字符串或跳过），外部允许传递空 content。
		for _, block := range msg.Content {
			switch b := block.(type) {
			case *TextBlock:
				textBuilder.WriteString(b.Text)
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
					cacheControlBlockType = "text"
				}
			case *ThinkingBlock:
				// 保留思考过程到 provider.Message 的 ThinkingContent/ThinkingSignature 字段，
				// 用于多轮对话中向 provider 回传 thinking block 内容。
				// 多个 ThinkingBlock 时拼接内容，保留最后一个签名及其血统。
				if b.Thinking != "" {
					thinkingContent += b.Thinking
				}
				if b.Signature != "" {
					thinkingSignature = b.Signature
					thinkingSignatureProvider = b.SignatureProvider
				} else if thinkingSignatureProvider == "" && b.SignatureProvider != "" {
					thinkingSignatureProvider = b.SignatureProvider
				}
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
					cacheControlBlockType = "thinking"
				}
			case *RedactedThinkingBlock:
				redactedThinkingData = b.Data
			case *ToolUseBlock:
				// 过滤无 id 的 tool_use 废片：无法与后续 tool_result 配对，
				// 且 OpenAI 等上游会因此报 500，直接忽略不向上传递。
				if b.ID == "" {
					xLog.WithName("bamboo").SugarWarn(context.Background(),
						"warning: tool_use block 缺少 id，已忽略（废片，不向上传递）")
					continue
				}
				toolCalls = append(toolCalls, provider.ToolCall{
					ID:   b.ID,
					Type: "function",
					Function: provider.FunctionCall{
						Name:      b.Name,
						Arguments: string(b.Input),
					},
				})
				if b.CacheControl != nil {
					ccCount++
					msgCacheControl = b.CacheControl
					cacheControlBlockType = "tool_use"
				}
			case *ToolResultBlock:
				// 过滤无 tool_use_id 的 tool_result 废片：无法与 assistant tool_call 配对，
				// 且 OpenAI 等上游会因此报 500，直接忽略不向上传递。
				if b.ToolUseID == "" {
					xLog.WithName("bamboo").SugarWarn(context.Background(),
						"warning: tool_result block 缺少 tool_use_id，已忽略（废片，不向上传递）")
					continue
				}
				toolName := b.ToolName
				if toolName == "" && b.ToolUseID != "" {
					toolName = toolNames[b.ToolUseID]
				}
				toolResults = append(toolResults, provider.Message{
					Role:                  provider.RoleTool,
					Content:               b.Content,
					ToolCallID:            b.ToolUseID,
					ToolName:              toolName,
					CacheControl:          b.CacheControl,
					CacheControlBlockType: "tool_result",
				})
			case *ImageBlock:
				// 仅用户消息支持图片内容块
				if msg.Role == RoleUser && b.Source != nil {
					contentBlocks = append(contentBlocks, provider.ImageContentBlock{
						Source: provider.ImageSource{
							Type:      b.Source.Type,
							MediaType: b.Source.MediaType,
							Data:      b.Source.Data,
							URL:       b.Source.URL,
						},
					})
					if b.CacheControl != nil {
						ccCount++
						msgCacheControl = b.CacheControl
						cacheControlBlockType = "image"
					}
				}
			case *DocumentBlock:
				// 仅用户消息支持文档内容块
				if msg.Role == RoleUser && b.Source != nil {
					contentBlocks = append(contentBlocks, provider.DocumentContentBlock{
						Source: provider.DocumentSource{
							Type:      b.Source.Type,
							MediaType: b.Source.MediaType,
							Data:      b.Source.Data,
							URL:       b.Source.URL,
						},
					})
					if b.CacheControl != nil {
						ccCount++
						msgCacheControl = b.CacheControl
						cacheControlBlockType = "document"
					}
				}
			default:
				// 未知的 ContentBlock 类型，记录详细信息以便排查
				xLog.WithName("bamboo").SugarWarn(context.Background(),
					fmt.Sprintf("warning: dropped unsupported content block type %q (implement %T in messagesToProvider to support it)", b.BlockType(), b))
			}
		}

		if ccCount > 1 {
			xLog.WithName("bamboo").SugarWarn(context.Background(),
				fmt.Sprintf("warning: message has %d cache_control breakpoints, only the last one is kept", ccCount))
		}

		content := textBuilder.String()
		hasContent := content != "" || len(toolCalls) > 0 || len(contentBlocks) > 0 || thinkingContent != "" || thinkingSignature != "" || redactedThinkingData != ""
		if hasContent || len(msg.Content) == 0 {
			if msg.Role == RoleAssistant && content == "" && len(toolCalls) == 0 && len(contentBlocks) == 0 && thinkingContent == "" && redactedThinkingData == "" {
				content = "-"
			}
			if msgCacheControl != nil && !hasContent && len(msg.Content) > 0 {
				xLog.WithName("bamboo").SugarWarn(context.Background(),
					"warning: message has CacheControl but no content blocks were produced, cache_control will be lost")
			}
			result = append(result, provider.Message{
				Role:                      providerRole(msg.Role),
				Content:                   content,
				ContentBlocks:             contentBlocks,
				ThinkingContent:           thinkingContent,
				ThinkingSignature:         thinkingSignature,
				ThinkingSignatureProvider: thinkingSignatureProvider,
				RedactedThinkingData:      redactedThinkingData,
				ReasoningID:               msg.ReasoningID,
				ToolCalls:                 toolCalls,
				CacheControl:              msgCacheControl,
				CacheControlBlockType:     cacheControlBlockType,
			})
		}
		result = append(result, toolResults...)
	}

	// 跨消息孤儿配对：丢弃无法配对的废片工具调用（场景 B/D），
	// 避免 OpenAI 等上游因 "insufficient tool messages following tool_calls"
	// 返回 500。正常配对的工具调用不受影响。
	result = sanitizeToolMessages(result)

	return result, nil
}

// sanitizeToolMessages 过滤消息序列中无法配对的孤儿工具调用。
//
// 配对规则（Chat Completions 语义下 tool 必须紧跟 assistant(tool_calls)）：
//   - assistant 声明了但没有任何 tool 响应的 tool_call → 丢弃（孤儿工具调用）
//   - tool 响应没有前置的 assistant 声明 → 丢弃（孤立工具响应）
//   - tool 响应出现在其声明 assistant 之前 → 丢弃（乱序，无法还原）
//   - 同一 tool_call_id 的重复响应 → 仅保留第一个
//   - assistant 消息过滤后无任何内容（无文本/思考/内容块）→ 整体跳过
//
// 丢弃后各消息保持原始相对顺序。仅处理含 tool 相关的消息，纯文本消息原样透传。
func sanitizeToolMessages(msgs []provider.Message) []provider.Message {
	// 声明记录：tool_call_id → 首次声明的 assistant 消息位置
	type toolDecl struct {
		msgIdx    int // 声明它的 assistant 在 msgs 中的下标
		callIdx   int // 在 ToolCalls 中的下标
		responded bool
	}
	declared := make(map[string]*toolDecl)
	for i := range msgs {
		if msgs[i].Role != provider.RoleAssistant {
			continue
		}
		for j := range msgs[i].ToolCalls {
			id := msgs[i].ToolCalls[j].ID
			if id == "" {
				continue // 修改 1 已过滤，此处防御
			}
			if _, ok := declared[id]; !ok {
				declared[id] = &toolDecl{msgIdx: i, callIdx: j}
			}
		}
	}

	// 响应记录：tool_call_id → 首个有效 tool 响应的 msgs 下标
	respondedPos := make(map[string]int)
	for i := range msgs {
		if msgs[i].Role != provider.RoleTool {
			continue
		}
		id := msgs[i].ToolCallID
		if id == "" {
			continue
		}
		d, ok := declared[id]
		if !ok {
			xLog.WithName("bamboo").SugarWarn(context.Background(),
				fmt.Sprintf("warning: tool_result(tool_use_id=%q) 无对应 assistant tool_call，已忽略（孤儿工具响应）", id))
			continue
		}
		if d.msgIdx > i {
			xLog.WithName("bamboo").SugarWarn(context.Background(),
				fmt.Sprintf("warning: tool_result(tool_use_id=%q) 出现在其 assistant tool_call 之前，已忽略（乱序）", id))
			continue
		}
		if _, dup := respondedPos[id]; dup {
			xLog.WithName("bamboo").SugarWarn(context.Background(),
				fmt.Sprintf("warning: tool_result(tool_use_id=%q) 重复，已忽略", id))
			continue
		}
		respondedPos[id] = i
		d.responded = true
	}

	result := make([]provider.Message, 0, len(msgs))
	for i := range msgs {
		m := msgs[i]
		switch m.Role {
		case provider.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				kept := make([]provider.ToolCall, 0, len(m.ToolCalls))
				for j := range m.ToolCalls {
					id := m.ToolCalls[j].ID
					d := declared[id]
					if d != nil && d.msgIdx == i && d.callIdx == j && d.responded {
						kept = append(kept, m.ToolCalls[j])
					} else if id != "" {
						xLog.WithName("bamboo").SugarWarn(context.Background(),
							fmt.Sprintf("warning: assistant tool_call(id=%q) 无对应 tool_result，已忽略（孤儿工具调用）", id))
					}
				}
				m.ToolCalls = kept
			}
			// 过滤后无任何内容 → 整体跳过
			if len(m.ToolCalls) == 0 && m.Content == "" && m.ThinkingContent == "" &&
				len(m.ContentBlocks) == 0 && m.RedactedThinkingData == "" {
				xLog.WithName("bamboo").SugarWarn(context.Background(),
					"warning: assistant 消息仅含被过滤的 tool_call，已整体忽略")
				continue
			}
			result = append(result, m)
		case provider.RoleTool:
			// 仅保留首个有效响应（respondedPos 记录的）。
			// 注意：必须用 ok 模式判断存在性，避免 key 缺失时
			// map 零值 0 与位置 0 的 tool 消息误匹配。
			if pos, ok := respondedPos[m.ToolCallID]; ok && pos == i {
				result = append(result, m)
			}
		default:
			result = append(result, m)
		}
	}
	return result
}

// providerRole 将 bamboo.MessageRole 转换为 provider.MessageRole。
//
// 已知角色直接映射；"system" 角色记录警告并降级为 user（应通过 system 参数传递）；
// 其他未知角色记录警告并降级为 user。
func providerRole(role MessageRole) provider.MessageRole {
	switch role {
	case RoleUser:
		return provider.RoleUser
	case RoleAssistant:
		return provider.RoleAssistant
	case "system":
		// system 角色应通过 Chat/Complete 的 system 参数传递，而非消息角色。
		// 此处记录警告并降级为 user 角色，避免请求被拒绝。
		xLog.WithName("bamboo").SugarWarn(context.Background(),
			`warning: message role "system" should use the system parameter instead, falling back to "user"`)
		return provider.RoleUser
	default:
		xLog.WithName("bamboo").SugarWarn(context.Background(),
			fmt.Sprintf("warning: unknown message role %q, falling back to \"user\"", role))
		return provider.RoleUser
	}
}

// configToProvider 将 bamboo.RequestConfig 转换为 provider.ChatConfig。
//
// 复制所有公共字段（Model, MaxTokens, Temperature, TopP, Stop, Tools, Metadata），
// 并透传 ThinkingConfig 和 ProviderExtra。
func configToProvider(cfg *RequestConfig) *provider.ChatConfig {
	if cfg == nil {
		return nil
	}

	var thinkingConfig *provider.ThinkingConfig
	var providerExtra map[string]any
	thinkingConfig = cfg.ThinkingConfig
	providerExtra = cfg.ProviderExtra
	if cfg.ToolChoice == "required" || cfg.ToolChoice == "forced" || cfg.ToolChoice == "none" {
		// 仅在强制/禁用工具时去掉 Anthropic 风格的 thinking 透传键，
		// 避免 GLM 等上游把 thinking JSON 与 tool_choice 冲突。
		// ThinkingConfig.Effort 必须保留：Gemini 思考+工具是官方支持组合，
		// Responses 客户端带 tool_choice=auto 时也不能把思考关掉。
		providerExtra = stripThinkingExtra(cfg.ProviderExtra)
	}

	return &provider.ChatConfig{
		Model:              cfg.Model,
		MaxTokens:          cfg.MaxTokens,
		Temperature:        cfg.Temperature,
		TopP:               cfg.TopP,
		Stop:               cfg.StopSequences,
		Tools:              toolsToProvider(cfg.Tools),
		Metadata:           cfg.Metadata,
		UserID:             cfg.UserID,
		ToolChoice:         cfg.ToolChoice,
		ResponseFormat:     cfg.ResponseFormat,
		ParallelToolCalls:  cfg.ParallelToolCalls,
		ThinkingConfig:     thinkingConfig,
		SystemCacheControl: cfg.SystemCacheControl,
		PromptCacheKey:     cfg.PromptCacheKey,
		ProviderExtra:      providerExtra,
	}
}

// stripThinkingExtra 返回剔除 thinking 键后的 ProviderExtra 副本。
//
// 原 map 无 thinking 键时直接返回原引用（避免无谓复制）；
// 含 thinking 键时复制一份再删除，不修改调用方持有的原始 map。
func stripThinkingExtra(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	if _, ok := extra["thinking"]; !ok {
		return extra
	}
	cloned := make(map[string]any, len(extra))
	for k, v := range extra {
		cloned[k] = v
	}
	delete(cloned, "thinking")
	return cloned
}

// toolsToProvider 将 bamboo.Tool 列表转换为 provider.Tool 列表。
//
// 将每个 bamboo.Tool 转换为 provider.Tool，类型固定为 "function"，
// 函数定义包含名称、描述和 JSON Schema 参数。
// InputSchema 作为 json.RawMessage 原样解析为 map，确保完整保留所有 JSON Schema 字段。
func toolsToProvider(tools []Tool) []provider.Tool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]provider.Tool, 0, len(tools))
	for _, t := range tools {
		result = append(result, provider.Tool{
			Type: "function",
			Function: provider.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  buildParameters(t.InputSchema),
			},
			CacheControl: t.CacheControl,
		})
	}
	return result
}

// buildParameters 将 InputSchema（json.RawMessage）解析为 map[string]any。
//
// 作为完整 JSON Schema 的无损中间表示，直接反序列化原始 JSON，
// 避免结构体字段穷举导致的 schema 字段丢失（如 additionalProperties、嵌套 properties 等）。
// 若 schema 为空则返回 nil（表示工具无参数）。
func buildParameters(schema json.RawMessage) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	var params map[string]any
	if err := json.Unmarshal(schema, &params); err != nil {
		// 解析失败时回退为最小合法 schema，保证请求不因工具定义格式问题中断
		return map[string]any{"type": "object"}
	}
	return params
}

// resultToResponse 将 provider.CompletionResult 转换为 bamboo.Response。
//
// 生成唯一的 ID 和 RequestID，填充 Type / Role / StopReason / Usage /
// ProviderType / CreatedAt 等字段，将工具调用转换为 ToolUse 类型的 ContentBlock。
// 若 result 包含 Thinking 内容，会生成 ThinkingBlock 放在 Content 最前面。
func resultToResponse(result *provider.CompletionResult, providerType string) *Response {
	if result == nil {
		return nil
	}
	var content []ContentBlock
	if result.Thinking != "" || result.ThinkingSignature != "" {
		sp := result.ThinkingSignatureProvider
		if sp == "" && result.ThinkingSignature != "" {
			sp = SignatureProviderFromUpstream(provider.ProviderType(providerType))
		}
		content = append(content, NewThinkingBlockWithProvider(result.Thinking, result.ThinkingSignature, sp))
	}
	for _, rt := range result.RedactedThinking {
		content = append(content, NewRedactedThinkingBlock(rt))
	}
	if result.Content != "" {
		content = append(content, NewTextBlock(result.Content))
	}
	for _, tc := range result.ToolCalls {
		content = append(content, NewToolUseBlockWithRawInput(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}
	if len(content) == 0 {
		content = []ContentBlock{}
	}
	id := fmt.Sprintf("bamboo_msg_%d", time.Now().UnixNano())
	if result.ResponseID != "" {
		id = result.ResponseID
	}
	return &Response{
		ID:         id,
		Type:       "message",
		Role:       RoleAssistant,
		Content:    content,
		StopReason: mapFinishReason(result.FinishReason),
		Usage: Usage{
			InputTokens:              result.Usage.InputTokens,
			OutputTokens:             result.Usage.OutputTokens,
			CacheCreationInputTokens: result.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     result.Usage.CacheReadInputTokens,
			ReasoningTokens:          result.Usage.ReasoningTokens,
		},
		ProviderType: providerType,
		RequestID:    fmt.Sprintf("req_%d", time.Now().UnixNano()),
		ResponseID:   result.ResponseID,
		ReasoningID:  result.ReasoningID,
		CreatedAt:    time.Now().Unix(),
	}
}

func mapFinishReason(reason provider.FinishReason) FinishReason {
	if r, ok := finishReasonMap[reason]; ok {
		return r
	}
	return FinishReasonEndTurn
}

// StreamConverter 维护流事件转换状态，将 provider.StreamEvent 序列映射为 Anthropic 风格事件序列。
type StreamConverter struct {
	blockIndex               int
	nextBlockIndex           int
	usage                    *Usage
	started                  bool
	textBlockStarted         bool
	textBlockIndex           int
	thinkingBlockStarted     bool
	thinkingBlockIndex       int
	currentToolBlockIndex    int
	toolBlockStarted         bool
	openToolBlockIndexes     []int
	toolBlockByProviderIndex map[int]int
	toolBlockByID            map[string]int
	finishReason             FinishReason
	stopHandled              bool
	stoppedBlockIndexes      map[int]bool  // 已发送 content_block_stop 的 block index 集合（防重复）
	metadata                 *MessageDelta // 由 MetadataDelta 收集的元信息，在 handleStop 时输出
	signatureProvider        string        // 当前上游协议的思考凭证血统，写入 thinking block / signature_delta
}

func NewStreamConverter() *StreamConverter { return &StreamConverter{} }

// NewStreamConverterForProvider 创建带上游血统的流转换器，thinking 块会打上对应 SignatureProvider。
func NewStreamConverterForProvider(pt provider.ProviderType) *StreamConverter {
	return &StreamConverter{signatureProvider: SignatureProviderFromUpstream(pt)}
}

// Convert 将单个 provider.StreamEvent 转换为 bamboo.StreamEvent 列表。
//
// 根据事件类型路由到不同的处理方法，自动管理内容块索引
// 和 BlockStart 状态追踪。
//
// StreamTypeStop 不会立即输出终止事件，而是缓存 finishReason 并等待
// StreamTypeDone 触发 handleStop()。这彻底消除了双 stop 导致的
// finish_reason 覆盖问题——无论上游发多少个 StreamTypeStop，
// 客户端只收到一次终止事件序列。
func (sc *StreamConverter) Convert(event provider.StreamEvent) []StreamEvent {
	switch event.Type {
	case provider.StreamTypeStart:
		return sc.handleStart(event)
	case provider.StreamTypeDelta:
		return sc.handleDelta(event.Delta)
	case provider.StreamTypeStop:
		sc.recordFinishReason(event.FinishReason)
		return nil
	case provider.StreamTypeDone:
		if !sc.started || sc.stopHandled {
			return nil
		}
		return sc.handleStop()
	case provider.StreamTypeError:
		return sc.handleError(event.Err)
	default:
		return nil
	}
}

// recordFinishReason 以优先级策略记录完成原因。
//
// 优先级：tool_use > max_tokens > end_turn > stop_sequence。
// 当已记录的原因为 tool_use 时不允许被后续的 stop 覆盖——
// 这是 agent loop 不中断的关键保障：即使上游先发 stop 后发 tool_calls，
// 最终输出给客户端的 finish_reason 始终是 tool_calls。
func (sc *StreamConverter) recordFinishReason(reason provider.FinishReason) {
	incoming := mapFinishReason(reason)
	if finishReasonPriority(incoming) > finishReasonPriority(sc.finishReason) {
		sc.finishReason = incoming
	}
}

// finishReasonPriority 返回完成原因的优先级权重。
// tool_use 最高（2），确保有工具调用时不会被其他原因覆盖。
// max_tokens 次之（1），因为 token 限制是上游的硬性截断。
// 其他原因最低（0）。
func finishReasonPriority(r FinishReason) int {
	switch r {
	case FinishReasonToolUse:
		return 2
	case FinishReasonMaxTokens:
		return 1
	default:
		return 0
	}
}

func (sc *StreamConverter) handleStart(event provider.StreamEvent) []StreamEvent {
	sc.started = true
	sc.blockIndex = 0
	sc.nextBlockIndex = 0
	sc.textBlockStarted = false
	sc.textBlockIndex = 0
	sc.thinkingBlockStarted = false
	sc.thinkingBlockIndex = 0
	sc.currentToolBlockIndex = 0
	sc.toolBlockStarted = false
	sc.openToolBlockIndexes = nil
	sc.toolBlockByProviderIndex = make(map[int]int)
	sc.toolBlockByID = make(map[string]int)
	sc.finishReason = ""
	sc.stopHandled = false
	sc.stoppedBlockIndexes = make(map[int]bool)
	sc.metadata = nil
	start := StreamEvent{
		Type:    EventMessageStart,
		Message: &BambooMessage{Role: RoleAssistant, Content: []ContentBlock{}},
		Usage:   &Usage{},
	}
	if event.Delta.Type == provider.StreamDeltaTypeMetadata {
		if data, ok := event.Delta.Data.(provider.MetadataData); ok {
			sc.metadata = &MessageDelta{
				ResponseID:       data.ResponseID,
				ReasoningID:      data.ReasoningID,
				EncryptedContent: data.EncryptedContent,
			}
			start.Delta = sc.metadata
		}
	}
	return []StreamEvent{start}
}

func (sc *StreamConverter) nextIndex() int {
	idx := sc.nextBlockIndex
	sc.nextBlockIndex++
	sc.blockIndex = idx
	return idx
}

// stopForNewBlock 在开始新内容块前关闭异类已活跃的块。
//
// 仅在类型切换时关闭对方类型（如开始 text 时关闭 thinking，反之亦然），
// 允许同一类型的增量在已有 block 上继续追加。
// startToolBlock 和 handleStop 仍通过 stopAllTextOrThinking 关闭全部。
func (sc *StreamConverter) stopForNewBlock(newType ContentBlockType) []StreamEvent {
	var events []StreamEvent
	if newType == ContentBlockText && sc.thinkingBlockStarted {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.thinkingBlockIndex,
		})
	}
	if newType == ContentBlockThinking && sc.textBlockStarted {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.textBlockIndex,
		})
	}
	return events
}

// stopAllTextOrThinking 关闭所有活跃的 text/thinking 块，用于 tool block 开始和流结束。
func (sc *StreamConverter) stopAllTextOrThinking() []StreamEvent {
	var events []StreamEvent
	if sc.thinkingBlockStarted {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.thinkingBlockIndex,
		})
	}
	if sc.textBlockStarted {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: sc.textBlockIndex,
		})
	}
	return events
}

func (sc *StreamConverter) startTextBlock() StreamEvent {
	idx := sc.nextIndex()
	sc.textBlockStarted = true
	sc.textBlockIndex = idx
	return StreamEvent{
		Type:         EventContentBlockStart,
		Index:        idx,
		ContentBlock: NewTextBlock(""),
	}
}

func (sc *StreamConverter) startThinkingBlock() StreamEvent {
	idx := sc.nextIndex()
	sc.thinkingBlockStarted = true
	sc.thinkingBlockIndex = idx
	return StreamEvent{
		Type:         EventContentBlockStart,
		Index:        idx,
		ContentBlock: NewThinkingBlockWithProvider("", "", sc.signatureProvider),
	}
}

func (sc *StreamConverter) startToolBlock(data provider.ToolCallData) []StreamEvent {
	var events []StreamEvent
	events = append(events, sc.stopAllTextOrThinking()...)

	if data.HasIndex {
		if idx, ok := sc.toolBlockByProviderIndex[data.Index]; ok {
			sc.currentToolBlockIndex = idx
			sc.toolBlockStarted = true
			sc.blockIndex = idx
			return events
		}
	}
	if data.ID != "" {
		if idx, ok := sc.toolBlockByID[data.ID]; ok {
			sc.currentToolBlockIndex = idx
			sc.toolBlockStarted = true
			sc.blockIndex = idx
			return events
		}
	}

	idx := sc.nextIndex()
	sc.currentToolBlockIndex = idx
	sc.toolBlockStarted = true
	sc.openToolBlockIndexes = append(sc.openToolBlockIndexes, idx)
	if data.HasIndex {
		sc.toolBlockByProviderIndex[data.Index] = idx
	}
	if data.ID != "" {
		sc.toolBlockByID[data.ID] = idx
	}

	events = append(events, StreamEvent{
		Type:         EventContentBlockStart,
		Index:        idx,
		ContentBlock: NewToolUseBlock(data.ID, data.Name, nil),
	})
	return events
}

func (sc *StreamConverter) toolDeltaIndex(data any) (int, bool) {
	switch d := data.(type) {
	case provider.ToolCallDeltaData:
		if sc.toolBlockStarted {
			return sc.currentToolBlockIndex, true
		}
		return sc.blockIndex, true
	case provider.IndexedToolCallDeltaData:
		if d.HasIndex {
			if idx, ok := sc.toolBlockByProviderIndex[d.Index]; ok {
				return idx, true
			}
		}
		if sc.toolBlockStarted {
			return sc.currentToolBlockIndex, true
		}
		return sc.blockIndex, true
	default:
		return 0, false
	}
}

func toolDeltaPartialJSON(data any) (string, bool) {
	switch d := data.(type) {
	case provider.ToolCallDeltaData:
		return string(d), true
	case provider.IndexedToolCallDeltaData:
		return d.PartialJSON, true
	default:
		return "", false
	}
}

func (sc *StreamConverter) handleDelta(delta provider.StreamDelta[any]) []StreamEvent {
	switch delta.Type {
	case provider.StreamDeltaTypeBlockStart:
		data, ok := delta.Data.(provider.BlockStartData)
		if !ok {
			return nil
		}

		var events []StreamEvent

		switch mapBlockType(data.BlockType) {
		case ContentBlockText:
			if !sc.textBlockStarted {
				events = append(events, sc.stopForNewBlock(ContentBlockText)...)
				events = append(events, sc.startTextBlock())
			}
		case ContentBlockThinking:
			if !sc.thinkingBlockStarted {
				events = append(events, sc.stopForNewBlock(ContentBlockThinking)...)
				events = append(events, sc.startThinkingBlock())
			}
		case ContentBlockRedactedThinking:
			// redacted_thinking 的完整生命周期由 StreamDeltaTypeRedactedThinking 独立处理，
			// block_start 阶段不创建任何内容块，避免被错误映射为文本块。
			return nil
		case ContentBlockToolUse:
			events = append(events, sc.startToolBlock(provider.ToolCallData{ID: data.ID, Name: data.Name})...)
		default:
			if !sc.textBlockStarted {
				events = append(events, sc.stopForNewBlock(ContentBlockText)...)
				events = append(events, sc.startTextBlock())
			}
		}
		return events
	case provider.StreamDeltaTypeTextOutput:
		var events []StreamEvent
		if !sc.textBlockStarted {
			events = append(events, sc.stopForNewBlock(ContentBlockText)...)
			events = append(events, sc.startTextBlock())
		}
		textData, ok := delta.Data.(provider.TextData)
		if !ok {
			return events
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.textBlockIndex,
			Delta: &StreamDelta{Type: DeltaTextDelta, Text: string(textData)},
		})
		return events
	case provider.StreamDeltaTypeThinking:
		var events []StreamEvent
		if !sc.thinkingBlockStarted {
			events = append(events, sc.stopForNewBlock(ContentBlockThinking)...)
			events = append(events, sc.startThinkingBlock())
		}
		thinkingData, ok := delta.Data.(provider.ThinkingData)
		if !ok {
			return events
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.thinkingBlockIndex,
			Delta: &StreamDelta{Type: DeltaThinkingDelta, Thinking: string(thinkingData)},
		})
		return events
	case provider.StreamDeltaTypeSignature:
		// 防御性处理: omitted 模式下 Provider 只发 signature_delta 不发 thinking_delta，
		// 此时 thinkingBlockStarted 为 false。自动开启 thinking block 以保留 signature。
		var events []StreamEvent
		if !sc.thinkingBlockStarted {
			events = append(events, sc.stopForNewBlock(ContentBlockThinking)...)
			events = append(events, sc.startThinkingBlock())
		}
		sig, sigProv, ok := signatureDeltaFields(delta.Data)
		if !ok {
			return events
		}
		if sigProv == "" {
			sigProv = sc.signatureProvider
		}
		events = append(events, StreamEvent{
			Type:  EventContentBlockDelta,
			Index: sc.thinkingBlockIndex,
			Delta: &StreamDelta{Type: DeltaSignature, Signature: sig, SignatureProvider: sigProv},
		})
		return events
	case provider.StreamDeltaTypeToolCall:
		data, ok := delta.Data.(provider.ToolCallData)
		if !ok {
			return nil
		}
		return sc.startToolBlock(data)
	case provider.StreamDeltaTypeToolCallDelta:
		partialJSON, ok := toolDeltaPartialJSON(delta.Data)
		if !ok {
			return nil
		}
		idx, ok := sc.toolDeltaIndex(delta.Data)
		if !ok {
			return nil
		}
		return []StreamEvent{{
			Type:  EventContentBlockDelta,
			Index: idx,
			Delta: &StreamDelta{Type: DeltaInputJSON, PartialJSON: partialJSON},
		}}
	case provider.StreamDeltaTypeBlockStop:
		data, ok := delta.Data.(provider.BlockStopData)
		if !ok {
			return nil
		}
		return sc.handleBlockStop(data)
	case provider.StreamDeltaTypeUsage:
		data, ok := delta.Data.(provider.UsageData)
		if !ok {
			return nil
		}
		sc.usage = &Usage{
			InputTokens:              data.InputTokens,
			OutputTokens:             data.OutputTokens,
			CacheCreationInputTokens: data.CacheCreationInputTokens,
			CacheReadInputTokens:     data.CacheReadInputTokens,
			ReasoningTokens:          data.ReasoningTokens,
		}
		// 通过 Ping 事件携带 usage，确保流中断时 relay 层仍可提取 usage。
		// 不使用 EventMessageDelta 以避免在 finish_reason 之前产生携带 usage 的终止语义 chunk，
		// 导致部分客户端（如 Vercel AI SDK）误判流结束。
		// 最终 usage 统一由 handleStop() 在 message_delta 中输出。
		return []StreamEvent{{
			Type: EventPing,
			Usage: &Usage{
				InputTokens:              data.InputTokens,
				OutputTokens:             data.OutputTokens,
				CacheCreationInputTokens: data.CacheCreationInputTokens,
				CacheReadInputTokens:     data.CacheReadInputTokens,
				ReasoningTokens:          data.ReasoningTokens,
			},
		}}
	case provider.StreamDeltaTypeMetadata:
		data, ok := delta.Data.(provider.MetadataData)
		if !ok {
			return nil
		}
		if sc.metadata == nil {
			sc.metadata = &MessageDelta{}
		}
		if data.ResponseID != "" {
			sc.metadata.ResponseID = data.ResponseID
		}
		if data.ReasoningID != "" {
			sc.metadata.ReasoningID = data.ReasoningID
		}
		if data.EncryptedContent != "" {
			sc.metadata.EncryptedContent = data.EncryptedContent
		}
		// 立即发出无 StopReason 的 message_delta，让 Responses 出口能在
		// output_item.done 到达时写入真实 rs_ id / encrypted_content。
		md := *sc.metadata
		return []StreamEvent{{
			Type:  EventMessageDelta,
			Delta: &md,
		}}
	case provider.StreamDeltaTypeRedactedThinking:
		// redacted_thinking 作为独立内容块输出: 先关闭异类活跃块，再发出 block_start + block_stop。
		// redacted_thinking 是原子块（无增量），直接 start + stop 完成生命周期。
		rtData, ok := delta.Data.(provider.RedactedThinkingData)
		if !ok {
			return nil
		}
		var events []StreamEvent
		events = append(events, sc.stopAllTextOrThinking()...)
		idx := sc.nextIndex()
		events = append(events, StreamEvent{
			Type:         EventContentBlockStart,
			Index:        idx,
			ContentBlock: NewRedactedThinkingBlock(string(rtData)),
		})
		events = append(events, StreamEvent{
			Type:  EventContentBlockStop,
			Index: idx,
		})
		sc.stoppedBlockIndexes[idx] = true
		return events
	default:
		return nil
	}
}

// handleBlockStop 处理 BlockStop delta，立即输出 content_block_stop 事件。
//
// 当 Provider 发出 BlockStop delta（如 Anthropic 的 content_block_stop）时，
// 立即关闭对应的活跃 block，无需等待 handleStop 统一关闭。
// 通过 stoppedBlockIndexes 集合防止与 handleStop 产生重复的 content_block_stop。
func (sc *StreamConverter) handleBlockStop(data provider.BlockStopData) []StreamEvent {
	if !data.HasIndex {
		return nil
	}
	index := data.Index
	if sc.stoppedBlockIndexes[index] {
		return nil
	}

	var events []StreamEvent

	// 关闭该 index 对应的活跃 block
	// 1. tool block
	for i, idx := range sc.openToolBlockIndexes {
		if idx == index {
			events = append(events, StreamEvent{Type: EventContentBlockStop, Index: idx})
			sc.openToolBlockIndexes = append(sc.openToolBlockIndexes[:i], sc.openToolBlockIndexes[i+1:]...)
			break
		}
	}
	// 2. text block
	if sc.textBlockStarted && sc.textBlockIndex == index {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: index})
	}
	// 3. thinking block
	if sc.thinkingBlockStarted && sc.thinkingBlockIndex == index {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: index})
	}

	sc.stoppedBlockIndexes[index] = true
	return events
}

func (sc *StreamConverter) handleStop() []StreamEvent {
	sc.stopHandled = true
	usage := sc.usage
	if usage == nil {
		usage = &Usage{}
	}
	// 使用适配器提供的完成原因，若未提供则根据流状态推断。
	// 关键兜底：当流被中断（stream.Err）但已收到 tool_call 增量时，
	// openToolBlockIndexes 非空 → 推断为 tool_use，避免 agent loop 误判为正常结束。
	stopReason := sc.finishReason
	if stopReason == "" {
		if len(sc.openToolBlockIndexes) > 0 {
			stopReason = FinishReasonToolUse
		} else {
			stopReason = FinishReasonEndTurn
		}
	}

	var events []StreamEvent
	if sc.thinkingBlockStarted && !sc.stoppedBlockIndexes[sc.thinkingBlockIndex] {
		sc.thinkingBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: sc.thinkingBlockIndex})
	}
	if sc.textBlockStarted && !sc.stoppedBlockIndexes[sc.textBlockIndex] {
		sc.textBlockStarted = false
		events = append(events, StreamEvent{Type: EventContentBlockStop, Index: sc.textBlockIndex})
	}
	for _, idx := range sc.openToolBlockIndexes {
		if !sc.stoppedBlockIndexes[idx] {
			events = append(events, StreamEvent{Type: EventContentBlockStop, Index: idx})
		}
	}
	sc.openToolBlockIndexes = nil
	sc.toolBlockStarted = false
	sc.toolBlockByProviderIndex = nil
	sc.toolBlockByID = nil

	events = append(events,
		StreamEvent{Type: EventMessageDelta, Delta: sc.buildMessageDelta(stopReason), Usage: usage},
		StreamEvent{Type: EventMessageStop},
	)
	return events
}

// buildMessageDelta 构造 message_delta 事件携带的 MessageDelta，
// 合并 stopReason 和流式过程中累积的 metadata（ResponseID/ReasoningID/EncryptedContent）。
func (sc *StreamConverter) buildMessageDelta(stopReason FinishReason) *MessageDelta {
	md := &MessageDelta{StopReason: stopReason}
	if sc.metadata != nil {
		md.ResponseID = sc.metadata.ResponseID
		md.ReasoningID = sc.metadata.ReasoningID
		md.EncryptedContent = sc.metadata.EncryptedContent
	}
	return md
}

func mapBlockType(blockType string) ContentBlockType {
	switch blockType {
	case "text":
		return ContentBlockText
	case "thinking":
		return ContentBlockThinking
	case "redacted_thinking":
		return ContentBlockRedactedThinking
	case "tool_use":
		return ContentBlockToolUse
	default:
		return ContentBlockText
	}
}

func (sc *StreamConverter) handleError(err *pkgErrors.BambooError) []StreamEvent {
	events := []StreamEvent{{
		Type:  EventError,
		Error: err,
	}}

	// 流中断兜底：若尚未发出终止序列，自动补发。
	// 参考 Vercel AI SDK flush 机制 — 无论流是否出错，客户端都必须收到完整终止信号。
	if sc.started && !sc.stopHandled {
		events = append(events, sc.handleStop()...)
	}
	return events
}

func signatureDeltaFields(data any) (signature, signatureProvider string, ok bool) {
	switch v := data.(type) {
	case provider.SignatureData:
		return string(v), "", true
	case provider.SignatureDeltaData:
		return v.Signature, v.Provider, true
	case string:
		return v, "", true
	default:
		return "", "", false
	}
}

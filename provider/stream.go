package provider

import "github.com/bamboo-services/bamboo-base-go/common/error"

// StreamEvent 表示流处理管道中产生的单个离散事件或信号。
// 它封装了事件的基本分类信息、负载内容以及处理过程中可能产生的错误状态。
//
// 该结构体属于值类型，通常在多个 goroutine 之间通过 channel 进行传递，
// 因此其内部字段在传递完成后应被视为只读，本身不保证并发安全的直接修改。
type StreamEvent struct {
	Type         StreamType       `json:"type" xml:"type"`                                       // 表示事件的具体分类或信号类型，用于在流处理管道中指导下游的分支路由逻辑。
	Delta        StreamDelta[any] `json:"delta" xml:"delta"`                                     // 包含事件的实际负载数据，通常是一个字符串，可以是 AI 模型生成的文本、工具调用结果或其他相关信息。
	Err          *xError.Error    `json:"err" xml:"err"`                                         // 字段用于在事件处理过程中捕获和传递可能发生的错误状态，允许下游组件根据该错误信息进行适当的错误处理或日志记录。
	FinishReason FinishReason     `json:"finish_reason,omitempty" xml:"finish_reason,omitempty"` // 完成原因，仅在 StreamTypeStop 事件中由适配器填充，标识流结束的具体原因。
}

// StreamDelta 流增量数据，支持泛型以确保类型安全
type StreamDelta[E any] struct {
	Type StreamDeltaType `json:"type" xml:"type"` // 表示流增量事件的具体分类或信号类型，用于在流式交互过程中指导下游的分支路由逻辑。
	Data E               `json:"data" xml:"data"` // 包含流增量事件的实际负载数据，根据 Type 不同存储不同类型的内容
}

// StreamType 表示流处理管道中的事件类型。
//
// 用于区分流处理过程中的不同阶段，如开始、停止、完成、
// 错误和增量数据传输。
type StreamType string

const (
	StreamTypeStart StreamType = "start" // 流开始事件，表示流处理管道已建立连接并开始传输
	StreamTypeStop  StreamType = "stop"  // 流停止事件，表示流处理管道正常结束传输
	StreamTypeDone  StreamType = "done"  // 流完成事件，表示整个流处理会话已完全结束，可用于通知下游关闭资源
	StreamTypeError StreamType = "error" // 错误事件，表示流处理过程中发生了错误
	StreamTypeDelta StreamType = "delta" // 增量事件，表示流处理过程中产生的增量数据
)

// StreamDeltaType 表示在流式交互过程中特定数据或事件类型的分类标识。
//
// 区分文本输出、思考过程、工具调用、用量统计等不同类型的
// 流增量事件，用于指导下游的路由和处理逻辑。
type StreamDeltaType string

const (
	StreamDeltaTypeTextOutput    StreamDeltaType = "text_output"     // 文本输出事件，表示 AI 模型生成的文本响应
	StreamDeltaTypeThinking      StreamDeltaType = "thinking"        // 思考事件，表示 AI 模型的推理或思考过程内容（如 Claude 的 extended thinking）
	StreamDeltaTypeSignature     StreamDeltaType = "signature"       // 签名事件，表示推理内容的加密签名/密文（OpenAI encrypted_content / Anthropic signature）
	StreamDeltaTypeToolCall      StreamDeltaType = "tool_call"       // 工具调用事件，表示 AI 模型请求调用某个工具
	StreamDeltaTypeToolCallDelta StreamDeltaType = "tool_call_delta" // 工具调用增量事件，表示工具调用 JSON 参数的增量部分
	StreamDeltaTypeUsage         StreamDeltaType = "usage"           // 用量统计事件，表示本次对话的 Token 使用量统计信息
	StreamDeltaTypeBlockStart    StreamDeltaType = "block_start"     // 内容块开始事件，标记新内容块的起始
	StreamDeltaTypeBlockStop     StreamDeltaType = "block_stop"      // 内容块停止事件，标记内容块的结束
	StreamDeltaTypeMetadata      StreamDeltaType = "metadata"        // 元数据事件，携带响应 ID / reasoning ID 等非内容性信息
)

// ============================================
// DeltaData 类型定义 - 流增量数据的具体类型
// ============================================

// TextData 文本数据。
//
// 用于文本输出增量，表示 AI 模型生成的文本响应内容。
type TextData string

// ThinkingData 思考数据。
//
// 用于 AI 模型的推理过程内容，如 Claude 的 extended thinking。
type ThinkingData string

// SignatureData 推理签名数据。
//
// 用于推理内容的加密签名或密文：
// - Anthropic: extended thinking 的验证签名
// - OpenAI Responses: encrypted_content（服务端加密的不透明 token）
type SignatureData string

// ToolCallData 工具调用开始数据。
//
// 包含工具调用的唯一标识和工具名称，用于标记工具调用的开始。
type ToolCallData struct {
	ID       string `json:"id"`                  // 工具调用唯一标识
	Name     string `json:"name"`                // 工具名称
	Index    int    `json:"index,omitempty"`     // Provider 原生工具索引（OpenAI 并行工具调用使用）
	HasIndex bool   `json:"has_index,omitempty"` // 是否显式携带工具索引
}

// ToolCallDeltaData 工具调用增量数据。
//
// 包含 JSON 参数的增量部分，用于流式传输工具调用参数。
type ToolCallDeltaData string

// IndexedToolCallDeltaData 带 Provider 原生索引的工具调用参数增量数据。
type IndexedToolCallDeltaData struct {
	PartialJSON string `json:"partial_json"`        // JSON 参数增量片段
	Index       int    `json:"index,omitempty"`     // Provider 原生工具索引
	HasIndex    bool   `json:"has_index,omitempty"` // 是否显式携带工具索引
}

func (d IndexedToolCallDeltaData) String() string { return d.PartialJSON }

// UsageData Token 使用量统计数据。
//
// 记录本次对话的输入和输出 Token 数量，用于计费和用量分析。
// CacheCreationInputTokens 和 CacheReadInputTokens 用于追踪 prompt caching 的命中情况。
type UsageData struct {
	InputTokens              int64 `json:"input_tokens"`                          // 输入 Token 数量
	OutputTokens             int64 `json:"output_tokens"`                         // 输出 Token 数量
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"` // 缓存创建消耗的输入 token 数量（Anthropic）
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`     // 缓存命中读取的输入 token 数量（Anthropic/OpenAI/Gemini）
}

// BlockStartData 内容块开始数据。
//
// 标记新内容块（text/thinking/tool_use）的起始，包含类型、
// ID 和名称信息，用于内容块的边界识别。
type BlockStartData struct {
	BlockType string `json:"block_type"`     // 内容块类型: "text" | "thinking" | "tool_use"
	ID        string `json:"id,omitempty"`   // 内容块 ID（tool_use 时使用）
	Name      string `json:"name,omitempty"` // 工具名称（tool_use 时使用）
}

// BlockStopData 内容块停止数据。
//
// 标记内容块（text/thinking/tool_use）的结束，包含索引信息用于匹配对应的 block。
type BlockStopData struct {
	Index    int  `json:"index,omitempty"`     // 内容块索引，用于匹配对应的 block
	HasIndex bool `json:"has_index,omitempty"` // 是否显式携带索引
}

// MetadataData 元数据增量数据。
//
// 用于在流式过程中传递非内容性的元信息，例如：
// - ResponseID: OpenAI Responses 的响应 ID（response.created 事件携带）
// - ReasoningID: reasoning item 的唯一标识（output_item.done 事件携带，如 "rs_xxx"）
// - EncryptedContent: reasoning item 的加密内容（用于多轮对话保留推理上下文）
//
// 上层可通过此 Delta 提取 ResponseID 和 ReasoningID 用于后续请求的上下文关联。
type MetadataData struct {
	ResponseID       string `json:"response_id,omitempty"`       // 响应 ID（OpenAI Responses response.id）
	ReasoningID      string `json:"reasoning_id,omitempty"`      // reasoning item ID（如 "rs_xxx"）
	EncryptedContent string `json:"encrypted_content,omitempty"` // reasoning 加密内容（不透明 token）
}

// ============================================
// StreamDelta 构造函数 - 返回 StreamDelta[any] 以便统一使用
// ============================================

// NewTextDelta 创建文本增量事件。
//
// 参数:
//   - text - 文本内容
//
// 返回类型为 StreamDelta[any] 的文本增量事件。
func NewTextDelta(text string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeTextOutput,
		Data: TextData(text),
	}
}

// NewThinkingDelta 创建思考增量事件。
//
// 参数:
//   - thinking - 思考内容
//
// 返回类型为 StreamDelta[any] 的思考增量事件。
func NewThinkingDelta(thinking string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeThinking,
		Data: ThinkingData(thinking),
	}
}

// NewSignatureDelta 创建推理签名增量事件。
//
// 参数:
//   - signature - 签名/加密内容（Anthropic signature 或 OpenAI encrypted_content）
func NewSignatureDelta(signature string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeSignature,
		Data: SignatureData(signature),
	}
}

// NewToolCallDelta 创建工具调用开始事件。
//
// 参数:
//   - id - 工具调用唯一标识
//   - name - 工具名称
//
// 返回类型为 StreamDelta[any] 的工具调用增量事件。
func NewToolCallDelta(id, name string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeToolCall,
		Data: ToolCallData{
			ID:   id,
			Name: name,
		},
	}
}

// NewToolCallDeltaWithIndex 创建带 Provider 原生索引的工具调用开始事件。
func NewToolCallDeltaWithIndex(id, name string, index int) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeToolCall,
		Data: ToolCallData{
			ID:       id,
			Name:     name,
			Index:    index,
			HasIndex: true,
		},
	}
}

// NewToolCallDeltaData 创建工具调用增量事件。
//
// 参数:
//   - partialJSON - JSON 参数的增量部分
//
// 返回类型为 StreamDelta[any] 的工具调用增量事件。
func NewToolCallDeltaData(partialJSON string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeToolCallDelta,
		Data: ToolCallDeltaData(partialJSON),
	}
}

// NewToolCallDeltaDataWithIndex 创建带 Provider 原生索引的工具调用参数增量事件。
func NewToolCallDeltaDataWithIndex(partialJSON string, index int) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeToolCallDelta,
		Data: IndexedToolCallDeltaData{
			PartialJSON: partialJSON,
			Index:       index,
			HasIndex:    true,
		},
	}
}

// NewUsageDelta 创建用量统计事件。
//
// 参数:
//   - inputTokens - 输入 Token 数量
//   - outputTokens - 输出 Token 数量
//
// 返回类型为 StreamDelta[any] 的用量统计事件。
func NewUsageDelta(inputTokens, outputTokens int64) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeUsage,
		Data: UsageData{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}
}

func NewUsageDeltaWithCache(inputTokens, outputTokens, cacheCreation, cacheRead int64) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeUsage,
		Data: UsageData{
			InputTokens:              inputTokens,
			OutputTokens:             outputTokens,
			CacheCreationInputTokens: cacheCreation,
			CacheReadInputTokens:     cacheRead,
		},
	}
}

// NewBlockStartDelta 创建内容块开始事件。
//
// 参数:
//   - blockType - 内容块类型（"text" | "thinking" | "tool_use"）
//
// 返回类型为 StreamDelta[any] 的内容块开始事件。
func NewBlockStartDelta(blockType string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeBlockStart,
		Data: BlockStartData{
			BlockType: blockType,
		},
	}
}

// NewBlockStartDeltaWithID 创建带 ID 的内容块开始事件。
//
// 参数:
//   - blockType - 内容块类型（"text" | "thinking" | "tool_use"）
//   - id - 内容块 ID（tool_use 时使用）
//   - name - 工具名称（tool_use 时使用）
//
// 返回类型为 StreamDelta[any] 的内容块开始事件。
func NewBlockStartDeltaWithID(blockType, id, name string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeBlockStart,
		Data: BlockStartData{
			BlockType: blockType,
			ID:        id,
			Name:      name,
		},
	}
}

// NewBlockStopDelta 创建带索引的内容块停止事件。
//
// 参数:
//   - index - 内容块索引，用于匹配对应的 block
//
// 返回类型为 StreamDelta[any] 的内容块停止事件。
func NewBlockStopDelta(index int) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeBlockStop,
		Data: BlockStopData{
			Index:    index,
			HasIndex: true,
		},
	}
}

// NewBlockStopDeltaNoIndex 创建无索引的内容块停止事件。
//
// 返回类型为 StreamDelta[any] 的内容块停止事件。
func NewBlockStopDeltaNoIndex() StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeBlockStop,
		Data: BlockStopData{
			HasIndex: false,
		},
	}
}

// NewMetadataDelta 创建元数据增量事件。
//
// 用于在流式过程中传递响应 ID、reasoning ID 和加密内容等非内容性元信息。
//
// 参数:
//   - responseID - 响应 ID（可为空）
//   - reasoningID - reasoning item ID（可为空）
//   - encryptedContent - reasoning 加密内容（可为空）
func NewMetadataDelta(responseID, reasoningID, encryptedContent string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeMetadata,
		Data: MetadataData{
			ResponseID:       responseID,
			ReasoningID:      reasoningID,
			EncryptedContent: encryptedContent,
		},
	}
}

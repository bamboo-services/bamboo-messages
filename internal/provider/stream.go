package provider

import xError "github.com/bamboo-services/bamboo-base-go/common/error"

// StreamEvent 表示流处理管道中产生的单个离散事件或信号。
// 它封装了事件的基本分类信息、负载内容以及处理过程中可能产生的错误状态。
//
// 该结构体属于值类型，通常在多个 goroutine 之间通过 channel 进行传递，
// 因此其内部字段在传递完成后应被视为只读，本身不保证并发安全的直接修改。
type StreamEvent struct {
	Type  StreamType       `json:"type" xml:"type"`   // 表示事件的具体分类或信号类型，用于在流处理管道中指导下游的分支路由逻辑。
	Delta StreamDelta[any] `json:"delta" xml:"delta"` // 包含事件的实际负载数据，通常是一个字符串，可以是 AI 模型生成的文本、工具调用结果或其他相关信息。
	Err   *xError.Error    `json:"err" xml:"err"`     // 字段用于在事件处理过程中捕获和传递可能发生的错误状态，允许下游组件根据该错误信息进行适当的错误处理或日志记录。
}

// StreamDelta 流增量数据，支持泛型以确保类型安全
type StreamDelta[E any] struct {
	Type StreamDeltaType `json:"type" xml:"type"` // 表示流增量事件的具体分类或信号类型，用于在流式交互过程中指导下游的分支路由逻辑。
	Data E               `json:"data" xml:"data"` // 包含流增量事件的实际负载数据，根据 Type 不同存储不同类型的内容
}

type StreamType string

const (
	StreamTypeStart StreamType = "start" // 流开始事件，表示流处理管道已建立连接并开始传输
	StreamTypeStop  StreamType = "stop"  // 流停止事件，表示流处理管道正常结束传输
	StreamTypeDone  StreamType = "done"  // 流完成事件，表示整个流处理会话已完全结束，可用于通知下游关闭资源
	StreamTypeError StreamType = "error" // 错误事件，表示流处理过程中发生了错误
	StreamTypeDelta StreamType = "delta" // 增量事件，表示流处理过程中产生的增量数据
)

// StreamDeltaType 表示在流式交互过程中特定数据或事件类型的分类标识。
type StreamDeltaType string

const (
	StreamDeltaTypeTextOutput    StreamDeltaType = "text_output"     // 文本输出事件，表示 AI 模型生成的文本响应
	StreamDeltaTypeThinking      StreamDeltaType = "thinking"        // 思考事件，表示 AI 模型的推理或思考过程内容（如 Claude 的 extended thinking）
	StreamDeltaTypeToolCall      StreamDeltaType = "tool_call"       // 工具调用事件，表示 AI 模型请求调用某个工具
	StreamDeltaTypeToolCallDelta StreamDeltaType = "tool_call_delta" // 工具调用增量事件，表示工具调用 JSON 参数的增量部分
	StreamDeltaTypeUsage         StreamDeltaType = "usage"           // 用量统计事件，表示本次对话的 Token 使用量统计信息
)

// ============================================
// DeltaData 类型定义 - 流增量数据的具体类型
// ============================================

// TextData 文本数据，用于文本输出增量
type TextData string

// ThinkingData 思考数据，用于 AI 模型的推理过程内容
type ThinkingData string

// ToolCallData 工具调用开始数据，包含工具调用的基本信息
type ToolCallData struct {
	ID   string `json:"id"`   // 工具调用唯一标识
	Name string `json:"name"` // 工具名称
}

// ToolCallDeltaData 工具调用增量数据，包含 JSON 参数的增量部分
type ToolCallDeltaData string

// UsageData Token 使用量统计数据
type UsageData struct {
	InputTokens  int64 `json:"input_tokens"`  // 输入 Token 数量
	OutputTokens int64 `json:"output_tokens"` // 输出 Token 数量
}

// ============================================
// StreamDelta 构造函数 - 返回 StreamDelta[any] 以便统一使用
// ============================================

// NewTextDelta 创建文本增量事件
func NewTextDelta(text string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeTextOutput,
		Data: TextData(text),
	}
}

// NewThinkingDelta 创建思考增量事件
func NewThinkingDelta(thinking string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeThinking,
		Data: ThinkingData(thinking),
	}
}

// NewToolCallDelta 创建工具调用开始事件
func NewToolCallDelta(id, name string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeToolCall,
		Data: ToolCallData{
			ID:   id,
			Name: name,
		},
	}
}

// NewToolCallDeltaData 创建工具调用增量事件
func NewToolCallDeltaData(partialJSON string) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeToolCallDelta,
		Data: ToolCallDeltaData(partialJSON),
	}
}

// NewUsageDelta 创建用量统计事件
func NewUsageDelta(inputTokens, outputTokens int64) StreamDelta[any] {
	return StreamDelta[any]{
		Type: StreamDeltaTypeUsage,
		Data: UsageData{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}
}

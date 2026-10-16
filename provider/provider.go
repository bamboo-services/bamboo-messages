package provider

import (
	"context"
)

// Provider AI 对话协议适配器的核心接口
//
// 该接口定义了与 AI 模型交互的统一方式，支持多种 AI 对话协议
// （如 Anthropic Messages 协议、OpenAI Chat Completions 协议、OpenAI Responses 协议等）。
// 每个实现可独立配置目标端点，从而对接任意兼容该协议的服务。
type Provider interface {
	// Chat 发起流式对话并返回只读事件 channel。
	//
	// 参数：
	//   - ctx: 控制请求生命周期的上下文，超时或取消时底层连接立即中止并关闭 channel；
	//   - messages: 按时间顺序排列的对话历史；
	//   - config: 请求配置（如模型、温度、工具声明等），传 nil 时使用默认配置。
	//
	// 契约保证：
	//   - 返回的 channel 在流结束（正常停止或发生错误）后由 Provider 负责关闭；
	//   - 首个文本增量前必须产生 BlockStart 事件；
	//   - 发生网络或协议错误时，必须发出包含具体状态码的 StreamTypeError 事件。
	Chat(ctx context.Context, messages []Message, config *ChatConfig) <-chan StreamEvent

	// ChatWithSystem 带系统提示词的便捷流式对话。
	//
	// 内部会在 messages 最前方自动追加一条包含 systemPrompt 的 System 消息后调用 [Provider.Chat]。
	ChatWithSystem(ctx context.Context, systemPrompt string, messages []Message, config *ChatConfig) <-chan StreamEvent

	// Complete 发起同步对话并等待完整响应返回。
	//
	// 适用于无需流式实时渲染的批处理、工具调用单步执行或离线推理场景。
	// 请求失败时返回具体的错误类型（推荐使用 [errors.As] 提取 [*pkgErrors.BambooError]）。
	Complete(ctx context.Context, messages []Message, config *ChatConfig) (*CompletionResult, error)

	// CompleteWithSystem 带系统提示词的便捷同步对话。
	CompleteWithSystem(ctx context.Context, systemPrompt string, messages []Message, config *ChatConfig) (*CompletionResult, error)

	// GetProviderType 返回当前 Provider 的协议类型标识（如 [ProviderAnthropic]、[ProviderOpenAICompletions]）。
	GetProviderType() ProviderType

	// GetAvailableModels 获取该提供商支持的已知推荐模型标识列表（开放端点可返回空列表）。
	GetAvailableModels() []string
}

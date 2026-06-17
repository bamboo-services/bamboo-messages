package provider

import (
	"context"
)

// BaseProvider 是所有协议适配器实现的通用基础结构体，封装了协议特定的底层客户端实例。
//
// 该结构体通过泛型参数 T 允许嵌入不同类型的底层客户端，不支持并发安全的直接修改，
// 通常作为匿名结构体嵌套在各个具体的协议适配器实现中以复用公共字段。
//
// Client 底层协议客户端，用于与目标端点进行通信和流式数据交互。
// 目标端点可以是官方 API、自建网关、代理服务或任何兼容第三方。
type BaseProvider[T any] struct {
	Client T `json:"client"` // 请求客户端 SDK
}

// Provider AI 对话协议适配器的核心接口
//
// 该接口定义了与 AI 模型交互的统一方式，支持多种 AI 对话协议
// （如 Anthropic Messages 协议、OpenAI Chat Completions 协议、OpenAI Responses 协议等）。
// 每个实现可独立配置目标端点，从而对接任意兼容该协议的服务。
type Provider interface {
	// Chat 流式对话
	//
	// messages: 对话历史（按时间顺序）
	// config: 请求配置（可选字段使用默认值）
	//
	// 返回流事件 channel，调用方通过 range 遍历接收 StreamEvent
	Chat(ctx context.Context, messages []Message, config *ChatConfig) <-chan StreamEvent

	// ChatWithSystem 带系统提示的流式对话
	//
	// systemPrompt: 系统提示词，用于设定 AI 的角色和行为
	// messages: 对话历史（不含 system 消息）
	// config: 请求配置
	//
	// 这是 Chat 的便捷方法，会自动在 messages 前添加 system 消息
	ChatWithSystem(ctx context.Context, systemPrompt string, messages []Message, config *ChatConfig) <-chan StreamEvent

	// Complete 同步对话，返回完整响应
	//
	// messages: 对话历史（按时间顺序）
	// config: 请求配置
	//
	// 返回完整的 CompletionResult，适用于不需要流式输出的场景
	Complete(ctx context.Context, messages []Message, config *ChatConfig) (*CompletionResult, error)

	// CompleteWithSystem 带系统提示的同步对话
	//
	// systemPrompt: 系统提示词，用于设定 AI 的角色和行为
	// messages: 对话历史（不含 system 消息）
	// config: 请求配置
	//
	// 这是 Complete 的便捷方法，会自动在 messages 前添加 system 消息
	CompleteWithSystem(ctx context.Context, systemPrompt string, messages []Message, config *ChatConfig) (*CompletionResult, error)

	// GetProviderType 获取提供商类型
	//
	// 返回当前 Provider 的类型标识，用于日志和调试
	GetProviderType() ProviderType

	// GetAvailableModels 获取可用模型列表
	//
	// 返回该提供商支持的所有模型名称
	GetAvailableModels() []string
}

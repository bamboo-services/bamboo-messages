package provider

import (
	"context"
)

// BaseProvider 是所有具体 AI 服务提供商实现的通用基础结构体，封装了特定于厂商的请求客户端 SDK 实例。
//
// 该结构体通过泛型参数 T 允许嵌入不同类型的底层客户端，不支持并发安全的直接修改，
// 通常作为匿名结构体嵌套在各个具体的提供商实现中以复用公共字段。
//
// Client 请求客户端 SDK，用于与具体的 AI 服务端点进行通信和流式数据交互。
type BaseProvider[T any] struct {
	Client T `json:"client"` // 请求客户端 SDK
}

// Provider AI 服务提供商的核心接口
//
// 该接口定义了与 AI 模型交互的统一方式，支持多种 AI 服务提供商
// （如 Anthropic/Claude、OpenAI、DeepSeek、通义千问等）
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

	// GetProviderType 获取提供商类型
	//
	// 返回当前 Provider 的类型标识，用于日志和调试
	GetProviderType() ProviderType

	// GetAvailableModels 获取可用模型列表
	//
	// 返回该提供商支持的所有模型名称
	GetAvailableModels() []string
}

// Package bamboo 提供 Bamboo Messages SDK 的统一客户端接口与多模态消息模型。
//
// 本包是 Bamboo Messages 面向上层业务的统一门面（Facade），通过标准化的抽象屏蔽底层
// Anthropic Messages、OpenAI Chat Completions、OpenAI Responses、Google Gemini 等主流
// AI 协议差异，实现上层业务零改动无缝切换 AI 后端。本包纯基于 Go 标准库构建，零外部重量级依赖。
//
// # 核心类型与调用路径
//
// [BambooClient] 是面向调用方的核心交互接口，定义流式对话与非流式对话两种交互模式：
//
//   - [BambooClient.Chat]：发起流式请求，返回 [StreamEvent] 的单向通道，支持文本、思维、签名、工具调用参数实时增量消费；
//   - [BambooClient.Complete]：发起非流式请求，等待模型输出聚合完成后返回结构化的 [*Response]。
//
// 客户端实例通常通过 [NewClient] 或 [NewClientWithOptions] 创建：
//
//	client := bamboo.NewClient(providerInstance)
//
// # 消息与多模态模型
//
// 对话历史通过 [BambooMessage] 切片组织，单条消息支持混合多个多态内容块 [ContentBlock]：
//
//   - [TextBlock]：普通文本内容；
//   - [ThinkingBlock]：模型推理与思考过程（支持保留原始加密签名 [ThinkingBlock.Signature] 与服务商来源）；
//   - [ToolUseBlock]：模型发起的函数/工具调用请求；
//   - [ToolResultBlock]：调用方执行工具后的回填结果，支持同时指定 ToolName 与 ToolCallID 并标记 IsError；
//   - [ImageBlock] 与 [DocumentBlock]：内联或 URL 多模态富媒体块。
//
// 本包提供各种便捷构造函数，如 [NewUserMessage]、[NewAssistantMessage]、[NewToolResultMessage] 等。
//
// # 典型用法
//
// 以下展示创建客户端并发起流式对话的完整过程：
//
//	func runChat(ctx context.Context, p provider.Provider) error {
//		client := bamboo.NewClient(p)
//		messages := []bamboo.BambooMessage{
//			bamboo.NewUserMessage("你好，请介绍一下你自己。"),
//		}
//		config := &bamboo.RequestConfig{
//			Model:       "claude-sonnet-4-20250514",
//			Temperature: bamboo.PtrFloat64(0.7),
//			MaxTokens:   1024,
//		}
//
//		eventCh, err := client.Chat(ctx, messages, "你是一个得力的 AI 助手。", config)
//		if err != nil {
//			return err
//		}
//
//		for event := range eventCh {
//			switch event.Type {
//			case bamboo.EventContentBlockDelta:
//				if delta, ok := event.Delta.(*bamboo.StreamDelta); ok && delta.Type == bamboo.DeltaTextDelta {
//					fmt.Print(delta.Text)
//				}
//			case bamboo.EventError:
//				if event.Error != nil {
//					return event.Error
//				}
//			}
//		}
//		return nil
//	}
//
// # 错误处理与透传
//
// 所有业务异常与上游端点错误均统一封装为 [*bamboo.BambooError]（来源于 [pkgErrors.BambooError]）。
// 包含 Category（错误分类，如 "上游" / "参数" / "下游"）、Message（具体诊断信息）与 StatusCode（HTTP 状态码）。
// 调用方可通过 [errors.As] 解包捕获精确的状态码与原因。
//
// # 并发与生命周期
//
// [BambooClient] 的实现是并发安全的，单个客户端实例可在多个 goroutine 中并发发起请求。
// [BambooMessage] 与 [StreamEvent] 为不可变值对象，通过 channel 传递后调用方应视为只读。
// 所有请求受传入的 [context.Context] 控制，支持随时取消或超时中断。
package bamboo

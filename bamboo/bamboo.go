// Package bamboo 提供 Bamboo Messages SDK 的公共类型系统与客户端接口。
//
// bamboo 包是 Bamboo Messages 的上层门面（Facade），屏蔽底层 AI 协议差异，
// 为上层业务提供统一的消息模型、流事件、工具定义和配置类型。
// 该包零外部 SDK 依赖，仅依赖 Go 标准库。
package bamboo

import (
	"context"
	"fmt"

	"github.com/bamboo-services/bamboo-messages/provider"
)

// BambooClient Bamboo Messages SDK 核心接口，定义流式对话和非流式对话两种交互模式。
//
// 所有方法均接收 context.Context 以支持取消和超时控制。
// system 参数为系统提示词，可为空字符串；config 参数为可选的请求配置，传 nil 使用默认值。
type BambooClient interface {
	// Chat 发起流式对话，通过 channel 逐步返回 StreamEvent。
	//
	// 返回的 channel 会在流结束后自动关闭。调用方应遍历 channel 处理每个事件。
	Chat(ctx context.Context, messages []BambooMessage, system string, config *RequestConfig) (<-chan StreamEvent, error)

	// Complete 发起非流式对话，等待完整响应后返回。
	Complete(ctx context.Context, messages []BambooMessage, system string, config *RequestConfig) (*Response, error)
}

// client SDK 客户端实现，封装 provider.Provider 进行协议适配。
//
// client 内部持有 provider.Provider 实例，将上层 BambooMessage / RequestConfig
// 转换为 provider 层的类型，并将 provider 层的响应结果转换回 bamboo 层的类型。
type client struct {
	provider     provider.Provider
	defaultModel string
}

// 确保 client 在编译期满足 BambooClient 接口。
var _ BambooClient = (*client)(nil)

// NewClient 创建 BambooClient 实例。
//
// p 为底层协议适配器，不可为 nil，否则 panic。
func NewClient(p provider.Provider) BambooClient {
	if p == nil {
		panic("bamboo: provider must not be nil")
	}
	return &client{provider: p}
}

// Chat 发起流式对话。
//
// 将 BambooMessage 转换为 provider.Message，根据 system 是否为空选择调用
// ChatWithSystem 或 Chat，然后在独立 goroutine 中将 provider.StreamEvent
// 转换为 bamboo.StreamEvent 发送到输出 channel。
// channel 会在流结束后自动关闭，或当 ctx 被取消时提前关闭。
func (c *client) Chat(ctx context.Context, messages []BambooMessage, system string, config *RequestConfig) (<-chan StreamEvent, error) {
	// 转换消息
	providerMsgs, err := messagesToProvider(messages)
	if err != nil {
		return nil, err
	}

	// 转换配置
	providerConfig := configToProvider(config)

	// 根据是否携带系统提示词选择对应方法
	var providerCh <-chan provider.StreamEvent
	if system != "" {
		providerCh = c.provider.ChatWithSystem(ctx, system, providerMsgs, providerConfig)
	} else {
		providerCh = c.provider.Chat(ctx, providerMsgs, providerConfig)
	}

	// 创建 bamboo 输出 channel 并启动转换 goroutine
	out := make(chan StreamEvent)
	converter := NewStreamConverter()

	go func() {
		defer close(out)
		for event := range providerCh {
			// 检查上下文取消
			select {
			case <-ctx.Done():
				out <- StreamEvent{
					Type:  EventError,
					Error: NewBambooError(ErrorTypeProvider, ctx.Err().Error()),
				}
				return
			default:
			}

			// 转换 provider 事件 → bamboo 事件
			bambooEvents := converter.Convert(event)
			for _, be := range bambooEvents {
				select {
				case out <- be:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// Complete 发起非流式对话，等待完整响应后返回。
//
// 将 BambooMessage 转换为 provider.Message，根据 system 是否为空选择调用
// CompleteWithSystem 或 Complete，最后将结果转换为 bamboo.Response。
func (c *client) Complete(ctx context.Context, messages []BambooMessage, system string, config *RequestConfig) (*Response, error) {
	// 转换消息
	providerMsgs, err := messagesToProvider(messages)
	if err != nil {
		return nil, err
	}

	// 转换配置
	providerConfig := configToProvider(config)

	// 根据是否携带系统提示词选择对应方法
	var result *provider.CompletionResult
	if system != "" {
		result, err = c.provider.CompleteWithSystem(ctx, system, providerMsgs, providerConfig)
	} else {
		result, err = c.provider.Complete(ctx, providerMsgs, providerConfig)
	}
	if err != nil {
		// 兜底：即使出错，如果 result 非 nil（Provider 返回了部分结果），仍转换为 Response 返回，
		// 上层可通过 resp != nil && err != nil 判断是"部分成功"还是完全失败。
		if result != nil {
			providerType := string(c.provider.GetProviderType())
			resp := resultToResponse(result, providerType)
			return resp, fmt.Errorf("bamboo: complete failed: %w", err)
		}
		return nil, fmt.Errorf("bamboo: complete failed: %w", err)
	}

	// 转换结果
	providerType := string(c.provider.GetProviderType())
	return resultToResponse(result, providerType), nil
}

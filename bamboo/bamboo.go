// Package bamboo 提供 Bamboo Messages SDK 的公共类型系统与客户端接口。
//
// bamboo 包是 Bamboo Messages 的上层门面（Facade），屏蔽底层 AI 协议差异，
// 为上层业务提供统一的消息模型、流事件、工具定义和配置类型。
// 该包零外部 SDK 依赖，仅依赖 Go 标准库。
package bamboo

import (
	"context"
	"fmt"

	"github.com/bamboo-services/bamboo-messages/internal/xerr"
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
//
// 连接级错误同步返回：若 provider 的首个事件为 Error（如上游 400 拒绝），
// 直接返回 (nil, error)，不启动 SSE 流，避免客户端收到空流一直等待。
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

	// Peek 首个 provider 事件：若为连接级 Error（无 Start 前导），同步返回错误
	var firstEvent provider.StreamEvent
	var hasFirst bool
	select {
	case firstEvent, hasFirst = <-providerCh:
	case <-ctx.Done():
		return nil, fmt.Errorf("bamboo: chat cancelled: %w", ctx.Err())
	}
	if !hasFirst {
		return nil, fmt.Errorf("bamboo: provider stream closed before any event")
	}
	if firstEvent.Type == provider.StreamTypeError && firstEvent.Err != nil {
		return nil, fmt.Errorf("bamboo: provider chat failed: %w", firstEvent.Err)
	}

	// 创建 bamboo 输出 channel 并启动转换 goroutine
	// buffer=64 容纳完整工具调用序列（start+block_start+deltas+stop+done），
	// 防止消费端因 HTTP write 阻塞导致 out channel 满后终止事件被丢弃。
	out := make(chan StreamEvent, 64)
	converter := NewStreamConverter()

	go func() {
		defer close(out)

		// 先处理已 peek 的首事件
		for _, be := range converter.Convert(firstEvent) {
			select {
			case out <- be:
			case <-ctx.Done():
				return
			}
		}

		for event := range providerCh {
			select {
			case <-ctx.Done():
				cancelErr := xerr.NewError(context.Background(), nil,
					fmt.Sprintf("bamboo: chat cancelled: %s", ctx.Err()), false)
				for _, be := range converter.Convert(provider.StreamEvent{
					Type: provider.StreamTypeError,
					Err:  cancelErr,
				}) {
					select {
					case out <- be:
					default:
					}
				}
				return
			default:
			}

			bambooEvents := converter.Convert(event)
			for _, be := range bambooEvents {
				select {
				case out <- be:
				case <-ctx.Done():
					return
				}
			}
		}

		// provider channel 正常关闭（未收到 Done），补发终止信号。
		// Convert 内部有幂等防御（stopHandled），重复调用安全。
		for _, be := range converter.Convert(provider.StreamEvent{Type: provider.StreamTypeDone}) {
			select {
			case out <- be:
			default:
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

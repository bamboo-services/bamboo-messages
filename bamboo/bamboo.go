// Package bamboo 提供 Bamboo Messages SDK 的公共类型系统与客户端接口。
//
// bamboo 包是 Bamboo Messages 的上层门面（Facade），屏蔽底层 AI 协议差异，
// 为上层业务提供统一的消息模型、流事件、工具定义和配置类型。
// 该包零外部 SDK 依赖，仅依赖 Go 标准库。
package bamboo

import (
	"context"
	"errors"
	"fmt"
	"time"

	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
	"github.com/bamboo-services/bamboo-messages/provider"
)

// pingIdleInterval provider 空闲超过此时长后，SDK 自动向消费端发送 EventPing 保活。
//
// 用于防止反向代理（Nginx/Cloudflare/ALB）在 LLM thinking 等长空闲阶段
// 因 idle timeout 关闭连接。10s 与 newapi 原生路径 DefaultPingInterval 对齐。
const pingIdleInterval = 10 * time.Second

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
		panic("bamboo: provider 不能为空")
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
		return nil, pkgErrors.NewBambooErrorWithCause("SDK", "对话已取消: "+ctx.Err().Error(), 0, ctx.Err())
	}
	if !hasFirst {
		return nil, pkgErrors.NewBambooError("SDK", "provider 流在发出任何事件前已关闭", 0)
	}
	if firstEvent.Type == provider.StreamTypeError && firstEvent.Err != nil {
		return nil, firstEvent.Err
	}
	if firstEvent.Type == provider.StreamTypeDone {
		return nil, pkgErrors.NewBambooError("SDK", "provider 流已关闭但未产生任何内容", 0)
	}

	// 创建 bamboo 输出 channel 并启动转换 goroutine
	// buffer=64 吸收短突发流量（Preto.ai 5000+ req/s 生产验证）。
	// 终止事件（message_stop 等）通过 select on ctx.Done() 保障写入，
	// 既不会被 default 丢弃，也不会因固定超时在消费者背压时被静默吞掉。
	out := make(chan StreamEvent, 64)
	converter := NewStreamConverterForProvider(c.provider.GetProviderType())

	go func() {
		defer close(out)

		// panic 时尝试发送 error 事件，防止消费者无限等待
		defer func() {
			if r := recover(); r != nil {
				panicMsg := fmt.Sprintf("bamboo: 内部 panic: %v", r)
				select {
				case out <- StreamEvent{
					Type:  EventError,
					Error: pkgErrors.NewBambooError("SDK", panicMsg, 0),
				}:
				default:
				}
			}
		}()

		pingTicker := time.NewTicker(pingIdleInterval)
		defer pingTicker.Stop()

		writeEvent := func(be StreamEvent) bool {
			select {
			case out <- be:
				pingTicker.Reset(pingIdleInterval)
				return true
			case <-ctx.Done():
				return false
			}
		}

		writeAll := func(events []StreamEvent) bool {
			for _, be := range events {
				if !writeEvent(be) {
					return false
				}
			}
			return true
		}

		if !writeAll(converter.Convert(firstEvent)) {
			return
		}

		for {
			select {
			case <-pingTicker.C:
				if !writeEvent(StreamEvent{Type: EventPing}) {
					return
				}

			case event, ok := <-providerCh:
				if !ok {
					writeAll(converter.Convert(provider.StreamEvent{Type: provider.StreamTypeDone}))
					return
				}

				select {
				case <-ctx.Done():
					cancelErr := pkgErrors.NewBambooErrorWithCause("SDK", "对话已取消: "+ctx.Err().Error(), 0, ctx.Err())
					writeAll(converter.Convert(provider.StreamEvent{
						Type: provider.StreamTypeError,
						Err:  cancelErr,
					}))
					return
				default:
				}

				if !writeAll(converter.Convert(event)) {
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
		if result != nil {
			providerType := string(c.provider.GetProviderType())
			resp := resultToResponse(result, providerType)
			return resp, wrapProviderError(err)
		}
		return nil, wrapProviderError(err)
	}

	// 转换结果
	providerType := string(c.provider.GetProviderType())
	return resultToResponse(result, providerType), nil
}

// wrapProviderError 将 Provider 返回的错误包装为 BambooError。
//
// 若错误链中已包含 *BambooError，则直接透传，避免重复包装；
// 否则降级为通用 SDK 错误。
func wrapProviderError(err error) *BambooError {
	var bambooErr *BambooError
	if errors.As(err, &bambooErr) {
		return bambooErr
	}
	return pkgErrors.NewBambooErrorWithCause("SDK", err.Error(), 0, err)
}

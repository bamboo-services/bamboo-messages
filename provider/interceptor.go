package provider

import (
	"context"
)

// ============================================
// 请求拦截器（RequestInterceptor）
// ============================================

// RequestInterceptor 请求拦截器函数类型。
//
// 在 Provider 发起上游 HTTP 请求之前，对已序列化为字节流的请求体进行任意修改。
// 拦截器拿到的是 marshal 完成的上游请求 body（JSON 字节流），返回处理后的 body；
// 返回 error 将立即中止本次请求并向上冒泡到调用方，由调用方决定如何转换错误。
//
// 设计意图：
//   - 为 SDK 用户提供正交的「请求改写」扩展点，无需 fork Provider 实现
//   - 让上层应用（如 new-api）可以复用已有的参数覆盖逻辑（如 ApplyParamOverrideWithRelayInfo）
//     而不必在 SDK 内部重新实现一套覆盖语义
//   - 保持 SDK 中立性：本类型不引入任何业务依赖，body 对拦截器而言只是 []byte
//
// 调用契约：
//   - ctx：承自 Provider.Chat/Complete 的 context，支持超时、取消与链路追踪
//   - body：已序列化的上游请求体；可能为 nil（具体取决于上游 Provider 实现）
//   - 返回值：处理后的 body（必须非 nil 若想上游正常发送）；error 不为 nil 时中止请求
//
// 并发与重入：
//   - 拦截器实现需自行保证线程安全（同一拦截器可能在多个 goroutine 并发调用）
//   - 拦截器本身可以是闭包，但闭包捕获的可变状态需加锁保护
type RequestInterceptor func(ctx context.Context, body []byte) ([]byte, error)

// ApplyInterceptors 按注册顺序依次应用拦截器链。
//
// 前一个拦截器的输出（返回的 body）作为后一个拦截器的输入；
// 任意一个拦截器返回 error 立即停止链式调用，原样向上冒泡该 error。
//
// 当 interceptors 为 nil 或空切片时，原样返回 body（含 nil body 场景），
// 且不产生任何副作用——这是「未注入拦截器时零行为变化」的硬契约，
// 保证 SDK 升级后不注册拦截器的既有调用方完全无感知。
//
// 该函数是 Provider 内部使用的 helper，业务代码通常通过 WithInterceptor option
// 注册拦截器，而非直接调用本函数。
func ApplyInterceptors(ctx context.Context, body []byte, interceptors []RequestInterceptor) ([]byte, error) {
	// 无拦截器：原样返回，零开销
	if len(interceptors) == 0 {
		return body, nil
	}
	// 链式应用：前一个的输出是后一个的输入
	var err error
	for _, interceptor := range interceptors {
		if interceptor == nil {
			// 防御性：跳过 nil 槽位，不 panic
			continue
		}
		body, err = interceptor(ctx, body)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

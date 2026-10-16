// Package errors 提供 Bamboo Messages SDK 的统一错误模型与构造方法。
//
// 本包定义了跨协议、跨适配器统一使用的错误类型 [BambooError]，消除多层适配调用链中
// 错误信息被静默截断、吞掉或重复包装的问题，保持错误语义对上层业务的透明透传。
//
// # 错误模型
//
// [*BambooError] 包含三项核心属性：
//
//   - Category：错误分类字符串（如 "上游"、"下游"、"参数"、"SDK"、"协议" 等）；
//   - Message：人类可读的详细错误诊断信息；
//   - StatusCode：关联的 HTTP 状态码（如 400、401、404、429、500 等；无 HTTP 映射时为 0）。
//
// # 错误链穿透
//
// [*BambooError] 实现了标准库的 [BambooError.Unwrap] 方法，能够保留底层的原始错误 cause。
// 当因网络中断或客户端主动中止导致 [context.Canceled] 时，上层代码可通过 [errors.Is]
// 穿透捕获原始上下文错误，无需进行字符串匹配。
package errors

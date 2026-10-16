// Package codec 提供 N-to-N 协议编解码抽象与全局编解码器注册表。
//
// 本包是跨协议中继架构中的格式转换基石。通过解耦协议表示与执行逻辑，将各异构平台的
// 原生请求 JSON 抽象为统一的 [RelayRequest]，并支持将内部结果或错误原路投影回目标协议格式。
//
// # 核心接口与类型
//
//   - [Codec]：无状态的协议编解码器接口，负责请求解析（[Codec.ParseRequest]）、响应序列化（[Codec.SerializeResponse]）、错误格式化（[Codec.SerializeError]）以及有状态流序列化器构造；
//   - [StreamSerializer]：有状态的流式序列化器，用于在流式交互中按协议状态机将 [bamboo.StreamEvent] 转换为对应的 SSE 数据帧（如 data: 行或 JSON 帧）；
//   - [RelayRequest]：统一的跨协议请求中间表示，承载消息切片、系统提示词、多模态内容与统一配置 [bamboo.RequestConfig]；
//   - [FormatType]：协议格式枚举标识，内置常量包括 [FormatOpenAI]、[FormatAnthropic]、[FormatResponses]、[FormatGemini] 与 [FormatBamboo]。
//
// # 全局注册表
//
// 各子包（如 codec/openai、codec/anthropic 等）在 init() 阶段通过 [Register] 自主注册到包级表。
// 上层中继逻辑通过 [Get] 安全获取目标协议的编解码器：
//
//	c, err := codec.Get(codec.FormatOpenAI)
//	if err != nil {
//		// 协议未注册处理
//	}
//
// # 并发与状态管理
//
// [Codec] 接口实现要求是无状态且并发安全的，单例可在多个 goroutine 间无锁共享。
// 流式状态由每次调用 [Codec.NewSerializer] 生成的独立 [StreamSerializer] 实例维护，
// 每个流独占一个序列化器实例，禁止跨流混用。
package codec

// Package helpers 提供通用的基础类型指针转换与 ProviderExtra 安全取值工具函数。
//
// 在 AI 对话协议配置中，常见配置项（如温度、最大 Token、核采样等）需要区分“未设置”与“零值”（如温度为 0.0）。
// 本包提供优雅的指针包装函数，同时提供防止 JSON map 裸类型断言 panic 的安全读取助手。
//
// # 核心工具
//
//   - 指针辅助：[PtrFloat64]、[PtrBool]、[PtrInt64]、[PtrString]；
//   - 安全取值：[GetExtraFloat64]、[GetExtraInt64]、[GetExtraString]、[GetExtraBool]、[GetExtraAny]，在键不存在或类型不匹配时安全返回零值与 false。
package helpers

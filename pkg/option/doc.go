// Package option 提供跨 Provider 适配器复用的通用 Functional Options 模式基座。
//
// 本包抽离出绝大多数 AI 协议适配器均需具备的通用网络与凭证配置能力（API Key、BaseURL、自定义 Headers），
// 使各个独立适配器包（如 provider/openai、provider/gemini 等）保持对外统一的调用规范，减少样板代码。
//
// # 核心类型与选项
//
//   - [Config]：通用配置载体，包含 APIKey、BaseURL 与 Headers 映射表；
//   - [Option]：配置选项函数签名 func(*Config)；
//   - [WithAPIKey]：配置身份验证凭证；
//   - [WithBaseURL]：重定向基础 URL（常用于对接反向代理或第三方兼容网关）；
//   - [WithHeader]：追加自定义 HTTP 请求头（如监控 TraceID、租户标识等）；
//   - [ApplyOptions]：合并默认值与传入选项列表。
package option

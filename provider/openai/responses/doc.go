// Package responses 实现面向 OpenAI Responses 规范的全新 Provider 适配器。
//
// Responses API 是 OpenAI 推出的下一代有状态对话协议。本包全面支持其多轮会话关联、
// 原生双轨推理（Summary + Raw Reasoning）、工具调用与上下文生命周期管理。
//
// # 传输模式支持
//
// 本包支持两种互通的底层传输机制：
//
//   - 标准 HTTP/SSE 模式：通过 /responses 端点发送 POST 请求，使用 SSEScanner 监听 response.* 系列事件；
//   - 原生 WebSocket 持久连接模式：通过 WithWebSocketMode(true) 或配置 wss:// URL 启用，建立双向 WS 会话，发送 response.create 帧并持续双工通信。
//
// # 构造入口
//
//   - [NewResponsesProvider]：最简构造函数；
//   - [NewResponsesProviderWithOptions]：完整构造函数，支持配置 BaseURL、WebSocket 模式、请求头与拦截器。
package responses

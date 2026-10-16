// Package ws 提供零外部依赖、纯基于 Go 标准库实现的 RFC 6455 WebSocket 协议核心。
//
// 本包为 Bamboo Messages SDK 与中继网关提供轻量级的双向 WebSocket 通信支持，
// 避免引入大型第三方 WebSocket 框架，保持 SDK 零重依赖的设计约束。
//
// # 核心 API 与能力
//
//   - [Dial]：客户端连接拨号器。支持 ws:// 与 wss:// 协议（支持 TLS 加密与代理设置），在服务端返回非 101 状态码时返回包含具体 HTTP 状态与响应体的 [*HandshakeError]；
//   - [Upgrade]：服务端协议升级器。通过 [http.Hijacker] 劫持底层 TCP 连接，验证 Sec-WebSocket-Key 并完成标准握手；
//   - [Conn]：WebSocket 双向连接会话抽象。
//
// # 帧协议支持
//
// [Conn] 完整实现了 RFC 6455 的基础帧处理机制：
//
//   - 消息读写：[Conn.ReadMessage] 与 [Conn.WriteText] 支持文本与二进制消息收发；
//   - 分片处理：透明组装连续的 Continuation 分片帧，直到 FIN 帧到达；
//   - 掩码要求：严格遵守规范，客户端发送必须带随机掩码（Masking-Key），服务端接收必须按掩码还原；
//   - 保活控制：在后台自动应答 Ping 帧并回复 Pong 帧，收到 Close 帧时优雅关闭连接。
//
// # 并发与资源边界
//
// [Conn.WriteText] 内部使用互斥锁保护，支持多个 goroutine 并发安全写入。
// 读取操作 [Conn.ReadMessage] 必须由单 goroutine 串行执行。
// 连接由调用方显式调用 [Conn.Close] 释放底层网络套接字。
package ws

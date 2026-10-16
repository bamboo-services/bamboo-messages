// Package bamboo 实现面向 Bamboo 原生端点（/v1/bamboo）的 Provider 适配器。
//
// 本包使用独立定义的镜像 wire DTO 直接与部署了 Bamboo Messages 服务端协议的端点通信，
// 避免引入上层 facade 包产生的循环导入，是 Bamboo 服务集群间内部级联调用的官方适配器。
//
// # 构造入口
//
//   - [NewProvider]：最简构造函数；
//   - [NewProviderWithOptions]：完整构造函数，支持配置 BaseURL、请求头与拦截器。
package bamboo

# 函数与方法

按需说明输入边界、返回值、错误条件、并发安全和调用时机。

检查参数与接收器的零值、nil 行为，Context 的取消和超时，以及资源的创建、归属与释放。重复或并发调用的语义以实现和测试为依据。

```go
// NewHandler 创建输出单行 JSON 的 slog.Handler。
//
// Writer 必须显式提供，ServiceName 必须非空；校验失败时返回错误。
// 每条记录的根节点携带权威 service.name，合法的关联回调结果提供成对的
// trace_id 与 span_id。返回的 Handler 可被多 goroutine 并发使用。
func NewHandler(options Options) (slog.Handler, error)
```

错误值的定义见 [错误注释](./errors.md)，调用与处理示例见 [调用示例](./examples.md)。

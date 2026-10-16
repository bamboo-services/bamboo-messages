# 函数类型与回调

说明回调的输入、结果与执行时机。涉及生命周期的函数类型，额外说明取消、超时、幂等性和并发调用语义：

```go
// ReaderFunc 是使普通函数实现 [Reader] 的适配器。
// [ReaderFunc.Lookup] 调用底层函数；nil 适配器返回空值与 false。
type ReaderFunc func(key EnvKey) (value string, found bool)
```

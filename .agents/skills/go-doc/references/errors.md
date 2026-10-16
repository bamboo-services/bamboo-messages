# 错误注释

说明导出错误的触发条件与调用方的处理方式。错误分类、重试策略与 `errors.Is` 的使用依据实际实现。

错误文本属于运行时文本，范围与例外以根目录 [AGENTS.md](../../../../AGENTS.md) 为准：

- 本包自行产生的诊断错误使用英文，以包名加冒号开头；按 Go Code Review Comments 的 Error Strings 约定，开头不大写（专有名词与缩写除外），末尾不带标点。
- 调用方传入的消息与错误码、下游错误（如 `context.Cause`）、用户输入与标识符原样保留，不翻译、不改大小写；表示调用方领域错误的错误值按其类型文档写明的格式输出，不强加包名前缀。
- 仓库维护脚本的提示不在此列；测试中对错误文本的期望值与被测文本一致。

示例：

```go
// ErrNotEnabled 表示配置已关闭 Traces 信号。
// 调用方可通过 errors.Is 识别该结果，并继续装配其余已启用能力。
var ErrNotEnabled = errors.New("tracing: traces signal is disabled by configuration")
```

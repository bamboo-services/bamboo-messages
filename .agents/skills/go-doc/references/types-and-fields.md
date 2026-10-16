# 类型、结构体字段与接口

## 类型与字段

类型注释说明职责与使用边界。每个导出字段的含义由类型注释或字段注释说明，不要求逐个字段单独注释。字段有来源、必填性、默认值或资源归属等约束时，在字段上方单独说明；一组字段共用一段说明时，注释写在这组字段之前：

```go
// Options 是构造日志 Handler 与 Logger 的配置选项。
type Options struct {
	// Writer 是日志输出目标，必须显式提供，由调用方管理其生命周期。
	Writer io.Writer

	// ServiceName 是每条记录根节点的权威服务名，非空必填。
	ServiceName string

	// SpanContext 是可选的关联函数；为 nil 时日志仅输出基础字段。
	SpanContext SpanContextFunc
}
```

## 接口与方法

接口方法的契约是调用方与实现方共同遵守的约定，写在方法注释或接口的类型注释中。标准库 `io.Reader`、`http.Handler` 把方法契约写在类型注释里；方法较多或契约各不相同时，逐个方法注释更清楚：

```go
// Keyer 抽象能够转换为基础通用 [Key] 的对象。
type Keyer interface {
	// Key 返回对象对应的通用键。
	Key() Key
}
```

方法的输入、返回值和调用约束按 [函数与方法](./functions-and-methods.md) 检查；函数签名类型按 [函数类型与回调](./function-types.md) 检查。

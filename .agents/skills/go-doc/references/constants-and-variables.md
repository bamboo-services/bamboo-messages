# 常量与变量

## 单个声明

未分组的常量与变量写完整的声明前文档注释，首句以标识符开头：

```go
// MaxKeyBytes 是单个键的字节上限。
const MaxKeyBytes = 128
```

## 成组声明

成组的 `const (...)` 与 `var (...)` 可以由块前的一段注释说明共同职责与约束，条目按需要说明的长度选择写法：

- 简短说明用行尾 `//` 注释，列对齐交由 `gofmt` 处理；
- 需要较长说明的条目，在条目上方写多行注释，条目之间用空行分隔；
- 组注释或类型注释已说明各项的含义、取值关系与必要约束，且名称能与之清楚对应时，可以省略逐项的重复说明；名称本身不能替代文档中尚未写明的契约；
- 导出的错误变量逐项说明各自的触发条件，行尾短注释或条目上方注释均可，内容按 [错误注释](./errors.md) 编写。

同一组内的写法保持一致。简短说明：

```go
// HTTP 与 gRPC 的监听配置。
const (
	HTTPHost       EnvKey = "HTTP_HOST"       // HTTP 监听地址。
	HTTPPort       EnvKey = "HTTP_PORT"       // HTTP 监听端口。
	GRPCPort       EnvKey = "GRPC_PORT"       // gRPC 监听端口。
	GRPCReflection EnvKey = "GRPC_REFLECTION" // 是否开启 gRPC 反射。
)
```

条目需要说明触发场景或状态迁移时，写在条目上方。标准库 `net/http` 的 `ConnState` 常量组与 `ErrBodyNotAllowed`、`ErrHijacked` 所在的错误变量组都采用这种写法：

```go
// 分页参数校验返回的错误，调用方可通过 errors.Is 识别。
var (
	// ErrInvalidPageNumber 表示页码小于 1。
	ErrInvalidPageNumber = errors.New("pagination: page number must be at least 1")

	// ErrPageSizeTooLarge 表示每页条数超过 MaxPageSize。
	// 调用方可以把每页条数降到上限后重试。
	ErrPageSizeTooLarge = errors.New("pagination: page size exceeds the maximum")
)
```

组注释承载各项含义的例子是标准库 `text/scanner` 的 token 常量组：组注释说明各项都是 `Scan` 的结果，条目不逐项注释。带类型的常量在 `go doc` 中显示在类型旁，取值关系可以写在类型注释中，例如 `time.Month` 的类型注释写明「January = 1」，各月份常量不逐项注释。

可变的导出变量额外说明修改时机与并发访问约束。

## 展示验证

通过 [验证与交付](./verification.md) 中的格式检查与 `go doc` 命令核验注释，确认单个声明和成组声明中的说明均可读。

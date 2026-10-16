# 调用示例

## GoDoc 中的代码块

代码块使用缩进，推荐在 `//` 后使用一个 Tab。引用已有函数体时，明确展示必要的错误处理：

```go
// 典型用法：
//
//	app, err := application.New()
//	if err != nil {
//		return err
//	}
//	if err := app.Run(ctx); err != nil {
//		return err
//	}
```

此片段放在具有 `ctx` 的调用上下文中，标识符仅示意写法，实际签名以目标包为准。需要独立运行的用法提供完整 package、import 和函数；较长的可执行示例使用 `example_test.go` 验证。

## 可执行示例测试

以下示例展示完整示例测试的形式（`greeting` 为示意包）：

```go
package greeting_test

import (
	"fmt"

	"example.com/sdk/greeting"
)

func ExampleGreet() {
	fmt.Println(greeting.Greet("konata"))
	// Output: hello, konata
}
```

## 示例约束

- 示例采用实际模块路径与当前 API 签名。
- 示例涉及资源创建时，展示资源归属、失败处理与释放方式；退出 Context 和超时按目标 API 的实际契约选取。
- 可执行示例使用确定性输入；带 `Output:` 的示例同时验证输出。`Output:` 保留 Go 示例测试要求的格式。

示例的编译、测试和临时文件清理流程见 [验证与交付](./verification.md)。

package provider

// ============================================
// Provider 公共 Options（拦截器等扩展字段）
// ============================================

// Options Provider 公共运行时选项。
//
// 各 Provider 实现的私有 config 结构体可匿名嵌入本结构体，
// 即可获得 Interceptors 字段而无需在 4 个 Provider 中各自重复定义。
//
// 该设计目标：
//   - 字段统一定义、统一管理
//   - 各 Provider 的 WithInterceptor wrapper 只需转发到 ApplyOptions
//   - 业务代码与测试都可直接访问 Interceptors 字段
//
// 字段首字母大写以保证嵌入子包后仍可访问；Provider 本身不会被 JSON marshal，
// 故无需担心序列化曝光。
type Options struct {
	// Interceptors 注册的请求拦截器列表。
	//
	// 按 WithInterceptor 调用顺序追加，ApplyInterceptors 也按此顺序执行。
	// nil 或空切片表示无拦截器，ApplyInterceptors 会原样返回 body（零开销）。
	Interceptors []RequestInterceptor
}

// Option 配置 Options 的函数选项（公共版本）。
//
// 各 Provider 包可定义自己的 Option 类型与 WithXxx 函数，
// 但涉及拦截器的注册应统一通过 WithInterceptor 转发到本包的 ApplyOptions。
type Option func(*Options)

// WithInterceptor 注册一个请求拦截器。
//
// 多次调用按调用顺序追加，ApplyInterceptors 会按相同顺序执行。
// 显式传入 nil 拦截器会被静默拒绝（防御性），避免后续 ApplyInterceptors 踩 nil 槽位。
//
// 使用示例：
//
//	p := anthropic.NewProviderWithOptions(
//	    anthropic.WithAPIKey("sk-xxx"),
//	    anthropic.WithInterceptor(myInterceptor), // 各 Provider 包提供同名 wrapper
//	)
func WithInterceptor(fn RequestInterceptor) Option {
	return func(o *Options) {
		if fn == nil {
			return // 防御：拒绝 nil
		}
		o.Interceptors = append(o.Interceptors, fn)
	}
}

// ApplyOptions 将公共 Option 列表应用到默认 Options。
//
// 创建空 Options 实例，遍历所有 Option 并应用。
// 无参数时返回零值 Options（Interceptors 为 nil 切片），
// 满足「未注入拦截器时零行为变化」的硬契约。
func ApplyOptions(opts ...Option) *Options {
	o := &Options{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(o)
	}
	return o
}

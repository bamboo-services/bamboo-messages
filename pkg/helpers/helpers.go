// Package helpers 提供通用的工具函数。
//
// 该包包含以下工具函数：
//   - 指针辅助函数: PtrFloat64, PtrBool, PtrInt64
//   - ProviderExtra 安全取值函数: GetExtraFloat64, GetExtraInt64, GetExtraString, GetExtraBool, GetExtraAny
//
// 这些函数可以被所有适配器和上层业务使用，减少重复代码。
package helpers

// PtrFloat64 返回 float64 指针。
//
// 用于设置 RequestConfig 的可选字段。
// 用法: config.Temperature = PtrFloat64(0.7)
func PtrFloat64(v float64) *float64 {
	return &v
}

// PtrBool 返回 bool 指针。
//
// 用于设置 RequestConfig 的可选字段。
// 用法: config.ThinkingConfig.Enabled = PtrBool(true)
func PtrBool(v bool) *bool {
	return &v
}

// PtrInt64 返回 int64 指针。
//
// 用于设置 RequestConfig 的可选字段。
// 用法: config.ThinkingConfig.BudgetTokens = PtrInt64(10000)
func PtrInt64(v int64) *int64 {
	return &v
}

// PtrString 返回 string 指针。
//
// 用于设置 RequestConfig 的可选字段。
// 用法: config.Model = PtrString("claude-sonnet-4-20250514")
func PtrString(v string) *string {
	return &v
}

// GetExtraFloat64 从 ProviderExtra 中安全获取 float64 值。
//
// 参数:
//   - extra - ProviderExtra 映射表，为 nil 时返回 (0, false)
//   - key - 要查找的键名
//
// 返回值中的 bool 表示是否成功找到并完成类型断言。
func GetExtraFloat64(extra map[string]any, key string) (float64, bool) {
	if extra == nil {
		return 0, false
	}
	v, ok := extra[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// GetExtraInt64 从 ProviderExtra 中安全获取 int64 值。
//
// 参数:
//   - extra - ProviderExtra 映射表，为 nil 时返回 (0, false)
//   - key - 要查找的键名
//
// 返回值中的 bool 表示是否成功找到并完成类型断言。
func GetExtraInt64(extra map[string]any, key string) (int64, bool) {
	if extra == nil {
		return 0, false
	}
	v, ok := extra[key]
	if !ok {
		return 0, false
	}
	i, ok := v.(int64)
	return i, ok
}

// GetExtraString 从 ProviderExtra 中安全获取 string 值。
//
// 参数:
//   - extra - ProviderExtra 映射表，为 nil 时返回 ("", false)
//   - key - 要查找的键名
//
// 返回值中的 bool 表示是否成功找到并完成类型断言。
func GetExtraString(extra map[string]any, key string) (string, bool) {
	if extra == nil {
		return "", false
	}
	v, ok := extra[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetExtraBool 从 ProviderExtra 中安全获取 bool 值。
//
// 参数:
//   - extra - ProviderExtra 映射表，为 nil 时返回 (false, false)
//   - key - 要查找的键名
//
// 返回值中的 bool 表示是否成功找到并完成类型断言。
func GetExtraBool(extra map[string]any, key string) (bool, bool) {
	if extra == nil {
		return false, false
	}
	v, ok := extra[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// GetExtraAny 从 ProviderExtra 中获取任意类型值。
//
// 参数:
//   - extra - ProviderExtra 映射表，为 nil 时返回 (nil, false)
//   - key - 要查找的键名
//
// 返回值中的 bool 表示是否成功找到。
func GetExtraAny(extra map[string]any, key string) (any, bool) {
	if extra == nil {
		return nil, false
	}
	v, ok := extra[key]
	return v, ok
}

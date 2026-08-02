package bamboo

import pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"

// BambooError 是 pkgErrors.BambooError 的类型别名。
//
// 通过别名复用 pkg/errors 包中的统一错误类型，避免在 bamboo 层重复定义
// 错误结构，同时保持上层 API 的稳定性。
type BambooError = pkgErrors.BambooError

// NewBambooError 是 pkgErrors.NewBambooError 的别名。
//
// 参数:
//   - category - 错误分类
//   - message - 错误描述信息
//   - statusCode - HTTP 状态码（可选，无特定状态码可传 0）
var NewBambooError = pkgErrors.NewBambooError

// NewBambooErrorWithCause 是 pkgErrors.NewBambooErrorWithCause 的别名。
//
// 参数:
//   - category - 错误分类
//   - message - 错误描述信息
//   - statusCode - HTTP 状态码（可选，无特定状态码可传 0）
//   - cause - 底层错误（可为 nil）
var NewBambooErrorWithCause = pkgErrors.NewBambooErrorWithCause

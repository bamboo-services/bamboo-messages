package xerr

import (
	"context"
	"errors"
)

// Error 是 bamboo-messages 内部使用的最小错误类型，替代原 bamboo-base-go/common/error.Error。
//
// 设计原则：bamboo 转换链仅读取错误的消息文本（见 bamboo/convert.go 的 handleError），
// 从不访问 ErrorCode/Output/Data 等字段，因此这里只需保留 err + Message。
// 保留 *Error 指针语义，确保 convert.go 的 handleError(err *xError.Error) 与
// internal/provider/stream.go 的 StreamEvent.Err *xError.Error 字段零行为变化。
type Error struct {
	err     error
	Message string
}

// Error 实现 error 接口，返回底层 cause 的消息。
func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Unwrap 支持 errors.Is / errors.As 链式解包。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// NewError 兼容原 xError.NewError 签名，用于平滑替换。
//
// 参数对齐原签名 NewError(ctx, err *ErrorCode, msg ErrMessage, throw bool, getErr ...error)，
// 但 ctx / ErrorCode / throw 在 bamboo 的全部调用点都不被实际使用（throw 恒为 false），
// 故这里以 _ 占位，仅保留 msg 与 cause 的语义。
func NewError(_ context.Context, _ any, msg string, _ bool, cause ...error) *Error {
	e := errors.New(msg)
	if len(cause) > 0 && cause[0] != nil {
		e = cause[0]
	}
	return &Error{err: e, Message: msg}
}

// Package errors 测试。
package errors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestBambooError_Unwrap_NilCause 验证 NewBambooError（无 cause）的 Unwrap 返回 nil。
func TestBambooError_Unwrap_NilCause(t *testing.T) {
	err := NewBambooError("SDK", "some error", 0)
	if err.Unwrap() != nil {
		t.Errorf("NewBambooError 不应携带 cause，got=%v", err.Unwrap())
	}
}

// TestBambooError_Unwrap_WithCause 验证 WithCause 的 Unwrap 返回 cause，
// 且 errors.Is(err, context.Canceled) 可穿透错误链。
func TestBambooError_Unwrap_WithCause(t *testing.T) {
	cause := context.Canceled
	err := NewBambooErrorWithCause("SDK", "对话已取消: context canceled", 0, cause)
	if err.Unwrap() != cause {
		t.Errorf("Unwrap() 应返回 cause，got=%v", err.Unwrap())
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) 应为 true，got=%v", err)
	}
}

// TestBambooError_ErrorsIs_NoCause 验证无 cause 的错误不匹配 context.Canceled。
func TestBambooError_ErrorsIs_NoCause(t *testing.T) {
	err := NewBambooError("SDK", "some error", 0)
	if errors.Is(err, context.Canceled) {
		t.Errorf("无 cause 的 BambooError 不应匹配 context.Canceled")
	}
}

// TestBambooError_ErrorFormat_Unchanged 验证 Error() 格式不因 cause 改变。
func TestBambooError_ErrorFormat_Unchanged(t *testing.T) {
	err := NewBambooErrorWithCause("SDK", "对话已取消: context canceled", 0, context.Canceled)
	got := err.Error()
	if got != "Bamboo[SDK]错误: 对话已取消: context canceled" {
		t.Errorf("Error() = %q，格式不应变化", got)
	}
}

// TestBambooError_JSON_ExcludesCause 验证 JSON 序列化只含导出字段，不泄露 cause。
func TestBambooError_JSON_ExcludesCause(t *testing.T) {
	err := NewBambooErrorWithCause("SDK", "对话已取消: context canceled", 400, context.Canceled)
	data, mErr := json.Marshal(err)
	if mErr != nil {
		t.Fatalf("marshal 失败: %v", mErr)
	}
	s := string(data)
	for _, want := range []string{`"category":"SDK"`, `"message"`, `"status_code":400`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON 应包含导出字段 %s，got=%s", want, s)
		}
	}
	if strings.Contains(s, "cause") || strings.Contains(s, "Canceled") {
		t.Errorf("JSON 不应包含 cause 字段或内容，got=%s", s)
	}
}

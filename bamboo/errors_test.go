package bamboo

import (
	"strings"
	"testing"
)

// TestNewBambooError 验证 NewBambooError 创建正确的错误实例。
func TestNewBambooError(t *testing.T) {
	err := NewBambooError("invalid_request", "bad input", 400)
	if err == nil {
		t.Fatal("NewBambooError 返回 nil")
	}
	if err.Category != "invalid_request" {
		t.Errorf("Category = %q, 期望 %q", err.Category, "invalid_request")
	}
	if err.Message != "bad input" {
		t.Errorf("Message = %q, 期望 %q", err.Message, "bad input")
	}
	if err.StatusCode != 400 {
		t.Errorf("StatusCode = %d, 期望 400", err.StatusCode)
	}
}

// TestNewBambooError_ZeroStatusCode 验证无特定状态码时可传 0。
func TestNewBambooError_ZeroStatusCode(t *testing.T) {
	err := NewBambooError("SDK", "client error", 0)
	if err == nil {
		t.Fatal("NewBambooError 返回 nil")
	}
	if err.StatusCode != 0 {
		t.Errorf("StatusCode = %d, 期望 0", err.StatusCode)
	}
}

// TestBambooError_Error 验证 Error() 方法的字符串格式。
func TestBambooError_Error(t *testing.T) {
	t.Run("常规格式", func(t *testing.T) {
		err := NewBambooError("invalid_request", "bad input", 400)
		got := err.Error()
		if !strings.Contains(got, "bad input") {
			t.Errorf("Error() = %q, 应包含 %q", got, "bad input")
		}
		if !strings.Contains(got, "invalid_request") {
			t.Errorf("Error() = %q, 应包含 %q", got, "invalid_request")
		}
		if !strings.HasPrefix(got, "Bamboo[invalid_request]错误:") {
			t.Errorf("Error() = %q, 期望前缀 %q", got, "Bamboo[invalid_request]错误:")
		}
	})

	t.Run("无状态码", func(t *testing.T) {
		err := NewBambooError("SDK", "client error", 0)
		got := err.Error()
		if !strings.Contains(got, "client error") {
			t.Errorf("Error() = %q, 应包含 %q", got, "client error")
		}
		if !strings.Contains(got, "SDK") {
			t.Errorf("Error() = %q, 应包含 %q", got, "SDK")
		}
	})
}

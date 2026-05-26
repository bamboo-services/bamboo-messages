package bamboo

import (
	"strings"
	"testing"
)

// TestNewBambooError 验证 NewBambooError 创建正确的错误实例。
func TestNewBambooError(t *testing.T) {
	err := NewBambooError("invalid_request_error", "bad input")
	if err == nil {
		t.Fatal("NewBambooError 返回 nil")
	}
	if err.Type != "invalid_request_error" {
		t.Errorf("Type = %q, 期望 %q", err.Type, "invalid_request_error")
	}
	if err.Message != "bad input" {
		t.Errorf("Message = %q, 期望 %q", err.Message, "bad input")
	}
	if err.Code != "" {
		t.Errorf("Code = %q, 期望空字符串", err.Code)
	}
}

// TestNewBambooErrorWithCode 验证带错误代码的错误创建。
func TestNewBambooErrorWithCode(t *testing.T) {
	err := NewBambooErrorWithCode("api_error", "server error", "ERR_500")
	if err == nil {
		t.Fatal("NewBambooErrorWithCode 返回 nil")
	}
	if err.Type != "api_error" {
		t.Errorf("Type = %q, 期望 %q", err.Type, "api_error")
	}
	if err.Message != "server error" {
		t.Errorf("Message = %q, 期望 %q", err.Message, "server error")
	}
	if err.Code != "ERR_500" {
		t.Errorf("Code = %q, 期望 %q", err.Code, "ERR_500")
	}
}

// TestBambooError_Error 验证 Error() 方法的字符串格式。
func TestBambooError_Error(t *testing.T) {
	t.Run("无错误代码", func(t *testing.T) {
		err := NewBambooError("invalid_request_error", "bad input")
		got := err.Error()
		if !strings.Contains(got, "bad input") {
			t.Errorf("Error() = %q, 应包含 %q", got, "bad input")
		}
		if !strings.Contains(got, "invalid_request_error") {
			t.Errorf("Error() = %q, 应包含 %q", got, "invalid_request_error")
		}
		// 不应包含 code 部分
		if strings.Contains(got, "code:") {
			t.Errorf("Error() = %q, 不应包含 code 部分", got)
		}
	})

	t.Run("有错误代码", func(t *testing.T) {
		err := NewBambooErrorWithCode("api_error", "server error", "ERR_500")
		got := err.Error()
		if !strings.Contains(got, "server error") {
			t.Errorf("Error() = %q, 应包含 %q", got, "server error")
		}
		if !strings.Contains(got, "ERR_500") {
			t.Errorf("Error() = %q, 应包含 %q", got, "ERR_500")
		}
		if !strings.Contains(got, "code:") {
			t.Errorf("Error() = %q, 应包含 'code:' 部分", got)
		}
	})
}

// TestErrorTypeConstants 验证 5 个错误类型常量值。
func TestErrorTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"ErrorTypeInvalidRequest", ErrorTypeInvalidRequest, "invalid_request_error"},
		{"ErrorTypeAuthentication", ErrorTypeAuthentication, "authentication_error"},
		{"ErrorTypeRateLimit", ErrorTypeRateLimit, "rate_limit_error"},
		{"ErrorTypeAPI", ErrorTypeAPI, "api_error"},
		{"ErrorTypeProvider", ErrorTypeProvider, "provider_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, 期望 %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

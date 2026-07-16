package bamboo

import (
	"encoding/json"
	"errors"
	"testing"

	bmbamboo "github.com/bamboo-services/bamboo-messages/bamboo"
	pkgErrors "github.com/bamboo-services/bamboo-messages/pkg/errors"
)

func TestSerializeError_BambooErrorPassthrough(t *testing.T) {
	err := pkgErrors.NewBambooError("下游", "upstream timeout", 504)

	data := serializeError(err)

	var out bambooErrorResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if out.Type != "error" {
		t.Errorf("Type = %q, want %q", out.Type, "error")
	}
	if out.Error.Category != "下游" {
		t.Errorf("Category = %q, want %q", out.Error.Category, "下游")
	}
	if out.Error.Message != "upstream timeout" {
		t.Errorf("Message = %q", out.Error.Message)
	}
	if out.Error.StatusCode != 504 {
		t.Errorf("StatusCode = %d, want 504", out.Error.StatusCode)
	}
}

func TestSerializeError_BambooErrorViaAlias(t *testing.T) {
	// 通过 bamboo.BambooError 类型别名（= pkgErrors.BambooError）构造
	err := bmbamboo.NewBambooError("SDK", "invalid config", 400)

	data := serializeError(err)

	var out bambooErrorResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if out.Error.Category != "SDK" {
		t.Errorf("Category = %q, want %q", out.Error.Category, "SDK")
	}
	if out.Error.Message != "invalid config" {
		t.Errorf("Message = %q", out.Error.Message)
	}
	if out.Error.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", out.Error.StatusCode)
	}
}

func TestSerializeError_PlainErrorFallsBackToSDK(t *testing.T) {
	err := errors.New("some plain error")

	data := serializeError(err)

	var out bambooErrorResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if out.Error.Category != "SDK" {
		t.Errorf("Category = %q, want %q (fallback)", out.Error.Category, "SDK")
	}
	if out.Error.Message != "some plain error" {
		t.Errorf("Message = %q, want %q", out.Error.Message, "some plain error")
	}
	if out.Error.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0", out.Error.StatusCode)
	}
}

func TestSerializeError_WrappedBambooError(t *testing.T) {
	// errors.As 应能穿透 wrapping 提取内层 *BambooError
	inner := pkgErrors.NewBambooError("认证", "invalid api key", 401)
	wrapped := errors.Join(inner, errors.New("context"))

	data := serializeError(wrapped)

	var out bambooErrorResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if out.Error.Category != "认证" {
		t.Errorf("Category = %q, want %q", out.Error.Category, "认证")
	}
	if out.Error.Message != "invalid api key" {
		t.Errorf("Message = %q", out.Error.Message)
	}
	if out.Error.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", out.Error.StatusCode)
	}
}

func TestSerializeError_NilSafety(t *testing.T) {
	// nil error 不应 panic，返回默认降级错误
	data := serializeError(nil)

	var out bambooErrorResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if out.Type != "error" {
		t.Errorf("Type = %q, want %q", out.Type, "error")
	}
	if out.Error.Category != "SDK" {
		t.Errorf("Category = %q, want %q (fallback for nil)", out.Error.Category, "SDK")
	}
	if out.Error.Message != "internal error" {
		t.Errorf("Message = %q, want %q", out.Error.Message, "internal error")
	}
}

func TestSerializeError_ZeroStatusCodeOmitted(t *testing.T) {
	// StatusCode 为 0 时 omitempty 应省略该字段
	err := pkgErrors.NewBambooError("SDK", "no status", 0)

	data := serializeError(err)

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	errObj, ok := raw["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field not an object: %T", raw["error"])
	}
	if _, exists := errObj["status_code"]; exists {
		t.Errorf("status_code should be omitted when 0, got %v", errObj["status_code"])
	}
}

// TestCodec_SerializeError_Delegation 验证 Codec.SerializeError 委托到 serializeError。
func TestCodec_SerializeError_Delegation(t *testing.T) {
	err := pkgErrors.NewBambooError("下游", "test via codec", 500)

	data := Codec.SerializeError(err)
	if len(data) == 0 {
		t.Fatal("Codec.SerializeError() returned empty data")
	}

	var out bambooErrorResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if out.Error.Message != "test via codec" {
		t.Errorf("Message = %q", out.Error.Message)
	}
}

// TestCodec_Format_ReturnsBamboo 验证 Codec.Format() 返回 FormatBamboo。
func TestCodec_Format_ReturnsBamboo(t *testing.T) {
	got := Codec.Format()
	if got != formatBambooExpected {
		t.Errorf("Format() = %q, want %q", got, formatBambooExpected)
	}
}

const formatBambooExpected = "bamboo"

// TestCodec_NewSerializer_NonNil 验证 NewSerializer 返回非 nil 的 StreamSerializer
// （stream.go 已在 todo 3 中实现）。
func TestCodec_NewSerializer_NonNil(t *testing.T) {
	got := Codec.NewSerializer("test-model")
	if got == nil {
		t.Errorf("NewSerializer() = nil, want non-nil StreamSerializer")
	}
}

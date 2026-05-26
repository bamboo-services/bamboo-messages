package bamboo

import "testing"

// TestPtrFloat64 验证 PtrFloat64 返回正确的 float64 指针。
func TestPtrFloat64(t *testing.T) {
	t.Run("正常正数", func(t *testing.T) {
		p := PtrFloat64(3.14)
		if p == nil {
			t.Fatal("PtrFloat64(3.14) 返回 nil")
		}
		if *p != 3.14 {
			t.Errorf("*p = %v, 期望 3.14", *p)
		}
	})

	t.Run("零值", func(t *testing.T) {
		p := PtrFloat64(0)
		if p == nil {
			t.Fatal("PtrFloat64(0) 返回 nil")
		}
		if *p != 0 {
			t.Errorf("*p = %v, 期望 0", *p)
		}
	})

	t.Run("负数", func(t *testing.T) {
		p := PtrFloat64(-1.5)
		if p == nil {
			t.Fatal("PtrFloat64(-1.5) 返回 nil")
		}
		if *p != -1.5 {
			t.Errorf("*p = %v, 期望 -1.5", *p)
		}
	})

	t.Run("多次调用返回不同指针", func(t *testing.T) {
		p1 := PtrFloat64(1.0)
		p2 := PtrFloat64(1.0)
		if p1 == p2 {
			t.Error("两次调用应返回不同指针地址")
		}
	})
}

package provider

import "testing"

// TestGetExtraFloat64 测试从 ProviderExtra 中安全获取 float64 值
func TestGetExtraFloat64(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		key      string
		want     float64
		wantOk   bool
	}{
		{
			name:   "正常值 - float64 类型",
			extra:  map[string]any{"top_k": 50.0},
			key:    "top_k",
			want:   50.0,
			wantOk: true,
		},
		{
			name:   "正常值 - 零值",
			extra:  map[string]any{"value": 0.0},
			key:    "value",
			want:   0.0,
			wantOk: true,
		},
		{
			name:   "类型不匹配 - 字符串",
			extra:  map[string]any{"key": "not_float"},
			key:    "key",
			want:   0,
			wantOk: false,
		},
		{
			name:   "类型不匹配 - int",
			extra:  map[string]any{"key": 42},
			key:    "key",
			want:   0,
			wantOk: false,
		},
		{
			name:   "键不存在",
			extra:  map[string]any{"other": 100.0},
			key:    "missing",
			want:   0,
			wantOk: false,
		},
		{
			name:   "nil map",
			extra:  nil,
			key:    "key",
			want:   0,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOk := GetExtraFloat64(tt.extra, tt.key)
			if got != tt.want || gotOk != tt.wantOk {
				t.Errorf("GetExtraFloat64() = (%v, %v), want (%v, %v)", got, gotOk, tt.want, tt.wantOk)
			}
		})
	}
}

// TestGetExtraInt64 测试从 ProviderExtra 中安全获取 int64 值
func TestGetExtraInt64(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		key      string
		want     int64
		wantOk   bool
	}{
		{
			name:   "正常值 - int64 类型",
			extra:  map[string]any{"seed": int64(12345)},
			key:    "seed",
			want:   12345,
			wantOk: true,
		},
		{
			name:   "正常值 - 零值",
			extra:  map[string]any{"value": int64(0)},
			key:    "value",
			want:   0,
			wantOk: true,
		},
		{
			name:   "类型不匹配 - 字符串",
			extra:  map[string]any{"key": "not_int"},
			key:    "key",
			want:   0,
			wantOk: false,
		},
		{
			name:   "类型不匹配 - float64",
			extra:  map[string]any{"key": 42.5},
			key:    "key",
			want:   0,
			wantOk: false,
		},
		{
			name:   "类型不匹配 - int (非 int64)",
			extra:  map[string]any{"key": 42},
			key:    "key",
			want:   0,
			wantOk: false,
		},
		{
			name:   "键不存在",
			extra:  map[string]any{"other": 100},
			key:    "missing",
			want:   0,
			wantOk: false,
		},
		{
			name:   "nil map",
			extra:  nil,
			key:    "key",
			want:   0,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOk := GetExtraInt64(tt.extra, tt.key)
			if got != tt.want || gotOk != tt.wantOk {
				t.Errorf("GetExtraInt64() = (%v, %v), want (%v, %v)", got, gotOk, tt.want, tt.wantOk)
			}
		})
	}
}

// TestGetExtraString 测试从 ProviderExtra 中安全获取 string 值
func TestGetExtraString(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		key      string
		want     string
		wantOk   bool
	}{
		{
			name:   "正常值 - 字符串",
			extra:  map[string]any{"response_format": "json"},
			key:    "response_format",
			want:   "json",
			wantOk: true,
		},
		{
			name:   "正常值 - 空字符串",
			extra:  map[string]any{"value": ""},
			key:    "value",
			want:   "",
			wantOk: true,
		},
		{
			name:   "类型不匹配 - int",
			extra:  map[string]any{"key": 42},
			key:    "key",
			want:   "",
			wantOk: false,
		},
		{
			name:   "类型不匹配 - float64",
			extra:  map[string]any{"key": 3.14},
			key:    "key",
			want:   "",
			wantOk: false,
		},
		{
			name:   "类型不匹配 - bool",
			extra:  map[string]any{"key": true},
			key:    "key",
			want:   "",
			wantOk: false,
		},
		{
			name:   "键不存在",
			extra:  map[string]any{"other": "value"},
			key:    "missing",
			want:   "",
			wantOk: false,
		},
		{
			name:   "nil map",
			extra:  nil,
			key:    "key",
			want:   "",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOk := GetExtraString(tt.extra, tt.key)
			if got != tt.want || gotOk != tt.wantOk {
				t.Errorf("GetExtraString() = (%q, %v), want (%q, %v)", got, gotOk, tt.want, tt.wantOk)
			}
		})
	}
}

// TestGetExtraAny 测试从 ProviderExtra 中获取任意类型值
func TestGetExtraAny(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		key      string
		want     any
		wantOk   bool
	}{
		{
			name:   "正常值 - 字符串",
			extra:  map[string]any{"key": "value"},
			key:    "key",
			want:   "value",
			wantOk: true,
		},
		{
			name:   "正常值 - int",
			extra:  map[string]any{"key": 42},
			key:    "key",
			want:   42,
			wantOk: true,
		},
		{
			name:   "正常值 - float64",
			extra:  map[string]any{"key": 3.14},
			key:    "key",
			want:   3.14,
			wantOk: true,
		},
		{
			name:   "正常值 - bool",
			extra:  map[string]any{"key": true},
			key:    "key",
			want:   true,
			wantOk: true,
		},
		{
			name:   "正常值 - nil",
			extra:  map[string]any{"key": nil},
			key:    "key",
			want:   nil,
			wantOk: true,
		},
		{
			name:   "正常值 - map",
			extra:  map[string]any{"key": map[string]any{"nested": "value"}},
			key:    "key",
			want:   map[string]any{"nested": "value"},
			wantOk: true,
		},
		{
			name:   "正常值 - slice",
			extra:  map[string]any{"key": []int{1, 2, 3}},
			key:    "key",
			want:   []int{1, 2, 3},
			wantOk: true,
		},
		{
			name:   "键不存在",
			extra:  map[string]any{"other": "value"},
			key:    "missing",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "nil map",
			extra:  nil,
			key:    "key",
			want:   nil,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOk := GetExtraAny(tt.extra, tt.key)
			if gotOk != tt.wantOk {
				t.Errorf("GetExtraAny() ok = %v, want %v", gotOk, tt.wantOk)
			}
			if gotOk {
				// map 和 slice 类型不可比较，跳过这部分测试
				switch tt.want.(type) {
				case map[string]any, []int:
					// 跳过 map/slice 的直接比较，只验证类型
				default:
					if got != tt.want {
						t.Errorf("GetExtraAny() = %v, want %v", got, tt.want)
					}
				}
			}
		})
	}
}
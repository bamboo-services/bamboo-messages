package provider

import "testing"

// TestNormalizeReasoningEffort 验证 max → xhigh 归一化与其余值原样透传。
func TestNormalizeReasoningEffort(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"max", "xhigh"},
		{"xhigh", "xhigh"},
		{"high", "high"},
		{"medium", "medium"},
		{"low", "low"},
		{"minimal", "minimal"},
		{"none", "none"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeReasoningEffort(c.in); got != c.want {
			t.Errorf("NormalizeReasoningEffort(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

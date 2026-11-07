//go:build !english

package chinese

import "testing"

func TestSimplified(t *testing.T) {
	for in, want := range map[string]string{"該國": "该国", "經": "经", "桌": "桌", "hello": "hello"} {
		if got := Simplified(in); got != want {
			t.Errorf("Simplified(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTraditional(t *testing.T) {
	for in, want := range map[string]string{"国家": "國家", "学习": "學習", "hello": "hello"} {
		if got := Traditional(in); got != want {
			t.Errorf("Traditional(%q) = %q, want %q", in, got, want)
		}
	}
}

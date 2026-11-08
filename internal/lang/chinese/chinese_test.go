package chinese

import (
	"slices"
	"testing"
)

func TestFormsTrimTrailingChars(t *testing.T) {
	got := forms("学生们的")
	for _, want := range []string{"学生们", "学生"} {
		if !slices.Contains(got, want) {
			t.Errorf("forms missing %q: %q", want, got)
		}
	}
	if got := forms("好"); len(got) != 0 {
		t.Errorf("forms(single char) = %q", got)
	}
}

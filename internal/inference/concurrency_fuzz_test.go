package inference

import "testing"

func FuzzParseCPUList(f *testing.F) {
	for _, s := range []string{"0-3,6,8-9", "", "5", "a-b", "3-1", "0-"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 64 {
			t.Skip()
		}
		_ = parseCPUList(s)
	})
}

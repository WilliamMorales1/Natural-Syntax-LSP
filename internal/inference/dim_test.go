package inference

import (
	"os"
	"testing"
)

func TestOutputLastDim(t *testing.T) {
	p := os.Getenv("NSLS_EMBED_ONNX")
	if p == "" {
		t.Skip("NSLS_EMBED_ONNX unset")
	}
	if lib := os.Getenv("NSLS_ORT_LIB"); lib != "" {
		SetORTLibPath(lib)
	}
	d, err := outputLastDim(p, "last_hidden_state")
	if err != nil || d <= 0 {
		t.Fatalf("dim %d err %v", d, err)
	}
	t.Log("dim", d)
}

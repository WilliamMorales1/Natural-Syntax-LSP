package inference

import (
	"slices"
	"sync"
	"testing"
	"time"

	"natural-syntax-ls/internal/tokenizer"
)

// TestIdleUnloadReloads frees every model's session after the idle delay and requires identical output once it reloads.
func TestIdleUnloadReloads(t *testing.T) {
	SetIdleUnload(200 * time.Millisecond)
	t.Cleanup(func() { SetIdleUnload(DefaultIdleUnload) })
	words := tokenizer.BasicTokenize("The cat sat on the mat.")
	forEachModel(t, func(t *testing.T, m Predictor) {
		var s *idleSession
		switch m := m.(type) {
		case *POSModel:
			s = m.session
		case *EmbeddingModel:
			s = m.session
		case *DependencyModel:
			s = m.session
		default:
			t.Skipf("no session on %T", m)
		}
		before, err := m.PredictChunk(words)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for s.loaded() && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if s.loaded() {
			t.Fatal("session still loaded after idle delay")
		}
		after, err := m.PredictChunk(words)
		if err != nil {
			t.Fatal(err)
		}
		if !s.loaded() || !slices.Equal(before, after) {
			t.Fatalf("reload changed output or did not load: %v vs %v", before, after)
		}
	})
}

// TestIdleUnloadConcurrent races predictions against a session that unloads every few milliseconds.
func TestIdleUnloadConcurrent(t *testing.T) {
	SetIdleUnload(3 * time.Millisecond)
	t.Cleanup(func() { SetIdleUnload(DefaultIdleUnload) })
	words := tokenizer.BasicTokenize("Dogs bark at night.")
	forEachModel(t, func(t *testing.T, m Predictor) {
		want, err := m.PredictChunk(words)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for g := range 4 {
			wg.Go(func() {
				for i := range 6 {
					time.Sleep(time.Duration((g+i)%3) * 4 * time.Millisecond)
					got, err := m.PredictChunk(words)
					if err != nil {
						t.Error(err)
						return
					}
					if !slices.Equal(got, want) {
						t.Errorf("output changed across reloads")
						return
					}
				}
			})
		}
		wg.Wait()
	})
}

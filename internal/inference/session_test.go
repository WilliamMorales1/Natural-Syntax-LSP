package inference

import (
	"slices"
	"sync"
	"testing"
	"time"

	"natural-syntax-ls/internal/tokenizer"
)

// TestUnloadReloads frees every model's weights on request, then after the idle delay, and requires identical output after each reload.
func TestUnloadReloads(t *testing.T) {
	SetIdleUnload(200 * time.Millisecond)
	t.Cleanup(func() { SetIdleUnload(DefaultIdleUnload) })
	words := tokenizer.BasicTokenize("The cat sat on the mat.")
	forEachModel(t, func(t *testing.T, m Predictor) {
		u, ok := m.(interface {
			Unload()
			loadedSession() bool
		})
		if !ok {
			t.Skipf("no session on %T", m)
		}
		before, err := m.PredictChunk(words)
		if err != nil {
			t.Fatal(err)
		}
		u.Unload()
		if u.loadedSession() {
			t.Fatal("session loaded after Unload")
		}
		// This reload arms the idle timer with the short delay; the shared model's first load used the default.
		after, err := m.PredictChunk(words)
		if err != nil {
			t.Fatal(err)
		}
		if !u.loadedSession() || !slices.Equal(before, after) {
			t.Fatalf("reload changed output or did not load: %v vs %v", before, after)
		}
		deadline := time.Now().Add(5 * time.Second)
		for u.loadedSession() && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if u.loadedSession() {
			t.Fatal("session still loaded after idle delay")
		}
		if after, err = m.PredictChunk(words); err != nil {
			t.Fatal(err)
		}
		if !u.loadedSession() || !slices.Equal(before, after) {
			t.Fatalf("idle reload changed output or did not load: %v vs %v", before, after)
		}
	})
}

// TestIdleUnloadConcurrent races predictions against a session that unloads every few milliseconds; the locking is the same for every model, so the smallest one is enough.
func TestIdleUnloadConcurrent(t *testing.T) {
	dir := testDataDir(t)
	i := slices.IndexFunc(testModels, func(tm testModel) bool { return tm.name == "minilm" })
	m, err := sharedModel(dir, testModels[i].name, testModels[i].load)
	if err != nil {
		t.Skip(err)
	}
	SetIdleUnload(3 * time.Millisecond)
	t.Cleanup(func() { SetIdleUnload(DefaultIdleUnload) })
	m.(*EmbeddingModel).Unload() // rearm the timer with the short delay
	words := tokenizer.BasicTokenize("Dogs bark at night.")
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
}

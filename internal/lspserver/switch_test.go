package lspserver

import (
	"slices"
	"sync"
	"testing"
)

func TestSwitcherPrefetchStartsOnlyOtherLanguages(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	done := make(chan struct{}, 4)
	sw := newSwitcher(Config{Switch: func(code string) (string, string) {
		mu.Lock()
		asked = append(asked, code)
		mu.Unlock()
		return "", ""
	}})
	sw.onReady = func() { done <- struct{}{} }
	sw.prefetch("The cat sleeps on the warm windowsill.")
	sw.prefetch("ok")
	sw.prefetch("El gato duerme junto a la ventana.")
	<-done
	sw.prefetch("El perro duerme junto a la puerta.") // es already failed: no second load
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(asked, []string{"es"}) {
		t.Fatalf("loads asked for %v, want [es]", asked)
	}
	if !sw.skip["es"] || len(sw.pending) != 0 {
		t.Errorf("skip %v pending %v", sw.skip, sw.pending)
	}
}

package docview

import (
	"fmt"
	"testing"
)

func TestMemoryKeepsPlacesPerDocument(t *testing.T) {
	m := NewMemory()
	if got := m.Place("a"); got != (Place{}) {
		t.Fatalf("empty: %+v", got)
	}
	lines := &LineRange{2, 3}
	m.SetPlace("a", Place{Mode: Preview, Scroll: 40, Lines: lines})
	m.SetPlace("b", Place{Scroll: 7})
	if got := m.Place("a"); got.Mode != Preview || got.Scroll != 40 || got.Lines != lines {
		t.Fatalf("a: %+v", got)
	}
	m.Forget("a")
	if got := m.Place("a"); got != (Place{}) {
		t.Fatalf("forgotten: %+v", got)
	}
	if m.Place("b").Scroll != 7 {
		t.Fatal("b stays")
	}
}

func TestMemoryIsBounded(t *testing.T) {
	m := NewMemory()
	for i := range memoryLimit + 1 {
		m.SetPlace(fmt.Sprint(i), Place{Scroll: 1})
	}
	if m.Place("0").Scroll != 0 || m.Place("1").Scroll != 1 {
		t.Fatal("oldest leaves first")
	}
}

func TestHistory(t *testing.T) {
	var h History
	if h.CanGoBack() || h.CanGoForward() {
		t.Fatal("empty history")
	}
	h.Push(Location{Path: "a"})
	h.Push(Location{Path: "b"})
	h.Push(Location{Path: "c"})
	if got, ok := h.Back(); !ok || got.Path != "b" {
		t.Fatalf("back: %+v", got)
	}
	if got, ok := h.Back(); !ok || got.Path != "a" {
		t.Fatalf("back: %+v", got)
	}
	if _, ok := h.Back(); ok {
		t.Fatal("start")
	}
	if got, ok := h.Forward(); !ok || got.Path != "b" {
		t.Fatalf("forward: %+v", got)
	}
	// A new document drops what was forward of it.
	h.Push(Location{Path: "d"})
	if h.CanGoForward() {
		t.Fatal("forward dropped")
	}
	if got, _ := h.Back(); got.Path != "b" {
		t.Fatalf("back: %+v", got)
	}
	// Selecting lines replaces the entry; the same document too.
	lines := &LineRange{1, 2}
	h.Replace(Location{Path: "b", Lines: lines})
	h.Forward()
	if got, _ := h.Back(); got.Lines != lines {
		t.Fatalf("replaced: %+v", got)
	}
	h.Push(Location{Path: "b"})
	if got, _ := h.Back(); got.Path != "a" {
		t.Fatalf("same document keeps one entry: %+v", got)
	}
	h.Reset()
	if h.CanGoBack() || h.CanGoForward() {
		t.Fatal("reset")
	}
}

func TestHistoryIsBounded(t *testing.T) {
	var h History
	for i := range historyLimit + 5 {
		h.Push(Location{Path: fmt.Sprint(i)})
	}
	steps := 0
	for h.CanGoBack() {
		h.Back()
		steps++
	}
	if steps != historyLimit-1 {
		t.Fatalf("steps %d", steps)
	}
}

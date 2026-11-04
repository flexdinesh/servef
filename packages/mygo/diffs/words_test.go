package diffs

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// The expected runs come from jsdiff 9 and Pierre 1.6.1's span joining
// (computeLineDiffDecorations) run in Node on the same pairs.
func TestEmphasisMatchesPierre(t *testing.T) {
	for _, tc := range []struct {
		old, new string
		mode     Inline
		wantOld  []Range
		wantNew  []Range
	}{
		{"\tfmt.Println(\"hello\") // go-search-token", "\tfmt.Println(\"hello from servef\") // go-search-token", WordAlt, nil, []Range{{19, 31}}},
		{"foo bar baz", "foo qux baz", WordAlt, []Range{{4, 7}}, []Range{{4, 7}}},
		{"a b c d", "a x y z d", WordAlt, []Range{{2, 6}}, []Range{{2, 8}}},
		{"a b c d", "a x y z d", Word, []Range{{2, 3}, {4, 5}}, []Range{{2, 3}, {4, 5}, {6, 8}}},
		{"line 30", "line thirty", WordAlt, []Range{{5, 7}}, []Range{{5, 11}}},
		{"const x = 1;", "let x = 2;", WordAlt, []Range{{0, 5}, {10, 11}}, []Range{{0, 3}, {8, 9}}},
		{"abcdef", "abXdeY", Char, []Range{{2, 3}, {5, 6}}, []Range{{2, 3}, {5, 6}}},
		{"return a+b", "return a - b", WordAlt, []Range{{8, 9}}, []Range{{8, 11}}},
		{"", "new text", WordAlt, nil, []Range{{0, 8}}},
	} {
		old, new := Emphasis(tc.old, tc.new, tc.mode)
		if !reflect.DeepEqual(old, tc.wantOld) || !reflect.DeepEqual(new, tc.wantNew) {
			t.Errorf("Emphasis(%q, %q, %d) = %v, %v; want %v, %v", tc.old, tc.new, tc.mode, old, new, tc.wantOld, tc.wantNew)
		}
	}
}

func TestEmphasisOffAndLongLines(t *testing.T) {
	if old, new := Emphasis("a", "b", NoInline); old != nil || new != nil {
		t.Errorf("NoInline emphasized %v, %v", old, new)
	}
	long := string(make([]byte, maxEmphasisLength+1))
	if old, new := Emphasis(long, "b", WordAlt); old != nil || new != nil {
		t.Errorf("a line over %d emphasized %v, %v", maxEmphasisLength, old, new)
	}
}

// pathological is two 999-character lines that share no word: the
// review's worst case for one pair.
func pathological() (old, new string) {
	var a, b []byte
	for i := 0; len(a) < 999; i++ {
		a = append(a, byte('a'+i%26), ' ')
		b = append(b, byte('A'+i%26), byte('0'+i%10), ' ')
	}
	return string(a[:999]), string(b[:999])
}

func BenchmarkEmphasisPathological(b *testing.B) {
	old, new := pathological()
	for _, mode := range []Inline{WordAlt, Char} {
		b.Run(map[Inline]string{WordAlt: "word-alt", Char: "char"}[mode], func(b *testing.B) {
			for b.Loop() {
				Emphasis(old, new, mode)
			}
		})
	}
}

// A pair past the search's bounds gets no emphasis; a long pair with a
// small change keeps it.
func TestEmphasisBoundsTheSearch(t *testing.T) {
	old, new := pathological()
	for _, mode := range []Inline{WordAlt, Word, Char} {
		if o, n := Emphasis(old, new, mode); o != nil || n != nil {
			t.Errorf("mode %d: a pair past %d edits emphasized %d and %d runs", mode, maxEmphasisEdits, len(o), len(n))
		}
	}
	long := strings.Repeat("word ", 190)
	o, n := Emphasis(long+"old", long+"new", WordAlt)
	if want := []Range{{950, 953}}; !reflect.DeepEqual(o, want) || !reflect.DeepEqual(n, want) {
		t.Errorf("a small change in a long pair: %v, %v; want %v", o, n, want)
	}
	a, b := strings.Repeat("x", 600), strings.Repeat("y", 600)
	if _, ok := diffTokens(a, b, charTokens(a), charTokens(b), 1<<20, time.Now().Add(-time.Second)); ok {
		t.Error("a search past its deadline finished")
	}
}

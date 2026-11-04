package docview

import (
	"fmt"
	"strings"
	"testing"
)

// Format failures read as one sentence.
func TestFormatFailureMessage(t *testing.T) {
	if got := FormatFailureMessage("invalid character '}' at line 3."); got != "Couldn't format: invalid character '}' at line 3." {
		t.Errorf("%q", got)
	}
	if got := FormatFailureMessage(""); got != "Couldn't format: the server returned an invalid response." {
		t.Errorf("%q", got)
	}
}

func TestDocumentKey(t *testing.T) {
	if DocumentKey("a.go", "") != "a.go" || DocumentKey("a.go", "abc") != "abc:a.go" {
		t.Error("keys")
	}
}

func TestFormatCacheTiesEntriesToTheirSource(t *testing.T) {
	var cache FormatCache
	if !cache.Wanted("a", "one") || cache.Get("a", "one") != nil {
		t.Fatal("empty cache holds nothing")
	}
	cache.Set("a", "one", FormatState{Kind: FormatLoading})
	if cache.Wanted("a", "one") {
		t.Fatal("a request in flight is not requested again")
	}
	cache.Set("a", "one", FormatState{Kind: FormatReady, Text: "1"})
	if got := cache.Get("a", "one"); got == nil || got.Text != "1" {
		t.Fatalf("ready: %+v", got)
	}
	if cache.Get("a", "two") != nil || !cache.Wanted("a", "two") {
		t.Fatal("a changed file formats anew")
	}
	cache.Set("a", "one", FormatState{Kind: FormatFailed, Message: "bad"})
	if !cache.Wanted("a", "one") {
		t.Fatal("a failure is retried")
	}
	cache.Clear()
	if cache.Get("a", "one") != nil {
		t.Fatal("cleared")
	}
}

func TestFormatCacheIsBounded(t *testing.T) {
	var cache FormatCache
	for i := range formatCacheEntries + 2 {
		cache.Set(fmt.Sprint(i), "s", FormatState{Kind: FormatReady, Text: "x"})
	}
	if cache.Get("0", "s") != nil || cache.Get("1", "s") != nil || cache.Get("2", "s") == nil {
		t.Fatal("oldest entries leave first")
	}

	var big FormatCache
	text := strings.Repeat("x", formatCacheBytes/2+1)
	big.Set("a", "s", FormatState{Kind: FormatReady, Text: text})
	big.Set("b", "s", FormatState{Kind: FormatReady, Text: text})
	if big.Get("a", "s") != nil || big.Get("b", "s") == nil {
		t.Fatal("the byte bound drops the oldest and keeps the newest")
	}
	big.Set("c", "s", FormatState{Kind: FormatReady, Text: strings.Repeat("x", formatCacheBytes+1)})
	if big.Get("c", "s") == nil {
		t.Fatal("the newest stays over the bound")
	}
}

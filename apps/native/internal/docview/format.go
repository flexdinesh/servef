package docview

import (
	"hash/maphash"
	"slices"
	"strings"
)

// FormatKind is where a document's Format stands.
type FormatKind int

const (
	FormatLoading FormatKind = iota
	FormatReady
	FormatFailed
)

// FormatState is document-format.ts's FormatState.
type FormatState struct {
	Kind FormatKind
	// Text is FormatReady's text; Message FormatFailed's reason.
	Text, Message string
}

// FormatFailureMessage turns a server reason into the inline notice's
// sentence.
func FormatFailureMessage(reason string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(reason), ". \t\r\n")
	if trimmed == "" {
		trimmed = "the server returned an invalid response"
	}
	return "Couldn't format: " + trimmed + "."
}

// DocumentKey identifies a document, a snapshot apart from the working
// tree, as the web's documentKey.
func DocumentKey(path, at string) string {
	if at != "" {
		return at + ":" + path
	}
	return path
}

// Bounds of FormatCache: a formatted file is at most about the preview
// limit (4 MiB), so a few entries suffice, and the newest always stays.
const (
	formatCacheEntries = 8
	formatCacheBytes   = 32 << 20
)

type formatEntry struct {
	key    string
	source uint64
	state  FormatState
}

// FormatCache keeps Format per document as use-document-format.ts does,
// with two differences: entries hold a hash of the source they formatted,
// not the source, and it is bounded, oldest out first. A changed source
// misses, so the file formats anew.
type FormatCache struct {
	seed    maphash.Seed
	seeded  bool
	entries []formatEntry
}

func (c *FormatCache) hash(source string) uint64 {
	if !c.seeded {
		c.seed, c.seeded = maphash.MakeSeed(), true
	}
	return maphash.String(c.seed, source)
}

// Get returns key's state for source; nil when there is none.
func (c *FormatCache) Get(key, source string) *FormatState {
	hash := c.hash(source)
	for i := range c.entries {
		if entry := &c.entries[i]; entry.key == key && entry.source == hash {
			return &entry.state
		}
	}
	return nil
}

// Wanted reports whether key needs formatting for source: nothing held for
// it, or a failure to retry.
func (c *FormatCache) Wanted(key, source string) bool {
	state := c.Get(key, source)
	return state == nil || state.Kind == FormatFailed
}

// Set makes state key's newest entry for source.
func (c *FormatCache) Set(key, source string, state FormatState) {
	hash := c.hash(source)
	c.entries = slices.DeleteFunc(c.entries, func(entry formatEntry) bool { return entry.key == key })
	c.entries = append(c.entries, formatEntry{key, hash, state})
	total := 0
	for _, entry := range c.entries {
		total += len(entry.state.Text)
	}
	for len(c.entries) > 1 && (len(c.entries) > formatCacheEntries || total > formatCacheBytes) {
		total -= len(c.entries[0].state.Text)
		c.entries = slices.Delete(c.entries, 0, 1)
	}
}

// Clear drops every entry.
func (c *FormatCache) Clear() { c.entries = nil }

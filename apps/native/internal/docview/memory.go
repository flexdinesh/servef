package docview

import "slices"

// Place is what a document remembers while the user is elsewhere (decision
// 0008, use-document-navigation.ts): its chosen view, scroll, and selected
// lines. A view choice of "" lets the document open by preference.
type Place struct {
	Mode   Mode
	Scroll float64
	Lines  *LineRange
}

// Memory keeps Places by DocumentKey. The workspace reads it through this
// interface, so whoever owns open documents (tabs) can own their places.
type Memory interface {
	Place(key string) Place
	SetPlace(key string, place Place)
	Forget(key string)
}

// memoryLimit bounds NewMemory's documents.
const memoryLimit = 256

type placeEntry struct {
	key   string
	place Place
}

type boundedMemory struct{ entries []placeEntry }

// NewMemory returns a Memory of the most recently set documents.
func NewMemory() Memory { return &boundedMemory{} }

func (m *boundedMemory) index(key string) int {
	return slices.IndexFunc(m.entries, func(entry placeEntry) bool { return entry.key == key })
}

func (m *boundedMemory) Place(key string) Place {
	if i := m.index(key); i >= 0 {
		return m.entries[i].place
	}
	return Place{}
}

func (m *boundedMemory) SetPlace(key string, place Place) {
	m.Forget(key)
	m.entries = append(m.entries, placeEntry{key, place})
	if len(m.entries) > memoryLimit {
		m.entries = slices.Delete(m.entries, 0, 1)
	}
}

func (m *boundedMemory) Forget(key string) {
	if i := m.index(key); i >= 0 {
		m.entries = slices.Delete(m.entries, i, i+1)
	}
}

// Location is an entry of the navigation history: a document and its
// selected lines.
type Location struct {
	Path  string
	Lines *LineRange
}

// historyLimit bounds History's entries.
const historyLimit = 128

// History is back and forward over the documents opened, as the browser's
// history is on the web.
type History struct {
	entries []Location
	at      int
}

// Push records a newly opened location after the current one, dropping
// what was forward of it. The same document again replaces its entry.
func (h *History) Push(location Location) {
	if len(h.entries) > 0 {
		h.entries = h.entries[:h.at+1]
		if h.entries[h.at].Path == location.Path {
			h.entries[h.at] = location
			return
		}
	}
	h.entries = append(h.entries, location)
	if len(h.entries) > historyLimit {
		h.entries = slices.Delete(h.entries, 0, 1)
	}
	h.at = len(h.entries) - 1
}

// Replace updates the current location, as selecting lines replaces the
// web's history entry.
func (h *History) Replace(location Location) {
	if len(h.entries) > 0 {
		h.entries[h.at] = location
	}
}

// CanGoBack reports an earlier location.
func (h *History) CanGoBack() bool { return h.at > 0 }

// CanGoForward reports a later location.
func (h *History) CanGoForward() bool { return h.at < len(h.entries)-1 }

// Back moves to the previous location; ok is false at the start.
func (h *History) Back() (Location, bool) {
	if !h.CanGoBack() {
		return Location{}, false
	}
	h.at--
	return h.entries[h.at], true
}

// Forward moves to the next location; ok is false at the end.
func (h *History) Forward() (Location, bool) {
	if !h.CanGoForward() {
		return Location{}, false
	}
	h.at++
	return h.entries[h.at], true
}

// Reset forgets every location.
func (h *History) Reset() { h.entries, h.at = nil, 0 }

package main

import (
	"sync"
	"time"
)

// firstPage decides the window's first page. Links can arrive before the
// window exists: macOS delivers one at launch, and a second instance can
// send one while the daemon is still starting. The latest wins over what the
// arguments asked for.
type firstPage struct {
	mu      sync.Mutex
	opened  bool
	pending string
}

// open creates the window on the start page, or on the page a link asked for
// meanwhile. Requests wait until create has returned, so none is lost.
func (f *firstPage) open(start string, create func(first string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pending != "" {
		start = f.pending
	}
	create(start)
	f.opened = true
}

// request keeps target for the first page; false once the window exists and
// the caller loads it there.
func (f *firstPage) request(target string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.opened {
		return false
	}
	f.pending = target
	return true
}

// crashReloadEvery bounds reloading a crashed page. A page that crashes on
// load would otherwise reload forever.
const crashReloadEvery = 30 * time.Second

// reloadLimiter allows one reload per crashReloadEvery.
type reloadLimiter struct {
	mu   sync.Mutex
	last time.Time
}

func (r *reloadLimiter) allow(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.last.IsZero() && now.Sub(r.last) < crashReloadEvery {
		return false
	}
	r.last = now
	return true
}

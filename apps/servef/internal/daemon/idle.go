package daemon

import (
	"net/http"
	"sync"
	"time"
)

// idleTracker fires once after d with no request in flight. Every request
// start stops the timer; the last request's end re-arms it. It is armed at
// start; d == 0 never fires. Event streams count as in flight for their
// lifetime.
type idleTracker struct {
	mu      sync.Mutex
	active  int
	timer   *time.Timer
	d       time.Duration
	stopped bool
}

func newIdleTracker(d time.Duration, fire func()) *idleTracker {
	t := &idleTracker{d: d}
	if d > 0 {
		t.timer = time.AfterFunc(d, fire)
	}
	return t
}

func (t *idleTracker) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.begin()
		defer t.end()
		next.ServeHTTP(w, r)
	})
}

func (t *idleTracker) begin() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active++
	if t.timer != nil {
		t.timer.Stop()
	}
}

func (t *idleTracker) end() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active--
	if t.active == 0 && t.timer != nil && !t.stopped {
		t.timer.Reset(t.d)
	}
}

// quiet reports no request in flight on a daemon that may exit when idle; a
// tracker with no duration was asked never to stop.
func (t *idleTracker) quiet() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active == 0 && t.d > 0
}

func (t *idleTracker) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	if t.timer != nil {
		t.timer.Stop()
	}
}

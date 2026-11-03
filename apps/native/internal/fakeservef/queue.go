package fakeservef

import (
	"sync"
	"time"
)

// Queue is a workspace.Runner for tests: Go queues work and Post queues
// applies until the test runs them on its own goroutine, the UI thread of
// a test.
type Queue struct {
	mu    sync.Mutex
	work  []func() func()
	posts chan func()
}

// NewQueue returns an empty queue.
func NewQueue() *Queue { return &Queue{posts: make(chan func(), 256)} }

// Go queues work.
func (q *Queue) Go(work func() func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.work = append(q.work, work)
}

// Post queues an apply from any goroutine.
func (q *Queue) Post(apply func()) { q.posts <- apply }

// Drain runs queued work, applies, and posts until none remain.
func (q *Queue) Drain() {
	for {
		q.mu.Lock()
		if len(q.work) > 0 {
			work := q.work[0]
			q.work = q.work[1:]
			q.mu.Unlock()
			if apply := work(); apply != nil {
				apply()
			}
			continue
		}
		q.mu.Unlock()
		select {
		case apply := <-q.posts:
			apply()
		default:
			return
		}
	}
}

// Until drains until done reports true or timeout passes; it reports done.
func (q *Queue) Until(done func() bool, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		q.Drain()
		if done() {
			return true
		}
		select {
		case apply := <-q.posts:
			apply()
		case <-deadline:
			return false
		}
	}
}

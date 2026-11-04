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

// Hold takes the work queued so far off the queue, and returns what runs it
// and applies its results later: work a test keeps in flight while other work
// passes it.
func (q *Queue) Hold() func() {
	q.mu.Lock()
	held := q.work
	q.work = nil
	q.mu.Unlock()
	return func() {
		for _, work := range held {
			if apply := work(); apply != nil {
				apply()
			}
		}
	}
}

// StepUntil runs queued work, applies, and posts one at a time until done
// reports true, leaving what is left queued: a test can draw frames while it
// is in flight. It reports done.
func (q *Queue) StepUntil(done func() bool, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for !done() {
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
		case <-deadline:
			return false
		}
	}
	return true
}

// Step runs the first queued work and its apply, or one post; it reports
// whether anything ran. Tests use it to act between a load and its answer.
func (q *Queue) Step() bool {
	q.mu.Lock()
	if len(q.work) > 0 {
		work := q.work[0]
		q.work = q.work[1:]
		q.mu.Unlock()
		if apply := work(); apply != nil {
			apply()
		}
		return true
	}
	q.mu.Unlock()
	select {
	case apply := <-q.posts:
		apply()
		return true
	default:
		return false
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

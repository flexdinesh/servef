package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func idleFired(d time.Duration) (*idleTracker, <-chan time.Time) {
	fired := make(chan time.Time, 1)
	tracker := newIdleTracker(d, func() { fired <- time.Now() })
	return tracker, fired
}

func TestIdleTrackerFiresWithoutRequests(t *testing.T) {
	started := time.Now()
	tracker, fired := idleFired(50 * time.Millisecond)
	defer tracker.stop()
	select {
	case at := <-fired:
		if at.Sub(started) < 50*time.Millisecond {
			t.Fatalf("fired after %s", at.Sub(started))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle tracker never fired")
	}
}

func TestIdleTrackerWaitsForInFlightRequests(t *testing.T) {
	const d = 50 * time.Millisecond
	tracker, fired := idleFired(d)
	defer tracker.stop()
	release, entered := make(chan struct{}), make(chan struct{})
	handler := tracker.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		close(done)
	}()
	<-entered
	select {
	case <-fired:
		t.Fatal("fired while a request was in flight")
	case <-time.After(3 * d):
	}
	ended := time.Now()
	close(release)
	<-done
	select {
	case at := <-fired:
		if at.Sub(ended) < d {
			t.Fatalf("fired %s after the request ended, want at least %s", at.Sub(ended), d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle tracker never fired after the request ended")
	}
}

func TestIdleTrackerZeroNeverFires(t *testing.T) {
	tracker, fired := idleFired(0)
	defer tracker.stop()
	tracker.wrap(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	select {
	case <-fired:
		t.Fatal("idle tracker with d = 0 fired")
	case <-time.After(100 * time.Millisecond):
	}
}

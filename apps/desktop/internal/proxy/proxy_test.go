package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/servef/pkg/connection"
)

// fakeDaemon answers Socket with a real Unix socket a test server listens on.
type fakeDaemon struct {
	socket    atomic.Pointer[string]
	err       error
	reensured atomic.Int32
	onEnsure  func() string
	forced    atomic.Bool
}

func (d *fakeDaemon) Socket(_ context.Context, force bool) (string, error) {
	if force {
		d.forced.Store(true)
	}
	if d.err != nil {
		return "", d.err
	}
	return *d.socket.Load(), nil
}

func (d *fakeDaemon) Reensure(context.Context) (string, error) {
	d.reensured.Add(1)
	if d.onEnsure == nil {
		return "", errors.New("could not ensure")
	}
	socket := d.onEnsure()
	d.socket.Store(&socket)
	return socket, nil
}

// serve starts handler on a Unix socket and returns its path.
func serve(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "d.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	stop := func() { _ = server.Close() }
	t.Cleanup(stop)
	return socket, stop
}

func newProxy(t *testing.T, handler http.Handler) (*Proxy, *fakeDaemon) {
	t.Helper()
	socket, _ := serve(t, handler)
	daemon := &fakeDaemon{}
	daemon.socket.Store(&socket)
	p := New(NewConnections(daemon, &fakeStore{}))
	t.Cleanup(p.Close)
	return p, daemon
}

// fakeStore is the saved connections.
type fakeStore struct {
	mu    sync.Mutex
	saved map[string]connection.Saved
	err   error
	reads int
}

func (s *fakeStore) Lookup(id string) (connection.Saved, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	if s.err != nil {
		return connection.Saved{}, false, s.err
	}
	saved, found := s.saved[id]
	return saved, found, nil
}

func (s *fakeStore) put(saved connection.Saved) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saved == nil {
		s.saved = map[string]connection.Saved{}
	}
	s.saved[saved.ID] = saved
}

func request(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	// httptest moves the host off the URL; mygo keeps it there.
	r.Header = http.Header{}
	return r
}

func TestRefusals(t *testing.T) {
	reached := false
	p, _ := newProxy(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	for name, target := range map[string]string{
		"daemon":          Origin + "/_daemon/status",
		"encoded daemon":  Origin + "/%5Fdaemon/status",
		"repeated slash":  Origin + "//_daemon/status",
		"dot segments":    Origin + "/x/../_daemon/status",
		"encoded dots":    Origin + "/x/%2e%2e/_daemon/status",
		"servef control":  Origin + "/_servef/shutdown",
		"encoded slash":   Origin + "/_daemon/..%2fstatus",
		"encoded after":   Origin + "/_daemon%2f..%2fstatus",
		"servef dots":     Origin + "/_servef/..%2fstatus",
		"reached and out": Origin + "/x/%2e%2e/_daemon/..%2fstatus",
		"two slashes out": Origin + "//_daemon/..%2fstatus",
		"encoded servef":  Origin + "/_%73ervef/shutdown",
		"daemon no slash": Origin + "/_daemon",
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			p.ServeHTTP(recorder, request(http.MethodGet, target, nil))
			if recorder.Code != http.StatusForbidden {
				t.Errorf("status %d, want 403", recorder.Code)
			}
			var answer struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.Code != "forbidden" {
				t.Errorf("body %q", recorder.Body.String())
			}
		})
	}

	t.Run("other hosts", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, request(http.MethodGet, Scheme+"://other/", nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("status %d, want 404", recorder.Code)
		}
	})
	t.Run("other origins", func(t *testing.T) {
		for _, origin := range []string{"https://example.com", Scheme + "://other", "mygo://localhost", "null"} {
			r := request(http.MethodGet, Origin+"/api/projects", nil)
			r.Header.Set("Origin", origin)
			recorder := httptest.NewRecorder()
			p.ServeHTTP(recorder, r)
			if recorder.Code != http.StatusForbidden {
				t.Errorf("Origin %s: status %d, want 403", origin, recorder.Code)
			}
		}
	})
	if reached {
		t.Error("a refused request reached the daemon")
	}
}

func TestForwardsAllowlistedHeadersAndSetsDaemonChecks(t *testing.T) {
	var got *http.Request
	p, _ := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Add("Set-Thing", "a")
		w.Header().Add("Set-Thing", "b")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "body")
	}))
	r := request(http.MethodGet, Origin+"/api/page?project=%2Fa%20b&path=x%2Fy#frag", nil)
	for name, value := range map[string]string{
		"Accept": "application/json", "Range": "bytes=0-3", "Last-Event-Id": "7", "X-Servef-Language-Servers": "on",
		"If-None-Match": `"tag"`, "Cookie": "secret", "Authorization": "secret", "Sec-Fetch-Site": "cross-site", "Host": "evil", "Origin": Origin,
	} {
		r.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, r)

	for name, want := range map[string]string{
		"Accept": "application/json", "Range": "bytes=0-3", "Last-Event-Id": "7", "X-Servef-Language-Servers": "on", "If-None-Match": `"tag"`,
		"Sec-Fetch-Site": "same-origin", "Cookie": "", "Authorization": "", "Origin": "",
	} {
		if value := got.Header.Get(name); value != want {
			t.Errorf("upstream %s = %q, want %q", name, value, want)
		}
	}
	if got.Host != "localhost" {
		t.Errorf("upstream Host = %q", got.Host)
	}
	if got.URL.RequestURI() != "/api/page?project=%2Fa%20b&path=x%2Fy" {
		t.Errorf("upstream target = %q", got.URL.RequestURI())
	}
	if recorder.Code != http.StatusTeapot || recorder.Body.String() != "body" {
		t.Errorf("response %d %q", recorder.Code, recorder.Body.String())
	}
	header := recorder.Result().Header
	if header.Get("Content-Security-Policy") != "default-src 'none'" {
		t.Errorf("the daemon's CSP was not kept: %v", header)
	}
	if header.Get("Keep-Alive") != "" {
		t.Errorf("hop-by-hop header passed: %v", header)
	}
	if len(header["Set-Thing"]) != 2 {
		t.Errorf("repeated header lost: %v", header)
	}
}

// WebKit sends no Content-Length; the body must still arrive.
func TestRequestBodyWithoutContentLength(t *testing.T) {
	var got string
	var length int64
	p, _ := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got, length = string(data), r.ContentLength
	}))
	r := request(http.MethodPost, Origin+"/api/walkthroughs", strings.NewReader(`{"a":1}`))
	r.ContentLength = 0
	p.ServeHTTP(httptest.NewRecorder(), r)
	if got != `{"a":1}` || length != 7 {
		t.Errorf("upstream body %q, length %d", got, length)
	}

	r = request(http.MethodPost, Origin+"/api/walkthroughs", strings.NewReader(strings.Repeat("x", maxBody+1)))
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, r)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: status %d", recorder.Code)
	}
}

func TestBodylessResponsesDropContentLength(t *testing.T) {
	p, _ := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "12")
		if r.Method != http.MethodHead {
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, request(method, Origin+"/assets/a.js", nil))
		if recorder.Result().Header.Get("Content-Length") != "" {
			t.Errorf("%s kept Content-Length", method)
		}
	}
}

// streamWriter is the webview's side: it records writes and flushes as they
// happen, and fails writes once the page stopped reading.
type streamWriter struct {
	header  http.Header
	mu      sync.Mutex
	written strings.Builder
	flushes int
	gone    bool
	cancel  context.CancelFunc
	chunks  chan string
}

func (w *streamWriter) Header() http.Header { return w.header }
func (w *streamWriter) WriteHeader(int)     {}
func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.gone {
		w.cancel() // as mygo's scheme writer on EPIPE
		return 0, io.ErrClosedPipe
	}
	w.written.Write(p)
	return len(p), nil
}

func (w *streamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushes++
	text := w.written.String()
	w.written.Reset()
	if text != "" {
		w.chunks <- text
	}
}

func TestEventStreamArrivesUnbufferedWithKeepalivesAndEndsUpstream(t *testing.T) {
	next := make(chan string)
	ended := make(chan struct{})
	p, _ := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		for {
			select {
			case text := <-next:
				_, _ = io.WriteString(w, text)
				w.(http.Flusher).Flush()
			case <-r.Context().Done():
				close(ended)
				return
			}
		}
	}))
	p.keepalive = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &streamWriter{header: http.Header{}, cancel: cancel, chunks: make(chan string, 1024)}
	r := request(http.MethodGet, Origin+"/api/events?project=x", nil).WithContext(ctx)
	r.Header.Set("Accept", "text/event-stream")
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.ServeHTTP(writer, r)
	}()

	// waitFor reads chunks until one holds want.
	waitFor := func(want string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case chunk := <-writer.chunks:
				if strings.Contains(chunk, want) {
					return
				}
			case <-deadline:
				t.Fatalf("no chunk with %q", want)
			}
		}
	}
	for i := range 3 {
		event := fmt.Sprintf("event: change\ndata: %d\n\n", i)
		next <- event
		// Each event arrives while the stream stays open: nothing buffers.
		waitFor(event)
	}
	waitFor(": keepalive\n\n")

	// The page closes the stream: the next write fails and the upstream
	// request ends, though the daemon has nothing to send.
	writer.mu.Lock()
	writer.gone = true
	writer.mu.Unlock()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the upstream request outlived the page's stream")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not return")
	}
}

func TestPlainResponsesGetNoKeepalive(t *testing.T) {
	p, _ := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.(http.Flusher).Flush()
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, "{}")
	}))
	p.keepalive = 10 * time.Millisecond
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, request(http.MethodGet, Origin+"/api/projects", nil))
	if recorder.Body.String() != "{}" {
		t.Errorf("body %q", recorder.Body.String())
	}
}

func TestDaemonGone(t *testing.T) {
	hello := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello") })
	gone := func(t *testing.T) (*Proxy, *fakeDaemon) {
		t.Helper()
		socket, stop := serve(t, hello)
		stop()
		daemon := &fakeDaemon{}
		daemon.socket.Store(&socket)
		p := New(NewConnections(daemon, &fakeStore{}))
		t.Cleanup(p.Close)
		return p, daemon
	}

	t.Run("a read retries once after ensuring", func(t *testing.T) {
		p, daemon := gone(t)
		daemon.onEnsure = func() string {
			socket, _ := serve(t, hello)
			return socket
		}
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, request(http.MethodGet, Origin+"/api/projects", nil))
		if recorder.Body.String() != "hello" || daemon.reensured.Load() != 1 {
			t.Errorf("body %q after %d ensures", recorder.Body.String(), daemon.reensured.Load())
		}
	})
	t.Run("a write is not sent twice", func(t *testing.T) {
		p, daemon := gone(t)
		daemon.onEnsure = func() string {
			socket, _ := serve(t, hello)
			return socket
		}
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, request(http.MethodPost, Origin+"/api/walkthroughs", strings.NewReader("{}")))
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "daemonRestarted") {
			t.Errorf("status %d, body %q", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("an api read gets JSON when ensuring fails", func(t *testing.T) {
		p, _ := gone(t)
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, request(http.MethodGet, Origin+"/api/projects", nil))
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "daemonUnavailable") {
			t.Errorf("status %d, body %q", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("a page load gets an escaped failure page and forces an ensure", func(t *testing.T) {
		daemon := &fakeDaemon{err: errors.New("<script>x</script>")}
		p := New(NewConnections(daemon, &fakeStore{}))
		r := request(http.MethodGet, Origin+"/", nil)
		r.Header.Set("Accept", "text/html")
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, r)
		body := recorder.Body.String()
		if recorder.Code != http.StatusServiceUnavailable || strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
			t.Errorf("status %d, body %q", recorder.Code, body)
		}
		if !strings.Contains(recorder.Result().Header.Get("Content-Security-Policy"), "default-src 'none'") {
			t.Error("the failure page has no CSP")
		}
		if !daemon.forced.Load() {
			t.Error("a page load did not force an ensure")
		}
	})
}

// A comment inside an event would end it early for EventSource.
func TestKeepaliveWaitsForTheEndOfAnEvent(t *testing.T) {
	p, _ := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first half\n")
		w.(http.Flusher).Flush()
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, "\n")
		w.(http.Flusher).Flush()
		time.Sleep(100 * time.Millisecond)
	}))
	p.keepalive = 10 * time.Millisecond
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, request(http.MethodGet, Origin+"/api/events", nil))
	body := recorder.Body.String()
	if !strings.HasPrefix(body, "data: first half\n\n") {
		t.Errorf("the event was split: %q", body)
	}
	if !strings.Contains(body, ": keepalive\n\n") {
		t.Errorf("no keepalive after the event: %q", body)
	}
}

// The page shown when the web process keeps crashing is the proxy's own: it
// must load whatever state the daemon is in, and offer a way back.
func TestCrashedPageIsNotForwarded(t *testing.T) {
	reached := false
	p, _ := newProxy(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, request(http.MethodGet, Origin+CrashedPath, nil))
	if recorder.Code != http.StatusOK || reached {
		t.Fatalf("status %d, reached the daemon: %v", recorder.Code, reached)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "crashed") || !strings.Contains(body, `href="/"`) {
		t.Fatalf("page %q", body)
	}
}

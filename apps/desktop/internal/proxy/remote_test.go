package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/pkg/connection"
)

// pinnedServer is a TLS server with a self-signed certificate, and the
// connection that pins it.
type pinnedServer struct {
	server *httptest.Server
	saved  connection.Saved
	token  string
}

func startPinned(t *testing.T, handler http.Handler) *pinnedServer {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	return &pinnedServer{server: server, saved: savedFor(t, server, [connection.TokenSize]byte{7}), token: connection.Credential{Token: [connection.TokenSize]byte{7}}.TokenText()}
}

func savedFor(t *testing.T, server *httptest.Server, token [connection.TokenSize]byte) connection.Saved {
	t.Helper()
	certificate, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	credential := connection.Credential{Fingerprint: connection.FingerprintOf(certificate.RawSubjectPublicKeyInfo), Token: token}
	tcp, ok := server.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener %s on %s:%s is not TCP", server.Listener.Addr(), host, portText)
	}
	return connection.Saved{ID: credential.ID(), Label: "box", Hosts: []string{host}, Port: tcp.Port, Credential: credential.Encode()}
}

func remoteProxy(t *testing.T, saved ...connection.Saved) (*Proxy, *fakeStore) {
	t.Helper()
	store := &fakeStore{}
	for _, entry := range saved {
		store.put(entry)
	}
	p := New(NewConnections(&fakeDaemon{}, store))
	t.Cleanup(p.Close)
	return p, store
}

func TestRemoteForwardsOverPinnedTLSAndAddsTheCredentialOnlyUpstream(t *testing.T) {
	var got *http.Request
	remote := startPinned(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		_, _ = io.WriteString(w, "from the server")
	}))
	p, _ := remoteProxy(t, remote.saved)
	r := request(http.MethodGet, OriginOf(remote.saved.ID)+"/api/projects?a=b", nil)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Authorization", "Bearer from-the-page")
	r.Header.Set("Cookie", "page=cookie")
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, r)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "from the server" {
		t.Fatalf("response %d %q", recorder.Code, recorder.Body.String())
	}
	if want := "Bearer " + remote.token; got.Header.Get("Authorization") != want {
		t.Errorf("upstream Authorization = %q, want %q", got.Header.Get("Authorization"), want)
	}
	if got.Header.Get("Cookie") != "" || got.Host != "localhost" || got.URL.RequestURI() != "/api/projects?a=b" {
		t.Errorf("upstream = %+v", got)
	}
	// Neither the token nor the credential reaches the page.
	for name, values := range recorder.Result().Header {
		if strings.Contains(strings.Join(values, ","), remote.token) {
			t.Errorf("response header %s carries the token", name)
		}
	}
}

func TestEveryConnectionRefusesControlAPIsAndOtherOrigins(t *testing.T) {
	reached := false
	remote := startPinned(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	p, _ := remoteProxy(t, remote.saved)
	origin := OriginOf(remote.saved.ID)
	for _, path := range []string{"/_daemon/status", "/_servef/remote", "/%5Fdaemon/status", "/x/../_daemon/stop", "/_daemon/..%2fstatus"} {
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, request(http.MethodGet, origin+path, nil))
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", path, recorder.Code)
		}
	}
	// One connection's page cannot read another's, or the local daemon's.
	for _, from := range []string{Origin, OriginOf("r-000000000000"), "https://example.com", "null"} {
		r := request(http.MethodGet, origin+"/api/projects", nil)
		r.Header.Set("Origin", from)
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, r)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("from %s: status %d, want 403", from, recorder.Code)
		}
	}
	r := request(http.MethodGet, Origin+"/api/projects", nil)
	r.Header.Set("Origin", origin)
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, r)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("a remote page reading the local daemon: status %d", recorder.Code)
	}
	if reached {
		t.Error("a refused request reached the server")
	}
	same := request(http.MethodPost, origin+"/api/walkthroughs", strings.NewReader("{}"))
	same.Header.Set("Origin", origin)
	recorder = httptest.NewRecorder()
	p.ServeHTTP(recorder, same)
	if recorder.Code == http.StatusForbidden || !reached {
		t.Errorf("a page's own origin: status %d, reached %v", recorder.Code, reached)
	}
}

func TestUnknownConnections(t *testing.T) {
	p, _ := remoteProxy(t)
	for _, host := range []string{"r-000000000000", "other", "10.0.0.5:7971"} {
		r := request(http.MethodGet, OriginOf(host)+"/", nil)
		r.Header.Set("Accept", "text/html")
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, r)
		if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "servef connect") {
			t.Errorf("%s: status %d, body %q", host, recorder.Code, recorder.Body.String())
		}
		recorder = httptest.NewRecorder()
		p.ServeHTTP(recorder, request(http.MethodGet, OriginOf(host)+"/api/projects", nil))
		var answer struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.Code != "unknownConnection" {
			t.Errorf("%s api: %q", host, recorder.Body.String())
		}
	}
}

func TestRemoteFailuresSayWhatToDo(t *testing.T) {
	document := func(t *testing.T, saved connection.Saved) (int, string) {
		t.Helper()
		p, _ := remoteProxy(t, saved)
		r := request(http.MethodGet, OriginOf(saved.ID)+"/", nil)
		r.Header.Set("Accept", "text/html")
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, r)
		return recorder.Code, recorder.Body.String()
	}
	t.Run("the server has gone", func(t *testing.T) {
		remote := startPinned(t, http.NotFoundHandler())
		saved := remote.saved
		remote.server.Close()
		code, body := document(t, saved)
		if code != http.StatusServiceUnavailable || !strings.Contains(body, "box") || !strings.Contains(body, "--remote") || !strings.Contains(body, `href="/"`) {
			t.Errorf("status %d, body %q", code, body)
		}
	})
	t.Run("another certificate answers", func(t *testing.T) {
		remote := startPinned(t, http.NotFoundHandler())
		saved := remote.saved
		other, err := connection.ParseCredential(saved.Credential)
		if err != nil {
			t.Fatal(err)
		}
		other.Fingerprint[0]++
		saved.Credential, saved.ID = other.Encode(), other.ID()
		code, body := document(t, saved)
		if code != http.StatusBadGateway || !strings.Contains(body, "different identity") {
			t.Errorf("status %d, body %q", code, body)
		}
	})
	t.Run("the server refuses the token", func(t *testing.T) {
		remote := startPinned(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
		code, body := document(t, remote.saved)
		if code != http.StatusUnauthorized || !strings.Contains(body, "refused") {
			t.Errorf("status %d, body %q", code, body)
		}
	})
	t.Run("a page's own request stays JSON", func(t *testing.T) {
		remote := startPinned(t, http.NotFoundHandler())
		saved := remote.saved
		remote.server.Close()
		p, _ := remoteProxy(t, saved)
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, request(http.MethodGet, OriginOf(saved.ID)+"/api/projects", nil))
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "remoteUnavailable") {
			t.Errorf("status %d, body %q", recorder.Code, recorder.Body.String())
		}
	})
}

// `servef connect` with new addresses applies on the next page load, without
// restarting the app; a page's own requests do not read the file.
func TestAPageLoadRereadsTheStore(t *testing.T) {
	first := startPinned(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "first") }))
	p, store := remoteProxy(t, first.saved)
	origin := OriginOf(first.saved.ID)
	get := func(path, accept string) string {
		t.Helper()
		r := request(http.MethodGet, origin+path, nil)
		r.Header.Set("Accept", accept)
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, r)
		return recorder.Body.String()
	}
	if got := get("/", "text/html"); got != "first" {
		t.Fatalf("page %q", got)
	}
	store.mu.Lock()
	reads := store.reads
	store.mu.Unlock()
	if got := get("/api/projects", "application/json"); got != "first" {
		t.Fatalf("api %q", got)
	}
	store.mu.Lock()
	if store.reads != reads {
		t.Errorf("a page's request read the store")
	}
	store.mu.Unlock()

	// Same identity, another address: same certificate behind a new listener.
	second := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "second") }))
	second.TLS = &tls.Config{Certificates: first.server.TLS.Certificates}
	second.StartTLS()
	t.Cleanup(second.Close)
	moved := first.saved
	tcp, ok := second.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("not TCP")
	}
	moved.Port = tcp.Port
	store.put(moved)
	if got := get("/", "text/html"); got != "second" {
		t.Errorf("after a new connect, page %q", got)
	}

	// Forgotten: a page load says so.
	store.mu.Lock()
	delete(store.saved, moved.ID)
	store.mu.Unlock()
	if got := get("/", "text/html"); !strings.Contains(got, "servef connect") {
		t.Errorf("forgotten connection: %q", got)
	}
}

func TestPageLoadsSurviveAStoreThatCannotBeRead(t *testing.T) {
	remote := startPinned(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	p, store := remoteProxy(t, remote.saved)
	get := func() *httptest.ResponseRecorder {
		r := request(http.MethodGet, OriginOf(remote.saved.ID)+"/", nil)
		r.Header.Set("Accept", "text/html")
		recorder := httptest.NewRecorder()
		p.ServeHTTP(recorder, r)
		return recorder
	}
	if got := get(); got.Body.String() != "ok" {
		t.Fatalf("page %q", got.Body.String())
	}
	store.mu.Lock()
	store.err = io.ErrUnexpectedEOF
	store.mu.Unlock()
	if got := get(); got.Body.String() != "ok" {
		t.Errorf("a window open before the failure: %q", got.Body.String())
	}
}

func TestKnownAndLabel(t *testing.T) {
	store := &fakeStore{}
	store.put(connection.Saved{ID: "r-0123456789ab", Label: "box"})
	connections := NewConnections(&fakeDaemon{}, store)
	for host, want := range map[string]bool{Host: true, "r-0123456789ab": true, "r-000000000000": false, "other": false} {
		if got := connections.Known(host); got != want {
			t.Errorf("Known(%q) = %v", host, got)
		}
	}
	if got := connections.Label("r-0123456789ab"); got != "box" {
		t.Errorf("Label = %q", got)
	}
}

// A remote server lost mid-stream ends the page's stream, so the page opens
// another and says it is reconnecting rather than showing "Live" over nothing.
func TestRemoteStreamEndsWithItsConnection(t *testing.T) {
	sent := make(chan struct{})
	lost := make(chan struct{})
	remote := startPinned(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: status\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		close(sent)
		<-lost
		panic(http.ErrAbortHandler)
	}))
	p, _ := remoteProxy(t, remote.saved)
	p.keepalive = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &streamWriter{header: http.Header{}, cancel: cancel, chunks: make(chan string, 16)}
	r := request(http.MethodGet, OriginOf(remote.saved.ID)+"/api/events?project=x", nil).WithContext(ctx)
	r.Header.Set("Accept", "text/event-stream")
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.ServeHTTP(writer, r)
	}()
	<-sent
	select {
	case chunk := <-writer.chunks:
		if !strings.Contains(chunk, "event: status") {
			t.Errorf("first chunk = %q", chunk)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event reached the page")
	}
	close(lost)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the page's stream outlived the server's")
	}
}

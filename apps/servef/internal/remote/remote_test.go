package remote

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/pkg/connection"
	"github.com/flexdinesh/servef/pkg/contract"
)

func newIdentityFile(t *testing.T) (*Identity, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "remote", "identity.json")
	identity, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	return identity, path
}

func TestIdentityPersistsPrivatelyUntilRotated(t *testing.T) {
	identity, path := newIdentityFile(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("identity mode = %o, want 600", info.Mode().Perm())
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if directory.Mode().Perm() != 0o700 {
		t.Errorf("directory mode = %o, want 700", directory.Mode().Perm())
	}
	again, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Credential != identity.Credential {
		t.Error("a reload changed the credential; restarts would break saved connections")
	}
	rotated, err := RotateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Credential.Fingerprint == identity.Credential.Fingerprint || rotated.Credential.Token == identity.Credential.Token {
		t.Error("rotation kept a secret")
	}
	if rotated.Credential.ID() == identity.Credential.ID() {
		t.Error("rotation kept the id")
	}
}

func TestIdentityKeepsItsPortAcrossReloadAndRotation(t *testing.T) {
	identity, path := newIdentityFile(t)
	if err := identity.RememberPort(7975); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Port != 7975 || reloaded.Credential != identity.Credential {
		t.Errorf("reloaded = port %d, same credential %t", reloaded.Port, reloaded.Credential == identity.Credential)
	}
	rotated, err := RotateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Port != 7975 {
		t.Errorf("rotated port = %d", rotated.Port)
	}
}

func TestIdentityRefusesAWidenedFile(t *testing.T) {
	_, path := newIdentityFile(t)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateIdentity(path); err == nil || !strings.Contains(err.Error(), "servef remote rotate") {
		t.Errorf("LoadOrCreateIdentity() = %v, want a refusal naming rotate", err)
	}
}

func TestIdentityRefusesMalformedFiles(t *testing.T) {
	for name, content := range map[string]string{
		"not json": "{",
		"version":  `{"version": 9}`,
		"no key":   `{"version": 1, "certificate": "", "key": "", "token": "AAAA"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "identity.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadOrCreateIdentity(path); err == nil {
				t.Error("LoadOrCreateIdentity() = nil error")
			}
		})
	}
}

// serveTLS serves handler as a remote server does and returns its endpoint.
func serveTLS(t *testing.T, identity *Identity, handler http.Handler) connection.TLS {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	_ = server.Listener.Close()
	server.Listener = listener
	server.TLS = identity.TLSConfig()
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	port, err := Port(listener)
	if err != nil {
		t.Fatal(err)
	}
	return connection.TLS{Hosts: []string{"127.0.0.1"}, Port: port, Credential: identity.Credential}
}

var testInfo = contract.RemoteInfo{Protocol: 1, Version: "1.2.3", Name: "box"}

func authed(identity *Identity, next http.Handler) http.Handler {
	return Authenticate(identity.Credential.TokenText(), testInfo, next)
}

func TestProbeFindsTheServerAndReportsWhatFailed(t *testing.T) {
	identity, _ := newIdentityFile(t)
	endpoint := serveTLS(t, identity, authed(identity, http.NotFoundHandler()))
	ctx := context.Background()

	t.Run("right credential", func(t *testing.T) {
		found, info, err := connection.Probe(ctx, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if info != testInfo || found.Hosts[0] != "127.0.0.1" {
			t.Errorf("Probe() = %+v, %+v", found, info)
		}
	})
	t.Run("the working address moves first", func(t *testing.T) {
		silent := endpoint
		silent.Hosts = []string{"127.0.0.2", "127.0.0.1"}
		found, _, err := connection.Probe(ctx, silent)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(found.Hosts, []string{"127.0.0.1", "127.0.0.2"}) {
			t.Errorf("Hosts = %v", found.Hosts)
		}
	})
	t.Run("another certificate", func(t *testing.T) {
		wrong := endpoint
		wrong.Credential.Fingerprint[0]++
		if _, _, err := connection.Probe(ctx, wrong); !errors.Is(err, connection.ErrFingerprint) {
			t.Errorf("Probe() = %v, want ErrFingerprint", err)
		}
	})
	t.Run("another token", func(t *testing.T) {
		wrong := endpoint
		wrong.Credential.Token[0]++
		if _, _, err := connection.Probe(ctx, wrong); !errors.Is(err, connection.ErrToken) {
			t.Errorf("Probe() = %v, want ErrToken", err)
		}
	})
	t.Run("nothing listening", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port, err := Port(listener)
		if err != nil {
			t.Fatal(err)
		}
		_ = listener.Close()
		gone := endpoint
		gone.Port = port
		if _, _, err := connection.Probe(ctx, gone); !errors.Is(err, connection.ErrUnreachable) {
			t.Errorf("Probe() = %v, want ErrUnreachable", err)
		}
	})
	t.Run("a pin beats silence", func(t *testing.T) {
		wrong := endpoint
		wrong.Credential.Fingerprint[0]++
		wrong.Hosts = []string{"127.0.0.2", "127.0.0.1"}
		if _, _, err := connection.Probe(ctx, wrong); !errors.Is(err, connection.ErrFingerprint) {
			t.Errorf("Probe() = %v, want ErrFingerprint", err)
		}
	})
}

// The pin holds without a matching host name, which is what makes one string
// work through any route.
func TestRoundTripperSendsTheTokenOverAPinnedConnection(t *testing.T) {
	identity, _ := newIdentityFile(t)
	var gotHost, gotProtocol string
	endpoint := serveTLS(t, identity, authed(identity, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost, gotProtocol = r.Host, r.Proto
		_, _ = io.WriteString(w, "reached")
	})))
	client := &http.Client{Transport: endpoint.RoundTripper()}
	response, err := client.Get(endpoint.BaseURL() + "/api/projects")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(body) != "reached" {
		t.Fatalf("answer = %d %q", response.StatusCode, body)
	}
	if gotHost != "localhost" {
		t.Errorf("Host = %q", gotHost)
	}
	if gotProtocol != "HTTP/2.0" {
		t.Errorf("protocol = %q, want HTTP/2", gotProtocol)
	}
}

func TestAuthenticate(t *testing.T) {
	const token = "the-token"
	var reached *http.Request
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = r
		w.WriteHeader(http.StatusTeapot)
	})
	handler := Authenticate(token, testInfo, next)
	serve := func(target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
		reached = nil
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Host = "10.0.0.5:7971"
		if mutate != nil {
			mutate(request)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	bearer := func(value string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+value) }
	}
	cookie := func(value string) func(*http.Request) {
		return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: sessionCookie, Value: value}) }
	}

	refused := map[string]*httptest.ResponseRecorder{
		"no credential":     serve("/api/projects", nil),
		"wrong bearer":      serve("/api/projects", bearer("nope")),
		"wrong cookie":      serve("/api/projects", cookie("nope")),
		"empty bearer":      serve("/api/projects", bearer("")),
		"wrong token query": serve("/?token=nope", nil),
		"token twice":       serve("/?token=nope&token="+token, nil),
		"info unauthorized": serve(contract.RemoteInfoPath, nil),
		"control API":       serve("/_daemon/status", nil),
		"token off a page":  serve("/api/projects?token="+token, nil),
	}
	for name, recorder := range refused {
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", name, recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), token) {
			t.Errorf("%s leaks the token: %s", name, recorder.Body)
		}
	}
	if reached != nil {
		t.Error("a refused request reached the web app")
	}

	for name, mutate := range map[string]func(*http.Request){"bearer": bearer(token), "cookie": cookie(token)} {
		if got := serve("/api/projects", mutate); got.Code != http.StatusTeapot {
			t.Errorf("%s = %d, want the web app's answer", name, got.Code)
		}
		if reached == nil || reached.Host != "localhost" {
			t.Errorf("%s: the web app saw Host %v", name, reached)
		}
	}
	if got := serve(contract.RemoteInfoPath, bearer(token)); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"name":"box"`) {
		t.Errorf("info = %d %s", got.Code, got.Body)
	}
}

func TestTokenLinkSwapsForAPrivateCookie(t *testing.T) {
	const token = "the-token"
	handler := Authenticate(token, testInfo, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "/view?project=%2Fa&token="+token+"&path=b.md", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", recorder.Code)
	}
	location := recorder.Header().Get("Location")
	if strings.Contains(location, token) {
		t.Errorf("Location %q keeps the token", location)
	}
	if !strings.HasPrefix(location, "/view?") || !strings.Contains(location, "project=%2Fa") || !strings.Contains(location, "path=b.md") {
		t.Errorf("Location %q lost the page", location)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookie || cookies[0].Value != token {
		t.Fatalf("cookies = %v", cookies)
	}
	if !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Strict", cookies[0])
	}
	if recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Error("the redirect can leak the token through Referer")
	}
}

// Browser writes pass the web app's guards from the server's own origin only.
func TestAuthenticateKeepsForeignOrigins(t *testing.T) {
	var origin string
	handler := Authenticate("t", testInfo, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { origin = r.Header.Get("Origin") }))
	for sent, want := range map[string]string{
		"https://box.example:7971": "",
		"https://evil.example":     "https://evil.example",
		"http://box.example:7971":  "http://box.example:7971",
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/walkthroughs", nil)
		request.Host = "box.example:7971"
		request.Header.Set("Authorization", "Bearer t")
		request.Header.Set("Origin", sent)
		handler.ServeHTTP(httptest.NewRecorder(), request)
		if origin != want {
			t.Errorf("Origin %q reached the web app as %q, want %q", sent, origin, want)
		}
	}
}

func decode(t *testing.T, data []byte) string {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(plain)
}

func TestCompress(t *testing.T) {
	large := strings.Repeat("servef ", 2000)
	serve := func(acceptEncoding, contentType string, length string, status int, body string) *httptest.ResponseRecorder {
		handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			if length != "" {
				w.Header().Set("Content-Length", length)
			}
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}))
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		if acceptEncoding != "" {
			request.Header.Set("Accept-Encoding", acceptEncoding)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	for _, contentType := range []string{"application/json; charset=utf-8", "text/html; charset=utf-8", "application/javascript", "image/svg+xml", "application/wasm"} {
		got := serve("gzip", contentType, "", http.StatusOK, large)
		if got.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("%s was not compressed", contentType)
			continue
		}
		if got.Body.Len() >= len(large) {
			t.Errorf("%s grew to %d bytes", contentType, got.Body.Len())
		}
		if decode(t, got.Body.Bytes()) != large {
			t.Errorf("%s did not round trip", contentType)
		}
	}

	passthrough := map[string]*httptest.ResponseRecorder{
		"no gzip accepted": serve("", "application/json", "", http.StatusOK, large),
		"gzip refused":     serve("gzip;q=0", "application/json", "", http.StatusOK, large),
		"event stream":     serve("gzip", "text/event-stream", "", http.StatusOK, large),
		"image":            serve("gzip", "image/png", "", http.StatusOK, large),
		"small":            serve("gzip", "application/json", "2", http.StatusOK, "{}"),
	}
	for name, got := range passthrough {
		if got.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s was compressed", name)
		}
	}
	if got := serve("gzip", "application/json", "", http.StatusNotModified, ""); got.Header().Get("Content-Encoding") != "" {
		t.Error("a 304 was compressed")
	}
	if got := serve("gzip", "application/json", "", http.StatusOK, large); got.Header().Get("Content-Length") != "" {
		t.Error("a compressed answer kept the plain length")
	}
	if got := serve("gzip", "application/json", "", http.StatusOK, large); !strings.Contains(got.Header().Get("Vary"), "Accept-Encoding") {
		t.Error("no Vary: Accept-Encoding")
	}
}

// Flushing reaches the client as it goes, and deadlines reach the connection,
// so nothing that streams through the writer stalls or times out differently.
func TestCompressFlushesAndUnwraps(t *testing.T) {
	flushed := make(chan string, 1)
	var deadlineErr error
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "first")
		controller := http.NewResponseController(w)
		deadlineErr = controller.SetWriteDeadline(time.Now().Add(time.Minute))
		_ = controller.Flush()
		<-flushed
	}))
	server := httptest.NewServer(handler)
	defer server.Close()
	transport := &http.Transport{DisableCompression: true}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept-Encoding", "gzip")
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, len("first"))
	if _, err := io.ReadFull(reader, buffer); err != nil {
		t.Fatalf("read before the handler ended: %v", err)
	}
	flushed <- "done"
	if string(buffer) != "first" {
		t.Errorf("read %q", buffer)
	}
	if deadlineErr != nil {
		t.Errorf("SetWriteDeadline through the writer: %v", deadlineErr)
	}
}

func TestListenPrefersTheRememberedPortAndFallsBack(t *testing.T) {
	first, err := Listen("127.0.0.1", new(int), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	busy, err := Port(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Listen("127.0.0.1", &busy, 0); !errors.Is(err, ErrPortBusy) {
		t.Errorf("an explicit busy port = %v, want ErrPortBusy", err)
	}
	fallback, err := Listen("127.0.0.1", nil, busy)
	if err != nil {
		t.Skipf("default range unavailable here: %v", err)
	}
	defer fallback.Close()
	port, err := Port(fallback)
	if err != nil {
		t.Fatal(err)
	}
	if port < defaultPortStart || port > defaultPortEnd {
		t.Errorf("fallback port = %d", port)
	}
}

func TestCandidatesOrderAndFilter(t *testing.T) {
	ip := net.ParseIP
	interfaces := []Interface{
		{Name: "docker0", Addresses: []net.IP{ip("172.17.0.1")}},
		{Name: "br-abc", Addresses: []net.IP{ip("172.18.0.1")}},
		{Name: "wlan0", Addresses: []net.IP{ip("fe80::1"), ip("2001:db8::5"), ip("192.168.1.5")}},
		{Name: "tailscale0", Addresses: []net.IP{ip("100.64.1.2")}},
	}
	for name, test := range map[string]struct {
		ssh   string
		route net.IP
		want  []string
	}{
		"ssh, route, then interfaces": {"10.1.1.1 5000 10.0.0.5 22", ip("192.168.1.5"), []string{"10.0.0.5", "192.168.1.5", "100.64.1.2", "2001:db8::5"}},
		"no ssh":                      {"", ip("192.168.1.5"), []string{"192.168.1.5", "100.64.1.2", "2001:db8::5"}},
		"malformed ssh ignored":       {"garbage", nil, []string{"192.168.1.5", "100.64.1.2", "2001:db8::5"}},
		"loopback ssh ignored":        {"::1 5000 ::1 22", nil, []string{"192.168.1.5", "100.64.1.2", "2001:db8::5"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Candidates(test.ssh, test.route, interfaces); !slices.Equal(got, test.want) {
				t.Errorf("Candidates() = %v, want %v", got, test.want)
			}
		})
	}
	many := []Interface{{Name: "eth0"}}
	for i := range 10 {
		many[0].Addresses = append(many[0].Addresses, ip(fmt.Sprintf("10.0.0.%d", i+1)))
	}
	if got := Candidates("", nil, many); len(got) != maxCandidates {
		t.Errorf("Candidates() listed %d addresses", len(got))
	}
	if got := Candidates("", nil, nil); len(got) != 0 {
		t.Errorf("Candidates() with no network = %v", got)
	}
}

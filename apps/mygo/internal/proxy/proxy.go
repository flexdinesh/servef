// Package proxy serves the app's custom scheme from the daemon's Unix socket
// (decision 0094), with the rules of the Electron app's servef-app:// proxy
// (decision 0067, apps/electron/src/main/proxy.ts, forward.ts, headers.ts):
// pages reach /api and the web app, never the control endpoints, and the
// daemon sees what its checks expect.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/servef/apps/mygo/internal/connect"
	"github.com/flexdinesh/servef/pkg/contract"
)

const (
	// Scheme is the custom scheme pages load from; Host its only host.
	Scheme = "servef-mygo-app"
	Host   = "local"
	// Origin is the only origin the app's pages have.
	Origin = Scheme + "://" + Host
	// CrashedPath is the page shown when the web process keeps crashing. It
	// is the proxy's own, so it loads whatever the page or the daemon does.
	CrashedPath = "/__servef-mygo/crashed"

	// maxBody bounds a request body: past the server's 1 MiB walkthrough
	// limit, so the server still answers for what it refuses.
	maxBody = 2 << 20
	// KeepaliveEvery is how often an event stream gets a comment. The
	// webview tells nobody when a page stops reading a custom scheme; the
	// next write fails, which ends the upstream request (decision 0077).
	KeepaliveEvery = 5 * time.Second
)

var forwardedRequestHeaders = []string{
	"Accept", "Accept-Language", "Content-Type", "Range", "If-None-Match",
	"If-Modified-Since", "Last-Event-Id", "Cache-Control", "X-Servef-Language-Servers",
}

var hopByHop = map[string]bool{"Connection": true, "Keep-Alive": true, "Transfer-Encoding": true, "Upgrade": true}

// Daemon is where requests go: the ensured daemon's socket.
type Daemon interface {
	// Socket returns the socket, ensuring when needed; force retries now
	// after a recent failure.
	Socket(ctx context.Context, force bool) (string, error)
	// Reensure ensures again after the daemon stopped answering.
	Reensure(ctx context.Context) (string, error)
}

// Proxy is the http.Handler of the scheme.
type Proxy struct {
	daemon Daemon
	// keepalive is KeepaliveEvery, shorter in tests.
	keepalive time.Duration

	mu        sync.Mutex
	socket    string
	transport *http.Transport
}

// New returns a Proxy to daemon.
func New(daemon Daemon) *Proxy {
	p := &Proxy{daemon: daemon, keepalive: KeepaliveEvery}
	p.transport = &http.Transport{
		// The socket can change when the daemon restarts.
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", p.currentSocket())
		},
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
		// Bytes pass through as the daemon sent them.
		DisableCompression: true,
	}
	return p
}

// Close drops idle connections to the daemon.
func (p *Proxy) Close() { p.transport.CloseIdleConnections() }

func (p *Proxy) currentSocket() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.socket
}

func (p *Proxy) setSocket(socket string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.socket = socket
}

// ServeHTTP refuses what pages may not reach, then forwards to the daemon.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// WebKit omits Origin on same-origin requests, so the guard applies only
	// when present.
	if origin := r.Header.Get("Origin"); origin != "" && origin != Origin {
		writeError(w, http.StatusForbidden, "crossOrigin", "Cross-origin requests are refused.")
		return
	}
	if r.URL.Host != Host || r.URL.User != nil {
		writeError(w, http.StatusNotFound, "unknownConnection", "Unknown servef connection.")
		return
	}
	if control := controlAPI(r.URL.Path); control != "" {
		writeError(w, http.StatusForbidden, "forbidden", "The "+control+" control API is not reachable from the app.")
		return
	}
	name := path.Clean("/" + r.URL.Path)
	if name == CrashedPath {
		writePage(w, http.StatusOK, "This page crashed", "servef reloaded it once already, and it crashed again.")
		return
	}

	document := isDocument(r, name)
	var body []byte
	readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead
	if !readOnly && r.Body != nil {
		// mygo holds the body in memory already; WebKit sends no
		// Content-Length, so the length is known only here.
		data, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalidBody", "The request body could not be read.")
			return
		}
		if len(data) > maxBody {
			writeError(w, http.StatusRequestEntityTooLarge, "bodyTooLarge", "The request body is too large.")
			return
		}
		body = data
	}

	// A reload of a failure page retries at once.
	socket, err := p.daemon.Socket(r.Context(), document)
	if err != nil {
		p.fail(w, document, err)
		return
	}
	p.setSocket(socket)
	response, err := p.send(r, body)
	if err != nil && connect.IsGone(err) && r.Context().Err() == nil {
		// One re-ensure, and a retry for reads only (decision 0067).
		socket, ensureErr := p.daemon.Reensure(r.Context())
		if ensureErr != nil {
			p.fail(w, document, ensureErr)
			return
		}
		p.setSocket(socket)
		if !readOnly {
			writeError(w, http.StatusServiceUnavailable, "daemonRestarted", "The servef daemon restarted; the request was not sent. Try again.")
			return
		}
		response, err = p.send(r, nil)
	}
	if err != nil {
		p.fail(w, document, err)
		return
	}
	defer response.Body.Close()
	stream := strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream")
	for key, values := range response.Header {
		if !hopByHop[http.CanonicalHeaderKey(key)] {
			w.Header()[key] = values
		}
	}
	if r.Method == http.MethodHead || response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotModified {
		// No body follows; mygo tells the webview to expect this length.
		w.Header().Del("Content-Length")
	}
	w.WriteHeader(response.StatusCode)
	if stream {
		p.copyStream(w, r.Context(), response.Body)
		return
	}
	_, _ = io.Copy(w, response.Body)
}

// send makes one upstream request with the allowlisted headers, Host for the
// daemon's Host check, and the Sec-Fetch-Site that keeps its control API
// refusing whatever arrives through a proxy.
func (p *Proxy) send(r *http.Request, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	upstream, err := http.NewRequestWithContext(r.Context(), r.Method, "http://localhost"+r.URL.RequestURI(), reader)
	if err != nil {
		return nil, err
	}
	for _, name := range forwardedRequestHeaders {
		if values, ok := r.Header[name]; ok {
			upstream.Header[name] = values
		}
	}
	upstream.Header.Set("Sec-Fetch-Site", "same-origin")
	upstream.Host = "localhost"
	return p.transport.RoundTrip(upstream)
}

// controlAPI names the control API a decoded path reaches, resolving dot
// segments one at a time: a path that enters /_daemon and climbs out of it
// (/_daemon/../status, sent as /_daemon/..%2fstatus) still names it, whatever
// the daemon makes of the rest.
func controlAPI(decoded string) string {
	var stack []string
	for _, segment := range strings.Split(decoded, "/") {
		switch segment {
		case "", ".":
		case "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, segment)
			if strings.HasPrefix(stack[0], "_daemon") {
				return "daemon"
			}
			if strings.HasPrefix(stack[0], "_servef") {
				return "servef"
			}
		}
	}
	return ""
}

// copyStream writes an event stream as it arrives, with a comment on a
// keepalive tick between events: inside one, a comment line would be part of
// it. Once the page stops reading, a write fails, mygo cancels the request's
// context, and the upstream request ends.
func (p *Proxy) copyStream(w http.ResponseWriter, ctx context.Context, body io.Reader) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, body)
		return
	}
	flusher.Flush()
	chunks := make(chan []byte)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(chunks)
		buffer := make([]byte, 32<<10)
		for {
			n, err := body.Read(buffer)
			if n > 0 {
				chunk := append([]byte(nil), buffer[:n]...)
				select {
				case chunks <- chunk:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(p.keepalive)
	defer ticker.Stop()
	// tail is the end of what was sent; a blank line ends an event.
	tail := []byte("\n\n")
	for {
		var data []byte
		select {
		case <-ctx.Done():
			return
		case chunk, open := <-chunks:
			if !open {
				return
			}
			data = chunk
			tail = append(tail, chunk...)
			tail = tail[len(tail)-2:]
		case <-ticker.C:
			if !bytes.Equal(tail, []byte("\n\n")) {
				continue
			}
			data = []byte(": keepalive\n\n")
		}
		if _, err := w.Write(data); err != nil {
			return
		}
		flusher.Flush()
	}
}

func (p *Proxy) fail(w http.ResponseWriter, document bool, err error) {
	if !document {
		writeError(w, http.StatusServiceUnavailable, "daemonUnavailable", err.Error())
		return
	}
	writePage(w, http.StatusServiceUnavailable, "servef can't reach its daemon", err.Error())
}

// isDocument reports a page load, which gets a page rather than JSON.
func isDocument(r *http.Request, name string) bool {
	if strings.HasPrefix(name, "/api/") {
		return false
	}
	return r.Header.Get("Sec-Fetch-Dest") == "document" || strings.Contains(r.Header.Get("Accept"), "text/html")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(contract.ErrorResponse{Error: message, Code: code})
}

// writePage answers with a page of the app's own. Its link reloads, which
// ensures the daemon again.
func writePage(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>servef</title>
<style>body{font:15px/1.5 system-ui,sans-serif;margin:3em;color-scheme:light dark}</style>
<h1>%s</h1><p>%s</p><p><a href="/">Try again</a></p>
`, html.EscapeString(title), html.EscapeString(detail))
}

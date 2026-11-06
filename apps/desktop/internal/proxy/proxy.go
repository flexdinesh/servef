// Package proxy serves the app's custom scheme (decisions 0067, 0094, 0098):
// srcz-app://local from the daemon's Unix socket, srcz-app://<id> from a
// saved remote server. Pages reach /api and the web app, never the control
// endpoints or a credential, and servers see what their checks expect.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/flexdinesh/srcz/pkg/contract"
)

const (
	// Scheme is the custom scheme pages load from; Host the local connection.
	Scheme = "srcz-app"
	Host   = "local"
	// Origin is the local connection's origin.
	Origin = Scheme + "://" + Host
	// CrashedPath is the page shown when the web process keeps crashing. It
	// is the proxy's own, so it loads whatever the page or the daemon does.
	CrashedPath = "/__srcz-desktop/crashed"

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
	"If-Modified-Since", "Last-Event-Id", "Cache-Control", "X-Srcz-Language-Servers",
}

var hopByHop = map[string]bool{"Connection": true, "Keep-Alive": true, "Transfer-Encoding": true, "Upgrade": true}

// OriginOf is a connection's origin.
func OriginOf(host string) string { return Scheme + "://" + host }

// Proxy is the http.Handler of the scheme: each host is one connection, and
// its own origin, so what a page stores and reaches stays with its server.
type Proxy struct {
	resolver Resolver
	// keepalive is KeepaliveEvery, shorter in tests.
	keepalive time.Duration
}

// New returns a Proxy to resolver's connections.
func New(resolver Resolver) *Proxy {
	return &Proxy{resolver: resolver, keepalive: KeepaliveEvery}
}

// Close drops idle connections to every server.
func (p *Proxy) Close() { p.resolver.Close() }

// ServeHTTP refuses what pages may not reach, then forwards to the connection.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// WebKit omits Origin on same-origin requests, so the guard applies only
	// when present.
	if origin := r.Header.Get("Origin"); origin != "" && origin != OriginOf(r.URL.Host) {
		writeError(w, http.StatusForbidden, "crossOrigin", "Cross-origin requests are refused.")
		return
	}
	if r.URL.User != nil {
		writeError(w, http.StatusNotFound, "unknownConnection", "Unknown srcz connection.")
		return
	}
	if control := controlAPI(r.URL.Path); control != "" {
		writeError(w, http.StatusForbidden, "forbidden", "The "+control+" control API is not reachable from the app.")
		return
	}
	name := path.Clean("/" + r.URL.Path)
	if name == CrashedPath {
		writePage(w, http.StatusOK, "This page crashed", "srcz reloaded it once already, and it crashed again.")
		return
	}

	document := isDocument(r, name)
	upstream, failure := p.resolver.Resolve(r.URL.Host, document)
	if failure != nil {
		fail(w, document, failure)
		return
	}
	var body []byte
	if !readOnly(r) && r.Body != nil {
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

	response, err := upstream.Send(r, body, document)
	if err != nil {
		var failure *Failure
		if !errors.As(err, &failure) {
			failure = &Failure{Status: http.StatusBadGateway, Code: "upstreamFailed", Title: "srcz can't reach this server", Detail: err.Error()}
		}
		fail(w, document, failure)
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
			if strings.HasPrefix(stack[0], "_srcz") {
				return "srcz"
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

func fail(w http.ResponseWriter, document bool, failure *Failure) {
	if !document {
		writeError(w, failure.Status, failure.Code, failure.message())
		return
	}
	writePage(w, failure.Status, failure.Title, failure.Detail)
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
	_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>srcz</title>
<style>body{font:15px/1.5 system-ui,sans-serif;margin:3em;color-scheme:light dark}</style>
<h1>%s</h1><p>%s</p><p><a href="/">Try again</a></p>
`, html.EscapeString(title), html.EscapeString(detail))
}

package connection

import (
	"bufio"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// relay forwards TCP to target until stall, then keeps its connections open and
// passes nothing: a network that went away without closing anything.
type relay struct {
	listener net.Listener
	target   string
	stalled  atomic.Bool
}

func startRelay(t *testing.T, target string) *relay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	r := &relay{listener: listener, target: target}
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", target)
			if err != nil {
				_ = client.Close()
				continue
			}
			t.Cleanup(func() {
				_ = client.Close()
				_ = server.Close()
			})
			go r.pump(client, server)
			go r.pump(server, client)
		}
	}()
	return r
}

func (r *relay) pump(to, from net.Conn) {
	buffer := make([]byte, 32<<10)
	for {
		n, err := from.Read(buffer)
		if err != nil || r.stalled.Load() {
			return
		}
		if _, err := to.Write(buffer[:n]); err != nil {
			return
		}
	}
}

// An event stream over a connection that died silently fails, so its reader can
// reconnect; without the health check it would wait forever.
func TestStreamFailsWhenTheConnectionDiesSilently(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ": servef\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	certificate, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	link := startRelay(t, server.Listener.Addr().String())
	address, ok := link.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("relay address = %v", link.listener.Addr())
	}
	endpoint := TLS{Hosts: []string{"127.0.0.1"}, Port: address.Port, Credential: Credential{Fingerprint: FingerprintOf(certificate.RawSubjectPublicKeyInfo)}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.BaseURL()+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := endpoint.roundTripper(200*time.Millisecond, 200*time.Millisecond).RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.ProtoMajor != 2 {
		t.Fatalf("protocol = %s, want HTTP/2", response.Proto)
	}
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, ": servef") {
		t.Fatalf("first line = %q, %v", line, err)
	}

	link.stalled.Store(true)
	_, err = io.Copy(io.Discard, reader)
	if err == nil {
		t.Fatal("the stream ended cleanly, want a failure")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the stream stayed silent until the test's deadline: %v", err)
	}
}

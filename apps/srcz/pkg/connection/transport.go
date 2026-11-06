package connection

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/flexdinesh/srcz/pkg/contract"
)

// dialTimeout bounds connecting to one address.
const dialTimeout = 2 * time.Second

// ProbeTimeout bounds checking all of a connection's addresses.
const ProbeTimeout = 2 * time.Second

// A silent connection is pinged this often, and is dead when no answer comes
// this soon after. These match the server's (internal/runtime).
const (
	healthPingEvery   = 30 * time.Second
	healthPingTimeout = 15 * time.Second
)

// What a probe can find. Each needs a different fix, so each has its message.
var (
	// ErrUnreachable: no address answered.
	ErrUnreachable = errors.New("no address of this srcz server answered")
	// ErrFingerprint: something answered with another certificate.
	ErrFingerprint = errors.New("the server's certificate does not match this connection string")
	// ErrToken: the right server refused the token.
	ErrToken = errors.New("the server refused this connection string's token")
)

// Endpoint is a way to reach a srcz server. A later SSH endpoint adds a
// forward; everything above RoundTripper stays the same.
type Endpoint interface {
	// BaseURL is what requests are addressed to. Its host is always
	// "localhost", which the web app's Host check accepts; where the bytes go
	// is the round tripper's business.
	BaseURL() string
	// RoundTripper sends requests to the endpoint, adding its credentials. It
	// uses no proxy.
	RoundTripper() http.RoundTripper
}

// Unix is a server on a Unix socket: the local daemon. The socket's
// permissions are its authentication.
type Unix struct{ Socket string }

// BaseURL implements Endpoint.
func (Unix) BaseURL() string { return "http://localhost" }

// RoundTripper implements Endpoint.
func (u Unix) RoundTripper() http.RoundTripper {
	dialer := &net.Dialer{Timeout: dialTimeout}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", u.Socket)
		},
	}
}

// TLS is a remote server on TCP with a self-signed, pinned certificate: any
// route to it works, through a VPN, NAT, or ssh -L, since nothing depends on
// the address. Hosts are candidates, tried in order.
type TLS struct {
	Hosts      []string
	Port       int
	Credential Credential
}

// BaseURL implements Endpoint.
func (TLS) BaseURL() string { return "https://localhost" }

// RoundTripper implements Endpoint. It gzip-decodes transparently: remote
// answers are compressed on the wire and no caller sees that.
func (t TLS) RoundTripper() http.RoundTripper {
	return t.roundTripper(healthPingEvery, healthPingTimeout)
}

func (t TLS) roundTripper(pingEvery, pingTimeout time.Duration) http.RoundTripper {
	dial := &hostDialer{hosts: t.Hosts, port: t.Port}
	return &authorized{
		token: t.Credential.TokenText(),
		next: &http.Transport{
			Proxy:             nil,
			DialContext:       dial.dial,
			TLSClientConfig:   pinnedConfig(t.Credential.Fingerprint),
			ForceAttemptHTTP2: true,
			// A connection lost without a close, as when a network drops, must
			// fail its open event streams rather than leave them silent.
			HTTP2: &http.HTTP2Config{SendPingTimeout: pingEvery, PingTimeout: pingTimeout},
			// A dead host must not hold requests for the default 10 s.
			TLSHandshakeTimeout: 5 * time.Second,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// pinnedConfig trusts exactly the certificate whose public key has fingerprint:
// no CA or host name is involved.
func pinnedConfig(fingerprint [FingerprintSize]byte) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // VerifyConnection does the pinning.
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return ErrFingerprint
			}
			got := FingerprintOf(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
			if subtle.ConstantTimeCompare(got[:], fingerprint[:]) != 1 {
				return ErrFingerprint
			}
			return nil
		},
	}
}

// authorized adds the bearer token to each request.
type authorized struct {
	token string
	next  *http.Transport
}

// RoundTrip implements http.RoundTripper.
func (a *authorized) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+a.token)
	return a.next.RoundTrip(request)
}

// CloseIdleConnections lets http.Client and callers drop idle connections.
func (a *authorized) CloseIdleConnections() { a.next.CloseIdleConnections() }

// hostDialer connects to the first candidate that accepts, starting from the
// one that last did.
type hostDialer struct {
	hosts []string
	port  int

	mu   sync.Mutex
	last int
}

func (d *hostDialer) dial(ctx context.Context, network, _ string) (net.Conn, error) {
	d.mu.Lock()
	first := d.last
	d.mu.Unlock()
	if len(d.hosts) == 0 {
		return nil, ErrUnreachable
	}
	dialer := &net.Dialer{Timeout: dialTimeout}
	var failure error
	for offset := range d.hosts {
		index := (first + offset) % len(d.hosts)
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(d.hosts[index], strconv.Itoa(d.port)))
		if err == nil {
			d.mu.Lock()
			d.last = index
			d.mu.Unlock()
			return conn, nil
		}
		failure = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, failure
}

// Probe checks each candidate at once and returns the endpoint with the first
// that answers as this server moved to the front, and the server's description.
// When none does, the most specific finding wins: another certificate or a
// refused token says more than silence.
func Probe(ctx context.Context, endpoint TLS) (TLS, contract.RemoteInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	type answer struct {
		host string
		info contract.RemoteInfo
		err  error
	}
	answers := make(chan answer, len(endpoint.Hosts))
	for _, host := range endpoint.Hosts {
		go func() {
			info, err := probeHost(ctx, TLS{Hosts: []string{host}, Port: endpoint.Port, Credential: endpoint.Credential})
			answers <- answer{host, info, err}
		}()
	}
	failure := ErrUnreachable
	for range endpoint.Hosts {
		found := <-answers
		if found.err == nil {
			return moveFirst(endpoint, found.host), found.info, nil
		}
		if errors.Is(found.err, ErrFingerprint) || (errors.Is(found.err, ErrToken) && !errors.Is(failure, ErrFingerprint)) {
			failure = found.err
		}
	}
	return endpoint, contract.RemoteInfo{}, failure
}

func moveFirst(endpoint TLS, host string) TLS {
	hosts := make([]string, 0, len(endpoint.Hosts))
	hosts = append(hosts, host)
	for _, candidate := range endpoint.Hosts {
		if candidate != host {
			hosts = append(hosts, candidate)
		}
	}
	endpoint.Hosts = hosts
	return endpoint
}

func probeHost(ctx context.Context, endpoint TLS) (contract.RemoteInfo, error) {
	transport := endpoint.RoundTripper()
	if closer, ok := transport.(interface{ CloseIdleConnections() }); ok {
		defer closer.CloseIdleConnections()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.BaseURL()+contract.RemoteInfoPath, nil)
	if err != nil {
		return contract.RemoteInfo{}, err
	}
	response, err := transport.RoundTrip(request)
	if errors.Is(err, ErrFingerprint) {
		return contract.RemoteInfo{}, ErrFingerprint
	}
	if err != nil {
		return contract.RemoteInfo{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized {
		return contract.RemoteInfo{}, ErrToken
	}
	var info contract.RemoteInfo
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&info) != nil || info.Protocol <= 0 {
		return contract.RemoteInfo{}, fmt.Errorf("%w: it does not answer as a srcz server", ErrUnreachable)
	}
	return info, nil
}

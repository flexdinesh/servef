// Package control coordinates graceful shutdown between servef processes.
package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const endpointPath = "/_servef/control"

// Registry stores private control records for running servef processes.
type Registry struct {
	directory string
}

// Instance is a registered servef process.
type Instance struct {
	record record
	path   string
	stop   chan struct{}
	once   sync.Once
}

// Remote is a registered servef process controlled over HTTP.
type Remote struct {
	record record
}

type record struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// DefaultDirectory returns the per-user directory for control records.
func DefaultDirectory() (string, error) {
	directory, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache: %w", err)
	}
	return filepath.Join(directory, "servef", "servers"), nil
}

// NewRegistry constructs a registry rooted at directory.
func NewRegistry(directory string) Registry {
	return Registry{directory: directory}
}

// Register creates a private control record for a running server.
func (r Registry) Register(host string, port int) (*Instance, error) {
	if err := os.MkdirAll(r.directory, 0o700); err != nil {
		return nil, fmt.Errorf("create control directory: %w", err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("create control token: %w", err)
	}
	entry := record{Host: host, Port: port, Token: hex.EncodeToString(tokenBytes)}
	path := r.recordPath(host, port)
	contents, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("encode control record: %w", err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return nil, fmt.Errorf("write control record: %w", err)
	}
	return &Instance{record: entry, path: path, stop: make(chan struct{})}, nil
}

// Handler wraps next with authenticated status and shutdown handling.
func (i *Instance) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != endpointPath {
			next.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Servef-Token")), []byte(i.record.Token)) != 1 {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			i.once.Do(func() { close(i.stop) })
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// StopRequested reports authenticated shutdown requests.
func (i *Instance) StopRequested() <-chan struct{} {
	return i.stop
}

// Close removes this instance's control record.
func (i *Instance) Close() error {
	contents, err := os.ReadFile(i.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read control record: %w", err)
	}
	var current record
	if err := json.Unmarshal(contents, &current); err != nil || current.Token != i.record.Token {
		return nil
	}
	if err := os.Remove(i.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove control record: %w", err)
	}
	return nil
}

// Lookup returns the registered process for host and port.
func (r Registry) Lookup(host string, port int) (Remote, error) {
	contents, err := os.ReadFile(r.recordPath(host, port))
	if err != nil {
		return Remote{}, err
	}
	var entry record
	if err := json.Unmarshal(contents, &entry); err != nil {
		return Remote{}, fmt.Errorf("decode control record: %w", err)
	}
	if entry.Host != host || entry.Port != port || entry.Token == "" {
		return Remote{}, errors.New("invalid control record")
	}
	return Remote{record: entry}, nil
}

// List returns every registered process's record; malformed records are
// skipped. A missing directory lists none. Callers verify with Running.
func (r Registry) List() ([]Remote, error) {
	names, err := filepath.Glob(filepath.Join(r.directory, "*.json"))
	if err != nil {
		return nil, err
	}
	remotes := make([]Remote, 0, len(names))
	for _, name := range names {
		contents, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		var entry record
		if err := json.Unmarshal(contents, &entry); err != nil || entry.Host == "" || entry.Port <= 0 || entry.Token == "" {
			continue
		}
		remotes = append(remotes, Remote{record: entry})
	}
	sort.Slice(remotes, func(a, b int) bool {
		left, right := remotes[a].record, remotes[b].record
		if left.Port != right.Port {
			return left.Port < right.Port
		}
		return left.Host < right.Host
	})
	return remotes, nil
}

// Host is the host the process registered (its --host).
func (r Remote) Host() string {
	return r.record.Host
}

// Port is the port the process listens on.
func (r Remote) Port() int {
	return r.record.Port
}

// Running reports whether the registered process responds as servef.
func (r Remote) Running(ctx context.Context) bool {
	return r.request(ctx, http.MethodGet) == nil
}

// Stop asks the registered process to shut down gracefully.
func (r Remote) Stop(ctx context.Context) error {
	if err := r.request(ctx, http.MethodPost); err != nil {
		return fmt.Errorf("stop servef on port %d: %w", r.record.Port, err)
	}
	return nil
}

func (r Remote) request(ctx context.Context, method string) error {
	req, err := http.NewRequestWithContext(ctx, method, r.url(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Servef-Token", r.record.Token)
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy:       nil,
			DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	want := http.StatusNoContent
	if method == http.MethodPost {
		want = http.StatusAccepted
	}
	if response.StatusCode != want {
		return fmt.Errorf("unexpected control response %s", response.Status)
	}
	return nil
}

// Origin is the server's HTTP origin, with unspecified hosts dialed on
// loopback, as the control endpoint is.
func (r Remote) Origin() string {
	host := r.record.Host
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
		if ip.To4() == nil {
			host = "::1"
		}
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(r.record.Port))
}

func (r Remote) url() string {
	return r.Origin() + endpointPath
}

func (r Registry) recordPath(host string, port int) string {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	sum := sha256.Sum256([]byte(address))
	return filepath.Join(r.directory, hex.EncodeToString(sum[:])+".json")
}

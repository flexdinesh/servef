//go:build darwin || linux

package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testDaemon struct {
	socket    string
	rootsFile string
	client    *Client
	cancel    context.CancelFunc
	done      chan error
	log       *bytes.Buffer
}

// daemonConfig returns a config with a short socket path and private
// config and cache directories.
func daemonConfig(t *testing.T) Config {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	return Config{Socket: filepath.Join(socketDir(t), "daemon.sock"), RootsFile: filepath.Join(config, "servef", "roots.json"), Depth: 5, ScanDepth: 3}
}

func startDaemon(t *testing.T, config Config) *testDaemon {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var log bytes.Buffer
	config.Log = &log
	d := &testDaemon{socket: config.Socket, rootsFile: config.RootsFile, client: NewClient(config.Socket), cancel: cancel, done: make(chan error, 1), log: &log}
	go func() { d.done <- Run(ctx, config) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-d.done:
		case <-time.After(10 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := d.client.Status(context.Background()); err == nil {
			return d
		}
		select {
		case err := <-d.done:
			t.Fatalf("Run() = %v before serving", err)
		case <-time.After(10 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not answer")
		}
	}
}

func (d *testDaemon) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-d.done:
		d.done <- err
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}

func (d *testDaemon) request(t *testing.T, method, target string, header http.Header, body string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, "http://localhost"+target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for name, values := range header {
		request.Header[name] = values
	}
	if host := header.Get("Host"); host != "" {
		request.Host = host
	}
	response, err := d.client.http.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, content
}

func canonicalDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

var jsonHeader = http.Header{"Content-Type": {"application/json"}}

func TestRunServesStatusRootsAndWebRoutes(t *testing.T) {
	config := daemonConfig(t)
	config.IdleExit = 2 * time.Minute
	parent := t.TempDir()
	work, personal := canonicalDir(t, parent, "work"), canonicalDir(t, parent, "personal")
	missing := filepath.Join(parent, "unmounted")
	if err := SaveRoots(config.RootsFile, []string{work, missing, personal}); err != nil {
		t.Fatal(err)
	}
	d := startDaemon(t, config)

	status, err := d.client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Protocol != ProtocolVersion || status.PID != os.Getpid() || status.Socket != config.Socket || status.Depth != 5 || status.ScanDepth != 3 ||
		status.IdleExit != "2m0s" || !reflect.DeepEqual(status.Roots, []string{work, personal}) || status.StartedAt.IsZero() {
		t.Fatalf("status = %+v", status)
	}

	code, body := d.request(t, http.MethodGet, "/_daemon/roots", nil, "")
	var roots rootsResponse
	if err := json.Unmarshal(body, &roots); err != nil || code != http.StatusOK {
		t.Fatalf("GET /_daemon/roots = %d %s", code, body)
	}
	if !reflect.DeepEqual(roots, rootsResponse{Roots: []string{work, personal}, Skipped: []string{missing}}) {
		t.Fatalf("roots = %+v", roots)
	}
	if !strings.Contains(d.log.String(), "skipping root "+missing) {
		t.Fatalf("log = %q, want skipped root", d.log.String())
	}

	code, body = d.request(t, http.MethodGet, "/api/projects", nil, "")
	var projects struct {
		SessionKey string `json:"sessionKey"`
		Projects   []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(body, &projects); err != nil || code != http.StatusOK {
		t.Fatalf("GET /api/projects = %d %s", code, body)
	}
	if projects.SessionKey != "daemon" {
		t.Fatalf("sessionKey = %q, want daemon", projects.SessionKey)
	}
	var ids []string
	for _, project := range projects.Projects {
		ids = append(ids, project.ID)
	}
	if !reflect.DeepEqual(ids, []string{work, personal}) {
		t.Fatalf("projects = %v", ids)
	}
	if code, _ := d.request(t, http.MethodGet, "/api/projects", http.Header{"Host": {"evil.example"}}, ""); code != http.StatusForbidden {
		t.Fatalf("foreign Host on web route = %d, want 403", code)
	}
}

func TestRunStartsWithNoRoots(t *testing.T) {
	d := startDaemon(t, daemonConfig(t))
	roots, err := d.client.Roots(context.Background())
	if err != nil || len(roots) != 0 {
		t.Fatalf("Roots() = %v, %v; want none", roots, err)
	}
	if code, body := d.request(t, http.MethodGet, "/api/projects", nil, ""); code != http.StatusOK {
		t.Fatalf("GET /api/projects = %d %s", code, body)
	}
}

func TestRunAddsAndPersistsRoots(t *testing.T) {
	config := daemonConfig(t)
	parent := t.TempDir()
	work, personal := canonicalDir(t, parent, "work"), canonicalDir(t, parent, "personal")
	missing := filepath.Join(parent, "unmounted")
	if err := SaveRoots(config.RootsFile, []string{missing}); err != nil {
		t.Fatal(err)
	}
	d := startDaemon(t, config)

	root, added, err := d.client.AddRoot(context.Background(), work)
	if err != nil || root != work || !added {
		t.Fatalf("AddRoot(work) = %q, %t, %v", root, added, err)
	}
	if code, body := d.request(t, http.MethodPost, "/_daemon/roots", jsonHeader, `{"path":"`+personal+`"}`); code != http.StatusCreated {
		t.Fatalf("POST personal = %d %s, want 201", code, body)
	}
	if code, body := d.request(t, http.MethodPost, "/_daemon/roots", jsonHeader, `{"path":"`+work+`/"}`); code != http.StatusOK || !strings.Contains(string(body), `"added":false`) {
		t.Fatalf("POST duplicate = %d %s, want 200 added false", code, body)
	}
	for name, body := range map[string]string{
		"overlap":  `{"path":"` + parent + `"}`,
		"relative": `{"path":"work"}`,
		"missing":  `{"path":"` + filepath.Join(parent, "nope") + `"}`,
		"unknown":  `{"root":"` + work + `"}`,
	} {
		if code, response := d.request(t, http.MethodPost, "/_daemon/roots", jsonHeader, body); code != http.StatusBadRequest {
			t.Errorf("POST %s = %d %s, want 400", name, code, response)
		}
	}
	if code, body := d.request(t, http.MethodPost, "/_daemon/roots", http.Header{"Content-Type": {"text/plain"}}, `{"path":"`+work+`"}`); code != http.StatusUnsupportedMediaType {
		t.Fatalf("POST text/plain = %d %s, want 415", code, body)
	}

	saved, err := LoadRoots(config.RootsFile)
	if err != nil || !reflect.DeepEqual(saved, []string{work, personal, missing}) {
		t.Fatalf("saved roots = %v, %v; want served roots then the skipped one", saved, err)
	}

	if err := d.client.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.wait(t); err != nil {
		t.Fatal(err)
	}
	restarted := startDaemon(t, config)
	roots, err := restarted.client.Roots(context.Background())
	if err != nil || !reflect.DeepEqual(roots, []string{work, personal}) {
		t.Fatalf("roots after restart = %v, %v", roots, err)
	}
}

func TestRunRefusesBrowserRequestsToControlAPI(t *testing.T) {
	d := startDaemon(t, daemonConfig(t))
	for _, header := range []http.Header{{"Origin": {"http://localhost"}}, {"Sec-Fetch-Site": {"same-origin"}}} {
		if code, body := d.request(t, http.MethodPost, "/_daemon/stop", header, ""); code != http.StatusForbidden {
			t.Fatalf("POST /_daemon/stop with %v = %d %s, want 403", header, code, body)
		}
	}
	if _, err := d.client.Status(context.Background()); err != nil {
		t.Fatalf("daemon stopped by a browser request: %v", err)
	}
	if code, _ := d.request(t, http.MethodGet, "/_daemon/missing", nil, ""); code != http.StatusNotFound {
		t.Fatalf("unknown control endpoint = %d, want 404", code)
	}
	if code, _ := d.request(t, http.MethodGet, "/_daemon/stop", nil, ""); code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /_daemon/stop = %d, want 405", code)
	}
}

func TestStopEndsRunAndRemovesSocket(t *testing.T) {
	d := startDaemon(t, daemonConfig(t))
	if err := d.client.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.wait(t); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if _, err := os.Lstat(d.socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after stop: %v, want removed", err)
	}
	if _, err := d.client.Status(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Status() after stop = %v, want ErrNotRunning", err)
	}
	if !strings.Contains(d.log.String(), "stopped: stop requested") {
		t.Fatalf("log = %q", d.log.String())
	}
	if err := d.client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() when not running = %v, want nil", err)
	}
}

func TestRunExitsWhenIdle(t *testing.T) {
	config := daemonConfig(t)
	config.IdleExit = 200 * time.Millisecond
	d := startDaemon(t, config)
	if err := d.wait(t); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if !strings.Contains(d.log.String(), "stopped: idle") {
		t.Fatalf("log = %q", d.log.String())
	}
}

func TestRunRefusesMalformedRootsFileBeforeListening(t *testing.T) {
	config := daemonConfig(t)
	if err := os.MkdirAll(filepath.Dir(config.RootsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.RootsFile, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), config); err == nil || !strings.Contains(err.Error(), config.RootsFile) {
		t.Fatalf("Run() = %v, want error naming the roots file", err)
	}
	if _, err := os.Lstat(config.Socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket = %v, want never created", err)
	}
	if content, _ := os.ReadFile(config.RootsFile); string(content) != "{" {
		t.Fatalf("roots file rewritten: %q", content)
	}
}

func TestRunIsSingleInstance(t *testing.T) {
	config := daemonConfig(t)
	startDaemon(t, config)
	if err := Run(context.Background(), config); !errors.Is(err, ErrRunning) {
		t.Fatalf("second Run() = %v, want ErrRunning", err)
	}
}

func TestClientStatusWithoutDaemon(t *testing.T) {
	client := NewClient(filepath.Join(socketDir(t), "daemon.sock"))
	if _, err := client.Status(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Status() = %v, want ErrNotRunning", err)
	}
}

func TestEnsureReturnsRunningDaemonWithoutStarting(t *testing.T) {
	d := startDaemon(t, daemonConfig(t))
	status, started, err := Ensure(context.Background(), d.socket, func(context.Context) (<-chan error, error) {
		t.Fatal("start called for a running daemon")
		return nil, nil
	})
	if err != nil || started || status.PID != os.Getpid() {
		t.Fatalf("Ensure() = %+v, %t, %v", status, started, err)
	}
}

func TestEnsureStartsDaemon(t *testing.T) {
	config := daemonConfig(t)
	runCtx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		<-runDone
	})
	var starts atomic.Int32
	start := func(context.Context) (<-chan error, error) {
		starts.Add(1)
		exited := make(chan error, 1)
		go func() {
			err := Run(runCtx, config)
			runDone <- err
			exited <- err
		}()
		return exited, nil
	}
	ctx, cancelEnsure := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelEnsure()
	status, started, err := Ensure(ctx, config.Socket, start)
	if err != nil || !started || status.Protocol != ProtocolVersion || starts.Load() != 1 {
		t.Fatalf("Ensure() = %+v, %t, %v after %d starts", status, started, err, starts.Load())
	}
}

func TestEnsureRetriesChildrenThatExitThenTimesOut(t *testing.T) {
	socket := filepath.Join(socketDir(t), "daemon.sock")
	var starts atomic.Int32
	start := func(context.Context) (<-chan error, error) {
		starts.Add(1)
		exited := make(chan error, 1)
		exited <- errors.New("exit status 1")
		return exited, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, started, err := Ensure(ctx, socket, start)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "exit status 1") || !started {
		t.Fatalf("Ensure() = %t, %v; want deadline error with the last exit", started, err)
	}
	if starts.Load() < 2 {
		t.Fatalf("start called %d times, want retries", starts.Load())
	}
}

func TestEnsureReportsProtocolMismatch(t *testing.T) {
	socket := filepath.Join(socketDir(t), "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, Status{Protocol: 999, Version: "9.9.9", PID: 1, Socket: socket})
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	status, started, err := Ensure(context.Background(), socket, func(context.Context) (<-chan error, error) {
		t.Fatal("start called for a running daemon")
		return nil, nil
	})
	var mismatch *ProtocolMismatchError
	if !errors.As(err, &mismatch) || mismatch.Daemon != 999 || mismatch.Client != ProtocolVersion || status.Protocol != 999 || started {
		t.Fatalf("Ensure() = %+v, %t, %v; want ProtocolMismatchError", status, started, err)
	}
	if !strings.Contains(err.Error(), "servef daemon stop") {
		t.Fatalf("mismatch error = %q, want stop instructions", err)
	}
}

func TestClientRejectsNonDaemonAnswers(t *testing.T) {
	socket := filepath.Join(socketDir(t), "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.NotFoundHandler()}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	if _, err := NewClient(socket).Status(context.Background()); err == nil || errors.Is(err, ErrNotRunning) || !strings.Contains(err.Error(), socket) {
		t.Fatalf("Status() = %v, want error naming the socket", err)
	}
}

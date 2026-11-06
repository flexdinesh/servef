//go:build darwin || linux

package daemon

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/srcz/pkg/connection"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// fakeTimers runs timers by hand.
type fakeTimers struct{ timers []*fakeTimer }

type fakeTimer struct {
	d       time.Duration
	fire    func()
	stopped bool
}

func (f *fakeTimers) after(d time.Duration, fire func()) func() bool {
	timer := &fakeTimer{d: d, fire: fire}
	f.timers = append(f.timers, timer)
	return func() bool {
		timer.stopped = true
		return true
	}
}

// armed is the one running timer's duration, or 0.
func (f *fakeTimers) armed(t *testing.T) time.Duration {
	t.Helper()
	var running []*fakeTimer
	for _, timer := range f.timers {
		if !timer.stopped {
			running = append(running, timer)
		}
	}
	if len(running) > 1 {
		t.Fatalf("%d timers running", len(running))
	}
	if len(running) == 0 {
		return 0
	}
	return running[0].d
}

func (f *fakeTimers) fireArmed() {
	for _, timer := range f.timers {
		if !timer.stopped {
			timer.stopped = true
			timer.fire()
		}
	}
}

func newLifetime() (*uiLifetime, *fakeTimers, *int) {
	timers := &fakeTimers{}
	fired := 0
	return &uiLifetime{after: timers.after, fire: func() { fired++ }}, timers, &fired
}

func TestLifetimeWaitsForTheFirstUI(t *testing.T) {
	lifetime, timers, fired := newLifetime()
	lifetime.start(10*time.Minute, 5*time.Minute)
	if got := timers.armed(t); got != 10*time.Minute {
		t.Fatalf("armed %s, want the first wait", got)
	}
	timers.fireArmed()
	if *fired != 1 {
		t.Errorf("fired %d times", *fired)
	}
}

func TestLifetimeGivesGraceAfterTheLastUI(t *testing.T) {
	lifetime, timers, fired := newLifetime()
	lifetime.start(10*time.Minute, 5*time.Minute)
	lifetime.join()
	lifetime.join()
	if got := timers.armed(t); got != 0 {
		t.Fatalf("armed %s with UIs connected", got)
	}
	lifetime.leave()
	if got := timers.armed(t); got != 0 {
		t.Fatalf("armed %s with one UI left", got)
	}
	lifetime.leave()
	if got := timers.armed(t); got != 5*time.Minute {
		t.Fatalf("armed %s, want the grace", got)
	}
	// A reload comes back in time.
	lifetime.join()
	if got := timers.armed(t); got != 0 {
		t.Fatalf("armed %s after the UI came back", got)
	}
	lifetime.leave()
	timers.fireArmed()
	if *fired != 1 {
		t.Errorf("fired %d times", *fired)
	}
}

func TestLifetimeRestartAndStop(t *testing.T) {
	lifetime, timers, fired := newLifetime()
	lifetime.start(10*time.Minute, 5*time.Minute)
	first := timers.timers[0]
	lifetime.start(10*time.Minute, 5*time.Minute)
	if !first.stopped {
		t.Error("a restart left the old wait running")
	}
	// A timer that fired while being replaced does nothing.
	first.fire()
	if *fired != 0 {
		t.Errorf("a replaced timer fired")
	}
	lifetime.stop()
	if got := timers.armed(t); got != 0 {
		t.Errorf("armed %s after stop", got)
	}
	lifetime.join()
	lifetime.leave()
	if got := timers.armed(t); got != 0 {
		t.Errorf("armed %s: a UI leaving a stopped listener restarted the wait", got)
	}
}

// A restart with a UI here must not start the first wait under it.
func TestLifetimeRestartWithAUIKeepsWaiting(t *testing.T) {
	lifetime, timers, _ := newLifetime()
	lifetime.start(time.Minute, time.Minute)
	lifetime.join()
	lifetime.start(time.Minute, time.Minute)
	if got := timers.armed(t); got != 0 {
		t.Errorf("armed %s with a UI connected", got)
	}
}

// remoteDaemon runs a real daemon with a project and an identity file.
func remoteDaemon(t *testing.T, idleExit time.Duration) (*testDaemon, string) {
	t.Helper()
	config := daemonConfig(t)
	config.IdleExit = idleExit
	config.IdentityFile = filepath.Join(t.TempDir(), "remote", "identity.json")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Remote\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	daemon := startDaemon(t, config)
	if _, err := daemon.client.Open(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return daemon, root
}

func startRemote(t *testing.T, daemon *testDaemon, firstWait, grace string) (contract.DaemonRemote, connection.TLS) {
	t.Helper()
	port := 0
	status, err := daemon.client.RemoteStart(context.Background(), contract.DaemonRemoteRequest{Host: "127.0.0.1", Port: &port, FirstWait: firstWait, Grace: grace})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := connection.ParseCredential(status.Credential)
	if err != nil {
		t.Fatal(err)
	}
	return status, connection.TLS{Hosts: []string{"127.0.0.1"}, Port: status.Port, Credential: credential}
}

// holdPresence opens a page's presence stream until the returned function.
func holdPresence(t *testing.T, transport http.RoundTripper, base string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/presence", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("presence = %d", response.StatusCode)
	}
	if _, err := bufio.NewReader(response.Body).ReadString('\n'); err != nil {
		cancel()
		t.Fatal(err)
	}
	return func() {
		cancel()
		_ = response.Body.Close()
	}
}

func waitForClients(t *testing.T, daemon *testDaemon, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, err := daemon.client.RemoteStatus(context.Background())
		if err == nil && status.Clients == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("clients = %d, %v, want %d", status.Clients, err, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRemoteServesTheAppButNeverTheControlAPI(t *testing.T) {
	daemon, _ := remoteDaemon(t, 0)
	status, endpoint := startRemote(t, daemon, "", "")
	if !status.Listening || status.ID != endpoint.Credential.ID() {
		t.Fatalf("status = %+v", status)
	}
	client := &http.Client{Transport: endpoint.RoundTripper()}
	defer client.CloseIdleConnections()
	get := func(path string) int {
		t.Helper()
		response, err := client.Get(endpoint.BaseURL() + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	if got := get("/api/projects"); got != http.StatusOK {
		t.Errorf("/api/projects = %d", got)
	}
	for _, path := range []string{"/_daemon/status", "/_daemon/remote", "/_daemon/stop", "/_srcz/control"} {
		if got := get(path); got == http.StatusOK || got == http.StatusAccepted {
			t.Errorf("%s over TCP = %d, want it unreachable", path, got)
		}
	}
	// The daemon's socket still answers the control API.
	if _, err := daemon.client.Status(context.Background()); err != nil {
		t.Errorf("control over the socket: %v", err)
	}
}

func TestRemoteRefusesAnyoneWithoutTheToken(t *testing.T) {
	daemon, _ := remoteDaemon(t, 0)
	_, endpoint := startRemote(t, daemon, "", "")
	wrong := endpoint
	wrong.Credential.Token[0]++
	if _, _, err := connection.Probe(context.Background(), wrong); !errors.Is(err, connection.ErrToken) {
		t.Errorf("Probe(wrong token) = %v", err)
	}
	if _, info, err := connection.Probe(context.Background(), endpoint); err != nil || info.Protocol != ProtocolVersion || info.Name == "" {
		t.Errorf("Probe() = %+v, %v", info, err)
	}
}

func TestRemoteStartIsIdempotentAndCredentialsSurviveRestart(t *testing.T) {
	config := daemonConfig(t)
	config.IdentityFile = filepath.Join(t.TempDir(), "identity.json")
	ctx := context.Background()
	daemon := startDaemon(t, config)
	first, err := daemon.client.RemoteStart(ctx, contract.DaemonRemoteRequest{Host: "127.0.0.1"})
	if err != nil {
		t.Skipf("default ports unavailable here: %v", err)
	}
	again, err := daemon.client.RemoteStart(ctx, contract.DaemonRemoteRequest{Host: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Errorf("second start = %+v, want %+v", again, first)
	}
	if err := daemon.client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := daemon.wait(t); err != nil {
		t.Fatal(err)
	}
	restarted := startDaemon(t, config)
	after, err := restarted.client.RemoteStart(ctx, contract.DaemonRemoteRequest{Host: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Credential != first.Credential || after.Port != first.Port || after.ID != first.ID {
		t.Errorf("after a restart = %+v, want %+v's credential and port", after, first)
	}
}

func TestRemoteExitsAfterTheLastUIWhenNothingElseIsActive(t *testing.T) {
	daemon, _ := remoteDaemon(t, time.Hour)
	_, endpoint := startRemote(t, daemon, "1h", "100ms")
	transport := endpoint.RoundTripper()
	release := holdPresence(t, transport, endpoint.BaseURL())
	waitForClients(t, daemon, 1)
	// Well past the grace: an open page, even hidden, keeps it running.
	time.Sleep(400 * time.Millisecond)
	if status, err := daemon.client.RemoteStatus(context.Background()); err != nil || !status.Listening {
		t.Fatalf("status with a UI connected = %+v, %v", status, err)
	}
	release()
	if err := daemon.wait(t); err != nil {
		t.Errorf("Run() = %v", err)
	}
	if !strings.Contains(daemon.log.String(), "no UI connected") {
		t.Errorf("log = %s", daemon.log)
	}
}

func TestRemoteExitsWhenNothingEverConnects(t *testing.T) {
	daemon, _ := remoteDaemon(t, time.Hour)
	startRemote(t, daemon, "100ms", "1h")
	if err := daemon.wait(t); err != nil {
		t.Errorf("Run() = %v", err)
	}
}

func TestLocalUIHoldsTheDaemonAfterTheListenerCloses(t *testing.T) {
	daemon, _ := remoteDaemon(t, time.Hour)
	local := connection.Unix{Socket: daemon.socket}
	release := holdPresence(t, local.RoundTripper(), local.BaseURL())
	defer release()
	_, endpoint := startRemote(t, daemon, "100ms", "100ms")
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, err := daemon.client.RemoteStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !status.Listening {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the listener never closed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(endpoint.Port)), time.Second); err == nil {
		_ = conn.Close()
		t.Error("the port still accepts")
	}
	// Past the settling time, the daemon is still there for the local window.
	time.Sleep(remoteSettle + 200*time.Millisecond)
	if _, err := daemon.client.Status(context.Background()); err != nil {
		t.Errorf("the local window's daemon stopped: %v", err)
	}
}

func TestRemoteStopClosesTheListenerAndExits(t *testing.T) {
	daemon, _ := remoteDaemon(t, time.Hour)
	startRemote(t, daemon, "", "")
	if err := daemon.client.RemoteStop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := daemon.wait(t); err != nil {
		t.Errorf("Run() = %v", err)
	}
}

func TestIdleExitZeroMeansNeverEvenAfterRemote(t *testing.T) {
	daemon, _ := remoteDaemon(t, 0)
	startRemote(t, daemon, "", "")
	if err := daemon.client.RemoteStop(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(remoteSettle + 200*time.Millisecond)
	if _, err := daemon.client.Status(context.Background()); err != nil {
		t.Errorf("--idle-exit 0 daemon stopped: %v", err)
	}
}

func TestRemoteRotateKeepsTheAddressAndChangesTheSecret(t *testing.T) {
	daemon, _ := remoteDaemon(t, 0)
	before, old := startRemote(t, daemon, "", "")
	after, err := daemon.client.RemoteRotate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !after.Listening || after.Port != before.Port || after.Credential == before.Credential || after.ID == before.ID {
		t.Fatalf("rotated = %+v, before = %+v", after, before)
	}
	if _, _, err := connection.Probe(context.Background(), old); !errors.Is(err, connection.ErrFingerprint) {
		t.Errorf("Probe(old string) = %v, want ErrFingerprint", err)
	}
	credential, err := connection.ParseCredential(after.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := connection.Probe(context.Background(), connection.TLS{Hosts: old.Hosts, Port: old.Port, Credential: credential}); err != nil {
		t.Errorf("Probe(new string) = %v", err)
	}
}

func TestRemoteRejectsBadRequests(t *testing.T) {
	daemon, _ := remoteDaemon(t, 0)
	ctx := context.Background()
	bad := 70000
	for name, request := range map[string]contract.DaemonRemoteRequest{
		"host":      {Host: "not an ip"},
		"port":      {Port: &bad},
		"firstWait": {FirstWait: "soon"},
		"grace":     {Grace: "-1s"},
	} {
		var failure *ControlError
		if _, err := daemon.client.RemoteStart(ctx, request); !errors.As(err, &failure) || failure.Status != http.StatusBadRequest {
			t.Errorf("%s: %v", name, err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("not TCP")
	}
	var failure *ControlError
	if _, err := daemon.client.RemoteStart(ctx, contract.DaemonRemoteRequest{Host: "127.0.0.1", Port: &address.Port}); !errors.As(err, &failure) || failure.Code != contract.CodePortBusy {
		t.Errorf("busy port: %v", err)
	}
}

func TestDaemonWithoutAnIdentityRefusesRemote(t *testing.T) {
	daemon := startDaemon(t, daemonConfig(t))
	var failure *ControlError
	if _, err := daemon.client.RemoteStart(context.Background(), contract.DaemonRemoteRequest{}); !errors.As(err, &failure) || failure.Code != contract.CodeRemoteUnavailable {
		t.Errorf("RemoteStart() = %v", err)
	}
}

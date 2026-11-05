//go:build darwin || linux

package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/flexdinesh/servef/internal/desktopapp"
	"github.com/flexdinesh/servef/pkg/connection"
	"github.com/flexdinesh/servef/pkg/contract"
)

// realRemote runs a real daemon, and the deps `servef --remote` uses against it.
func realRemote(t *testing.T) (remoteDeps, string) {
	t.Helper()
	// Unix socket paths are short: t.TempDir() names can be too long.
	socketDir, err := os.MkdirTemp("", "servef-cmd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "d.sock")
	config := daemon.Config{
		Socket:       socket,
		RootsFile:    filepath.Join(t.TempDir(), "roots.json"),
		Depth:        5,
		ScanDepth:    10,
		IdentityFile: filepath.Join(t.TempDir(), "identity.json"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx, config) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	client := daemon.NewClient(socket)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := client.Status(context.Background()); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the daemon did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return remoteDeps{
		roots: daemonRootsDeps{
			socket: socket,
			ensure: func(ctx context.Context, opts daemonOptions) (daemon.Status, bool, error) {
				status, err := client.Status(ctx)
				return status, false, err
			},
			openRoot: openDaemonRoot,
		},
		start:      startDaemonRemote,
		served:     servedRoots,
		candidates: func() []string { return []string{"127.0.0.1"} },
		getenv:     func(string) string { return "" },
		terminal:   func(io.Writer) bool { return false },
	}, socket
}

func remoteOptions(root string) options {
	return options{paths: []string{root}, depth: 5, scanDepth: 10, host: defaultHost, port: 0, portSet: true, remoteFirstWait: 10 * time.Minute, remoteGrace: 5 * time.Minute}
}

// connectLine pulls the string out of what --remote printed, as a person would.
func connectLine(t *testing.T, output string) string {
	t.Helper()
	for line := range strings.Lines(output) {
		if text, ok := strings.CutPrefix(strings.TrimSpace(line), "servef connect "); ok {
			return text
		}
	}
	t.Fatalf("no connect line in %q", output)
	return ""
}

type connectFake struct {
	opened    []string
	installed bool
	openErr   error
	store     string
}

func newConnectFake(t *testing.T) (*connectFake, connectDeps) {
	t.Helper()
	fake := &connectFake{installed: true, store: filepath.Join(t.TempDir(), "servef", "connections.json")}
	return fake, connectDeps{
		goos:      "linux",
		getenv:    func(string) string { return "" },
		terminal:  func(io.Writer) bool { return false },
		probe:     connection.Probe,
		storePath: func() (string, error) { return fake.store, nil },
		installed: func(context.Context) (bool, error) { return fake.installed, nil },
		openApp: func(_ context.Context, target string) error {
			fake.opened = append(fake.opened, target)
			return fake.openErr
		},
		now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
}

// What `servef --remote` prints is what `servef connect` accepts, and the
// link it opens carries no secret.
func TestRemoteStringConnects(t *testing.T) {
	disableColor(t)
	root := appRoot(t)
	deps, socket := realRemote(t)
	var printed, stderr bytes.Buffer
	if err := serveRemote(context.Background(), remoteOptions(root), &printed, &stderr, deps); err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	for _, want := range []string{root, "servef connect 127.0.0.1:", "stops 10 min after this if nothing connects, and 5 min after the last UI closes", "servef remote stop"} {
		if !strings.Contains(printed.String(), want) {
			t.Errorf("output = %q, want %q", printed.String(), want)
		}
	}
	if strings.Contains(printed.String(), "web:") {
		t.Errorf("output = %q, want no browser link without --web", printed.String())
	}
	text := connectLine(t, printed.String())

	fake, connect := newConnectFake(t)
	var stdout bytes.Buffer
	if err := runConnect(context.Background(), connectOptions{text: text}, &stdout, &stderr, connect); err != nil {
		t.Fatalf("connect: %v", err)
	}
	endpoint, err := connection.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	wantLink := "servef://" + endpoint.Credential.ID() + "/"
	if len(fake.opened) != 1 || fake.opened[0] != wantLink {
		t.Errorf("opened %v, want %s", fake.opened, wantLink)
	}
	if !strings.Contains(stdout.String(), "desktop: "+wantLink+"\n") || !strings.Contains(stdout.String(), "connected to ") {
		t.Errorf("stdout = %q", stdout.String())
	}
	if strings.Contains(stdout.String(), endpoint.Credential.Encode()) {
		t.Errorf("stdout = %q carries the secret", stdout.String())
	}
	saved, found, err := connection.NewStore(fake.store).Lookup(endpoint.Credential.ID())
	if err != nil || !found {
		t.Fatalf("Lookup() = %v, %t, %v", saved, found, err)
	}
	if saved.Credential != endpoint.Credential.Encode() || saved.Port != endpoint.Port || saved.Label == "" {
		t.Errorf("saved = %+v", saved)
	}
	info, err := os.Stat(fake.store)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("store mode = %v, %v", info, err)
	}

	// status prints the same string; stop closes it.
	client := daemon.NewClient(socket)
	var status bytes.Buffer
	if err := remoteStatus(context.Background(), client, false, deps.candidates, &status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.String(), "servef connect "+text+"\n") {
		t.Errorf("status = %q, want the string %s", status.String(), text)
	}
	var rotated bytes.Buffer
	if err := remoteRotate(context.Background(), client, func() (string, error) { return "", errors.New("unused") }, deps.candidates, &rotated); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rotated.String(), text) || !strings.Contains(rotated.String(), "servef connect 127.0.0.1:") {
		t.Errorf("rotated = %q", rotated.String())
	}
	// The old string now names a server with another identity.
	err = runConnect(context.Background(), connectOptions{text: text}, &stdout, &stderr, connect)
	if err == nil || !strings.Contains(err.Error(), "different identity") {
		t.Errorf("connect after rotate = %v", err)
	}
	var stopped bytes.Buffer
	if err := remoteStop(context.Background(), client, &stopped); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stopped.String(), "stopped") {
		t.Errorf("stop = %q", stopped.String())
	}
}

func TestRemoteWebPrintsATokenLinkOnlyWhenAsked(t *testing.T) {
	disableColor(t)
	root := appRoot(t)
	deps, _ := realRemote(t)
	opts := remoteOptions(root)
	opts.web = true
	var printed, stderr bytes.Buffer
	if err := serveRemote(context.Background(), opts, &printed, &stderr, deps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(printed.String(), "web:  https://127.0.0.1:") || !strings.Contains(printed.String(), "/?token=") {
		t.Errorf("output = %q", printed.String())
	}
}

func TestRemoteRequestFollowsTheFlags(t *testing.T) {
	opts := options{host: defaultHost, port: 0, remoteFirstWait: time.Minute, remoteGrace: time.Second}
	request := remoteRequest(opts)
	if request.Host != "" || request.Port != nil || request.FirstWait != "1m0s" || request.Grace != "1s" {
		t.Errorf("no address flags: %+v", request)
	}
	opts.host, opts.hostSet, opts.port, opts.portSet = "10.0.0.5", true, 8000, true
	request = remoteRequest(opts)
	if request.Host != "10.0.0.5" || request.Port == nil || *request.Port != 8000 {
		t.Errorf("address flags: %+v", request)
	}
}

func TestConnectHosts(t *testing.T) {
	detected := func() []string { return []string{"10.0.0.5", "192.168.1.20"} }
	for _, test := range []struct {
		name string
		opts options
		want string
	}{
		{"detected", options{host: defaultHost}, "10.0.0.5,192.168.1.20"},
		{"one address", options{host: "10.9.9.9", hostSet: true}, "10.9.9.9"},
		{"localhost", options{host: defaultHost, hostSet: true}, defaultHost},
		{"every interface", options{host: "0.0.0.0", hostSet: true}, "10.0.0.5,192.168.1.20"},
		{"every interface v6", options{host: "::", hostSet: true}, "10.0.0.5,192.168.1.20"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := strings.Join(connectHosts(test.opts, detected), ","); got != test.want {
				t.Errorf("connectHosts() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestRemoteExplainsAnOlderDaemon(t *testing.T) {
	err := remoteFailure(&daemon.ControlError{Status: 404, Message: "Unknown daemon endpoint."})
	if !strings.Contains(err.Error(), "servef daemon stop") {
		t.Errorf("err = %v", err)
	}
	err = remoteFailure(&daemon.ControlError{Status: 503, Code: contract.CodePortBusy, Message: "busy"})
	var failure *daemon.ControlError
	if !errors.As(err, &failure) {
		t.Errorf("err = %v, want the daemon's own failure", err)
	}
}

func validConnectText(t *testing.T) string {
	t.Helper()
	return connection.TLS{Hosts: []string{"10.0.0.5"}, Port: 7971, Credential: connection.Credential{Fingerprint: [connection.FingerprintSize]byte{1}, Token: [16]byte{2}}}.Format()
}

func TestConnectExplainsEachFailure(t *testing.T) {
	text := validConnectText(t)
	for _, test := range []struct {
		name string
		err  error
		want []string
	}{
		{"unreachable", connection.ErrUnreachable, []string{"could not reach 10.0.0.5 on port 7971", "ssh -L 7971:localhost:7971 <server>", "--host localhost", "servef <path> --remote"}},
		{"fingerprint", connection.ErrFingerprint, []string{"different identity", "rotated"}},
		{"token", connection.ErrToken, []string{"refused this string's token"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake, deps := newConnectFake(t)
			deps.probe = func(context.Context, connection.TLS) (connection.TLS, contract.RemoteInfo, error) {
				return connection.TLS{}, contract.RemoteInfo{}, test.err
			}
			err := runConnect(context.Background(), connectOptions{text: text}, io.Discard, io.Discard, deps)
			if err == nil {
				t.Fatal("connected")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %q, want %q", err, want)
				}
			}
			if len(fake.opened) != 0 {
				t.Errorf("opened %v", fake.opened)
			}
			if _, err := os.Stat(fake.store); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a failed connect saved: %v", err)
			}
		})
	}
}

func TestConnectRejectsWhatIsNotAString(t *testing.T) {
	_, deps := newConnectFake(t)
	deps.probe = func(context.Context, connection.TLS) (connection.TLS, contract.RemoteInfo, error) {
		t.Fatal("probed")
		return connection.TLS{}, contract.RemoteInfo{}, nil
	}
	for _, text := range []string{"", "nonsense", "http://10.0.0.5:7971/"} {
		err := runConnect(context.Background(), connectOptions{text: text}, io.Discard, io.Discard, deps)
		if err == nil || !strings.Contains(err.Error(), "not a servef connection string") {
			t.Errorf("connect(%q) = %v", text, err)
		}
	}
	err := runConnect(context.Background(), connectOptions{text: validConnectText(t), host: "not a host!"}, io.Discard, io.Discard, deps)
	if err == nil || !strings.Contains(err.Error(), "--host") {
		t.Errorf("bad --host = %v", err)
	}
}

func TestConnectHostOverrideReplacesTheOffered(t *testing.T) {
	_, deps := newConnectFake(t)
	var probed []string
	deps.probe = func(_ context.Context, endpoint connection.TLS) (connection.TLS, contract.RemoteInfo, error) {
		probed = endpoint.Hosts
		return endpoint, contract.RemoteInfo{Name: "box", Version: "1"}, nil
	}
	if err := runConnect(context.Background(), connectOptions{text: validConnectText(t), host: "localhost", noOpen: true}, io.Discard, io.Discard, deps); err != nil {
		t.Fatal(err)
	}
	if len(probed) != 1 || probed[0] != "localhost" {
		t.Errorf("probed %v", probed)
	}
}

func TestConnectWithoutTheAppOffersTheBrowser(t *testing.T) {
	notInstalled := func(fake *connectFake) { fake.installed = false }
	noHandler := func(fake *connectFake) { fake.openErr = desktopapp.ErrNotInstalled }
	for name, arrange := range map[string]func(*connectFake){"undetected": notInstalled, "no handler": noHandler} {
		t.Run(name, func(t *testing.T) {
			fake, deps := newConnectFake(t)
			arrange(fake)
			deps.probe = func(_ context.Context, endpoint connection.TLS) (connection.TLS, contract.RemoteInfo, error) {
				return endpoint, contract.RemoteInfo{Name: "box", Version: "1"}, nil
			}
			var stdout, stderr bytes.Buffer
			if err := runConnect(context.Background(), connectOptions{text: validConnectText(t)}, &stdout, &stderr, deps); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stderr.String(), desktopapp.ErrNotInstalled.Error()) || !strings.Contains(stderr.String(), desktopapp.InstallHint("linux")) {
				t.Errorf("stderr = %q", stderr.String())
			}
			if !strings.Contains(stdout.String(), "web: https://10.0.0.5:7971/?token=") {
				t.Errorf("stdout = %q", stdout.String())
			}
		})
	}
}

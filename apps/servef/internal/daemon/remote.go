package daemon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/remote"
	"github.com/flexdinesh/servef/internal/runtime"
	"github.com/flexdinesh/servef/internal/version"
	"github.com/flexdinesh/servef/pkg/contract"
)

const (
	// DefaultRemoteFirstWait is how long a remote listener waits for its
	// first UI: long enough to copy the string to another machine.
	DefaultRemoteFirstWait = 10 * time.Minute
	// DefaultRemoteGrace is how long a remote listener outlives its last UI:
	// through reloads, app restarts, and network changes.
	DefaultRemoteGrace = 5 * time.Minute
	// remoteWriteTimeout replaces the server's 30 s: search documents can take
	// minutes over a slow link.
	remoteWriteTimeout = 10 * time.Minute
	// remoteSettle lets a closing listener's requests finish before asking
	// whether anything else is active.
	remoteSettle = time.Second
)

// presencePath is the stream a page holds while it is open (decision 0098).
const presencePath = "/api/presence"

// RemoteControl serves the daemon over TCP on request. Its operations back
// /_daemon/remote. Failures are *app.Error.
type RemoteControl interface {
	Status() contract.DaemonRemote
	// Start listens, or keeps listening, and restarts the wait for a UI.
	Start(request contract.DaemonRemoteRequest) (contract.DaemonRemote, error)
	// Stop closes the listener; the daemon exits if nothing else is active.
	Stop() contract.DaemonRemote
	// Rotate replaces the identity. A listener keeps its address.
	Rotate() (contract.DaemonRemote, error)
}

// timerFunc starts a timer; its result stops it. Tests replace it.
type timerFunc func(time.Duration, func()) (stop func() bool)

func realTimer(d time.Duration, fire func()) func() bool { return time.AfterFunc(d, fire).Stop }

// uiLifetime fires once when a listener has served no UI for too long: first
// when none has come yet, grace after the last leaves.
type uiLifetime struct {
	after timerFunc
	fire  func()

	mu      sync.Mutex
	grace   time.Duration
	clients int
	running bool
	// generation discards a timer that fired while being replaced.
	generation int
	stopTimer  func() bool
}

// start begins or restarts the wait; a UI already here holds it off.
func (l *uiLifetime) start(first, grace time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.running, l.grace = true, grace
	if l.clients == 0 {
		l.arm(first)
	}
}

// stop ends the wait; nothing fires until the next start.
func (l *uiLifetime) stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.running = false
	l.disarm()
}

func (l *uiLifetime) arm(d time.Duration) {
	l.disarm()
	generation := l.generation
	l.stopTimer = l.after(d, func() {
		l.mu.Lock()
		if generation != l.generation || !l.running {
			l.mu.Unlock()
			return
		}
		l.running = false
		l.mu.Unlock()
		l.fire()
	})
}

func (l *uiLifetime) disarm() {
	l.generation++
	if l.stopTimer != nil {
		l.stopTimer()
		l.stopTimer = nil
	}
}

func (l *uiLifetime) join() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.clients++
	l.disarm()
}

func (l *uiLifetime) leave() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.clients--
	if l.clients == 0 && l.running {
		l.arm(l.grace)
	}
}

func (l *uiLifetime) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.clients
}

// track counts a page's presence stream as a UI for as long as it is open.
func (l *uiLifetime) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == presencePath && r.Method == http.MethodGet {
			l.join()
			defer l.leave()
		}
		next.ServeHTTP(w, r)
	})
}

// remoteServer is the daemon's optional TCP binding. Only the web app is
// reachable through it: the control API stays on the Unix socket.
type remoteServer struct {
	runtime      *runtime.Runtime
	web          http.Handler
	identityFile string
	idle         *idleTracker
	exit         func()
	after        timerFunc
	log          io.Writer
	lifetime     *uiLifetime

	mu       sync.Mutex
	identity *remote.Identity
	bound    *runtime.Bound
	host     string
	port     int
}

func newRemoteServer(rt *runtime.Runtime, web http.Handler, identityFile string, idle *idleTracker, exit func(), log io.Writer) *remoteServer {
	r := &remoteServer{runtime: rt, web: web, identityFile: identityFile, idle: idle, exit: exit, after: realTimer, log: log}
	r.lifetime = &uiLifetime{after: r.after, fire: func() {
		_, _ = fmt.Fprintln(r.log, "remote listener closing: no UI connected")
		r.closeListener()
	}}
	return r
}

// Status implements RemoteControl.
func (r *remoteServer) Status() contract.DaemonRemote {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked()
}

func (r *remoteServer) statusLocked() contract.DaemonRemote {
	if r.bound == nil {
		return contract.DaemonRemote{}
	}
	return contract.DaemonRemote{
		Listening:  true,
		Host:       r.host,
		Port:       r.port,
		ID:         r.identity.Credential.ID(),
		Credential: r.identity.Credential.Encode(),
		Clients:    r.lifetime.count(),
	}
}

// Start implements RemoteControl.
func (r *remoteServer) Start(request contract.DaemonRemoteRequest) (contract.DaemonRemote, error) {
	first, err := remoteDuration(request.FirstWait, DefaultRemoteFirstWait)
	if err != nil {
		return contract.DaemonRemote{}, err
	}
	grace, err := remoteDuration(request.Grace, DefaultRemoteGrace)
	if err != nil {
		return contract.DaemonRemote{}, err
	}
	if request.Host != "" && request.Host != "localhost" && net.ParseIP(request.Host) == nil {
		return contract.DaemonRemote{}, invalidRemote("Host must be an IP address.")
	}
	if request.Port != nil && (*request.Port < 0 || *request.Port > 65535) {
		return contract.DaemonRemote{}, invalidRemote("Port must be between 0 and 65535.")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.identity == nil {
		identity, err := remote.LoadOrCreateIdentity(r.identityFile)
		if err != nil {
			return contract.DaemonRemote{}, &app.Error{Kind: contract.KindInternal, Message: err.Error()}
		}
		r.identity = identity
	}
	sameAddress := r.host == request.Host && (request.Port == nil || *request.Port == r.port)
	if r.bound != nil && sameAddress {
		r.lifetime.start(first, grace)
		return r.statusLocked(), nil
	}
	if r.bound != nil {
		r.bound.Close()
		r.bound = nil
	}
	if err := r.listenLocked(request.Host, request.Port); err != nil {
		return contract.DaemonRemote{}, err
	}
	r.lifetime.start(first, grace)
	return r.statusLocked(), nil
}

func (r *remoteServer) listenLocked(host string, port *int) error {
	listener, err := remote.Listen(host, port, r.identity.Port)
	if errors.Is(err, remote.ErrPortBusy) {
		return &app.Error{Kind: contract.KindUnavailable, Code: contract.CodePortBusy, Message: err.Error()}
	}
	if err != nil {
		return &app.Error{Kind: contract.KindInternal, Message: "Listen: " + err.Error()}
	}
	bound, err := remote.Port(listener)
	if err != nil {
		_ = listener.Close()
		return &app.Error{Kind: contract.KindInternal, Message: err.Error()}
	}
	// A failure only means the next start may pick another port.
	if err := r.identity.RememberPort(bound); err != nil {
		_, _ = fmt.Fprintf(r.log, "remember remote port: %v\n", err)
	}
	served, err := r.runtime.Bind(runtime.Binding{
		Listener:     listener,
		Handler:      r.handler(),
		TLS:          r.identity.TLSConfig(),
		WriteTimeout: remoteWriteTimeout,
	})
	if err != nil {
		return &app.Error{Kind: contract.KindUnavailable, Message: err.Error()}
	}
	r.bound, r.host, r.port = served, host, bound
	_, _ = fmt.Fprintf(r.log, "remote listener on port %d\n", bound)
	return nil
}

// handler composes what TCP adds to the web app. Authentication comes first,
// so only a real client counts as a UI.
func (r *remoteServer) handler() http.Handler {
	name, err := os.Hostname()
	if err != nil {
		name = "servef"
	}
	info := contract.RemoteInfo{Protocol: ProtocolVersion, Version: version.Number(), Name: name}
	return r.idle.wrap(remote.Authenticate(r.identity.Credential.TokenText(), info, r.lifetime.track(remote.Compress(r.web))))
}

// Stop implements RemoteControl.
func (r *remoteServer) Stop() contract.DaemonRemote {
	r.closeListener()
	return contract.DaemonRemote{}
}

// closeListener closes the listener, then exits the daemon if nothing else
// needs it. A local window's presence stream is such a thing.
func (r *remoteServer) closeListener() {
	r.lifetime.stop()
	r.mu.Lock()
	bound := r.bound
	r.bound = nil
	r.mu.Unlock()
	if bound != nil {
		bound.Close()
	}
	r.after(remoteSettle, func() {
		r.mu.Lock()
		listening := r.bound != nil
		r.mu.Unlock()
		if !listening && r.idle.quiet() {
			r.exit()
		}
	})
}

// Rotate implements RemoteControl.
func (r *remoteServer) Rotate() (contract.DaemonRemote, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wasListening := r.bound != nil
	if wasListening {
		r.lifetime.stop()
		r.bound.Close()
		r.bound = nil
	}
	identity, err := remote.RotateIdentity(r.identityFile)
	if err != nil {
		return contract.DaemonRemote{}, &app.Error{Kind: contract.KindInternal, Message: err.Error()}
	}
	r.identity = identity
	if !wasListening {
		return contract.DaemonRemote{}, nil
	}
	port := r.port
	if err := r.listenLocked(r.host, &port); err != nil {
		return contract.DaemonRemote{}, err
	}
	r.lifetime.start(DefaultRemoteFirstWait, r.lifetime.currentGrace())
	return r.statusLocked(), nil
}

func (l *uiLifetime) currentGrace() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.grace
}

// shutdown stops the timers when the daemon stops; Serve closes the binding.
func (r *remoteServer) shutdown() { r.lifetime.stop() }

func remoteDuration(text string, fallback time.Duration) (time.Duration, error) {
	if text == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(text)
	if err != nil || duration <= 0 {
		return 0, invalidRemote("Durations must be positive, such as 10m.")
	}
	return duration, nil
}

func invalidRemote(message string) error {
	return &app.Error{Kind: contract.KindInvalid, Code: contract.CodeInvalidRequest, Message: message}
}

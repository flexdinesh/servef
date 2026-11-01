// Package daemon serves servef over a per-user Unix socket (decision 0061):
// the web routes plus a /_daemon control API, persisted runtime roots, single
// instance, and idle exit. It knows no CLI flags or process spawning.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/runtime"
	"github.com/flexdinesh/servef/internal/version"
)

// ProtocolVersion versions what clients rely on: /_daemon/* and Status. Only
// a protocol mismatch blocks; /_daemon/status and /_daemon/stop keep their
// shape across protocol versions so any client can report and stop.
const ProtocolVersion = 1

// DefaultIdleExit is how long a daemon with no request in flight keeps
// running.
const DefaultIdleExit = 30 * time.Minute

// Config configures Run.
type Config struct {
	Socket    string // required
	RootsFile string // required
	Depth     int
	ScanDepth int
	// IdleExit stops the daemon after this long with no request in flight;
	// 0 never stops.
	IdleExit time.Duration
	// Log receives startup, skipped roots, and the stop reason; nil discards.
	Log io.Writer
}

var (
	errStopRequested = errors.New("stop requested")
	errIdle          = errors.New("idle")
)

// Run serves until ctx ends, /_daemon/stop, or idle exit. It loads the saved
// roots (a malformed file fails before listening), skipping and logging roots
// that are missing or overlap, then takes the socket (ErrRunning if another
// daemon holds it) and serves the web routes and the control API.
func Run(ctx context.Context, config Config) error {
	if config.Socket == "" || config.RootsFile == "" {
		return errors.New("daemon: socket and roots file are required")
	}
	log := config.Log
	if log == nil {
		log = io.Discard
	}
	saved, err := LoadRoots(config.RootsFile)
	if err != nil {
		return err
	}
	roots := &projects.RootSet{}
	var skipped []string
	for _, root := range saved {
		if _, _, err := roots.Add(root); err != nil {
			_, _ = fmt.Fprintf(log, "skipping root %s: %v\n", root, err)
			skipped = append(skipped, root)
		}
	}
	listener, err := Listen(config.Socket)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	rt, err := runtime.New(runtime.Config{Roots: roots, Depth: config.Depth, ScanDepth: config.ScanDepth, SessionKey: "daemon"})
	if err != nil {
		return err
	}
	defer rt.Close()

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	control := &control{
		roots:     roots,
		skipped:   skipped,
		rootsFile: config.RootsFile,
		stop:      func() { cancel(errStopRequested) },
		done:      ctx.Done(),
		notices:   rt,
		status: Status{
			Protocol:  ProtocolVersion,
			Version:   version.Number(),
			PID:       pid(),
			Socket:    config.Socket,
			Depth:     config.Depth,
			ScanDepth: config.ScanDepth,
			IdleExit:  config.IdleExit.String(),
			StartedAt: time.Now().UTC().Truncate(time.Second),
		},
	}
	idle := newIdleTracker(config.IdleExit, func() { cancel(errIdle) })
	defer idle.stop()
	_, _ = fmt.Fprintf(log, "servef daemon %s (protocol %d) listening on %s with %d roots\n", version.Number(), ProtocolVersion, config.Socket, len(roots.List()))
	err = rt.Serve(ctx, listener, idle.wrap(control.wrap(rt.Handler())))
	_, _ = fmt.Fprintf(log, "servef daemon stopped: %v\n", context.Cause(ctx))
	return err
}

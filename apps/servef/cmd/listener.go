package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/flexdinesh/servef/internal/control"
)

const (
	defaultPortStart = 7971
	defaultPortEnd   = 7980
)

var errDefaultPortsBusy = errors.New("all default ports are busy")

type listenFunc func(string, int) (net.Listener, error)

func openListener(host string, opts options, output io.Writer, listenAt listenFunc) (net.Listener, error) {
	if opts.portSet {
		return listenAt(host, opts.port)
	}
	for port := defaultPortStart; port <= defaultPortEnd; port++ {
		listener, err := listenAt(host, port)
		if err == nil {
			return listener, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}
		writePortBusy(output, port, port < defaultPortEnd)
	}
	return nil, errDefaultPortsBusy
}

func runningDefaultServers(ctx context.Context, registry control.Registry, host string) ([]control.Remote, error) {
	servers := make([]control.Remote, 0, defaultPortEnd-defaultPortStart+1)
	for port := defaultPortStart; port <= defaultPortEnd; port++ {
		server, err := registry.Lookup(host, port)
		if err != nil || !server.Running(ctx) {
			return nil, fmt.Errorf("port %d is busy but is not a verified servef server", port)
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func stopServers(ctx context.Context, servers []control.Remote) error {
	for _, server := range servers {
		if err := server.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

func waitForListener(ctx context.Context, host string, port int, listenAt listenFunc) (net.Listener, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		listener, err := listenAt(host, port)
		if err == nil {
			return listener, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for port %d: %w", port, ctx.Err())
		case <-ticker.C:
		}
	}
}

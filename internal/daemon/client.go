package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

// ErrNotRunning reports that nothing answers on the socket.
var ErrNotRunning = errors.New("servef daemon is not running")

// ProtocolMismatchError reports a daemon speaking another protocol.
type ProtocolMismatchError struct {
	Daemon, Client        int
	Socket, DaemonVersion string
}

func (e *ProtocolMismatchError) Error() string {
	return fmt.Sprintf("servef daemon %s at %s speaks protocol %d; this servef speaks protocol %d. Stop it with `servef daemon stop`, or use a servef build with protocol %d",
		e.DaemonVersion, e.Socket, e.Daemon, e.Client, e.Daemon)
}

// stopPollInterval paces Stop's wait for the socket to go quiet.
const stopPollInterval = 50 * time.Millisecond

// Client talks HTTP to a daemon over its socket with Host "localhost".
type Client struct {
	socket string
	http   *http.Client
}

// NewClient returns a client for the daemon at socket. It uses no proxy,
// dials within 2 s, and does not follow redirects.
func NewClient(socket string) *Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}
	return &Client{socket: socket, http: &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Status returns the daemon's status, or ErrNotRunning when nothing answers.
// An answer that is not a servef daemon is an error naming the socket. Status
// never checks the protocol; callers do.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var status Status
	if err := c.call(ctx, http.MethodGet, "status", nil, http.StatusOK, &status); err != nil {
		return Status{}, err
	}
	if status.Protocol <= 0 {
		return Status{}, fmt.Errorf("%s does not answer as a servef daemon", c.socket)
	}
	return status, nil
}

// Roots returns the daemon's served roots.
func (c *Client) Roots(ctx context.Context) ([]string, error) {
	var response rootsResponse
	if err := c.call(ctx, http.MethodGet, "roots", nil, http.StatusOK, &response); err != nil {
		return nil, err
	}
	return response.Roots, nil
}

// AddRoot asks the daemon to serve path, an absolute directory. It returns
// the canonical root and whether it was new.
func (c *Client) AddRoot(ctx context.Context, path string) (string, bool, error) {
	var response addRootResponse
	err := c.call(ctx, http.MethodPost, "roots", addRootRequest{Path: path}, 0, &response)
	if err != nil {
		return "", false, err
	}
	return response.Root, response.Added, nil
}

// Stop asks the daemon to stop and waits, within ctx, until the socket stops
// answering. A daemon that is not running is already stopped.
func (c *Client) Stop(ctx context.Context) error {
	var response stopResponse
	if err := c.call(ctx, http.MethodPost, "stop", nil, http.StatusAccepted, &response); err != nil {
		if errors.Is(err, ErrNotRunning) {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()
	for {
		if _, err := c.Status(ctx); errors.Is(err, ErrNotRunning) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("servef daemon at %s did not stop: %w", c.socket, ctx.Err())
		case <-ticker.C:
		}
	}
}

// call sends a control request. want is the success status; 0 accepts 200
// and 201. A JSON error body becomes the error.
func (c *Client) call(ctx context.Context, method, name string, body any, want int, result any) error {
	var reader io.Reader
	if body != nil {
		content, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(content)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://localhost"+controlPrefix+name, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return ErrNotRunning
		}
		return fmt.Errorf("reach servef daemon at %s: %w", c.socket, err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read servef daemon response: %w", err)
	}
	ok := response.StatusCode == want || (want == 0 && (response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated))
	if !ok {
		var failure errorResponse
		if json.Unmarshal(content, &failure) == nil && failure.Error != "" {
			return fmt.Errorf("servef daemon: %s", failure.Error)
		}
		return fmt.Errorf("%s does not answer as a servef daemon (%s %s: %s)", c.socket, method, request.URL.Path, response.Status)
	}
	if err := json.Unmarshal(content, result); err != nil {
		return fmt.Errorf("%s does not answer as a servef daemon: %w", c.socket, err)
	}
	return nil
}

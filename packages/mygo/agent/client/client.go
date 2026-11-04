// Package client speaks mygo's automation protocol to a socket: a headless
// session (agent/session) or a built app's MYGO_AUTOMATION socket.
package client

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/servef/packages/mygo/agent/protocol"
)

// Client is a connection; its calls run one at a time.
type Client struct {
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
	next int
}

// Dial connects to the socket at path.
func Dial(path string) (*Client, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, r: bufio.NewReaderSize(conn, 1<<20)}, nil
}

// DialWait dials until the socket answers or timeout passes.
func DialWait(path string, timeout time.Duration) (*Client, error) {
	deadline := time.Now().Add(timeout)
	for {
		c, err := Dial(path)
		if err == nil {
			return c, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Call sends a request and decodes its result into result (nil to skip).
// A failure the app answers is a *protocol.Error.
func (c *Client) Call(method string, params, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	req := protocol.Request{ID: json.RawMessage(strconv.Itoa(c.next)), Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = b
	}
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		return err
	}
	answer, err := c.r.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	var res protocol.Response
	if err := json.Unmarshal(answer, &res); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if res.Error != nil {
		return res.Error
	}
	if result != nil && len(res.Result) > 0 {
		return json.Unmarshal(res.Result, result)
	}
	return nil
}

// Windows lists the open windows.
func (c *Client) Windows() ([]protocol.Window, error) {
	var r protocol.WindowsResult
	err := c.Call(protocol.MethodWindows, nil, &r)
	return r.Windows, err
}

// Tree returns the elements of a window's last frame.
func (c *Client) Tree(window int) (protocol.TreeResult, error) {
	var r protocol.TreeResult
	err := c.Call(protocol.MethodTree, protocol.WindowParams{Window: window}, &r)
	return r, err
}

// Act performs an action on an element.
func (c *Client) Act(p protocol.ActParams) error { return c.Call(protocol.MethodAct, p, nil) }

// Input sends events to a window.
func (c *Client) Input(window int, events ...protocol.InputEvent) error {
	return c.Call(protocol.MethodInput, protocol.InputParams{Window: window, Events: events}, nil)
}

// Menu chooses a menu item by its path.
func (c *Client) Menu(window int, path ...string) error {
	return c.Call(protocol.MethodMenu, protocol.MenuParams{Window: window, Path: path}, nil)
}

// Capture returns a window as PNG bytes.
func (c *Client) Capture(window int) ([]byte, error) {
	var r protocol.CaptureResult
	err := c.Call(protocol.MethodCapture, protocol.WindowParams{Window: window}, &r)
	return r.PNG, err
}

// Idle waits until a window settles; timeout 0 waits for ever.
func (c *Client) Idle(window int, timeout time.Duration) error {
	return c.Call(protocol.MethodIdle, protocol.IdleParams{Window: window, Timeout: int(timeout / time.Millisecond)}, nil)
}

// Resize resizes a headless window.
func (c *Client) Resize(window, width, height int) error {
	return c.Call(protocol.MethodResize, protocol.ResizeParams{Window: window, Width: width, Height: height}, nil)
}

// SetDark switches a headless window's appearance.
func (c *Client) SetDark(window int, dark bool) error {
	return c.Call(protocol.MethodSetDark, protocol.DarkParams{Window: window, Dark: dark}, nil)
}

// Advance moves a headless window's clock on.
func (c *Client) Advance(window int, d time.Duration) error {
	return c.Call(protocol.MethodAdvance, protocol.AdvanceParams{Window: window, MS: int(d / time.Millisecond)}, nil)
}

// Shutdown stops a headless session.
func (c *Client) Shutdown() error { return c.Call(protocol.MethodShutdown, nil, nil) }

// ErrNotFound reports no element showing a text.
var ErrNotFound = errors.New("no element shows the text")

// FindLabel returns the first node labeled text, or, failing that, the
// first whose label contains it.
func FindLabel(nodes []protocol.Node, text string) (protocol.Node, bool) {
	for _, n := range nodes {
		if n.Label == text {
			return n, true
		}
	}
	for _, n := range nodes {
		if text != "" && strings.Contains(n.Label, text) {
			return n, true
		}
	}
	return protocol.Node{}, false
}

// Press presses node: by its press action when it has one, else with a
// click at the middle of its box, as a text inside a button needs in a
// built app.
func (c *Client) Press(window int, node protocol.Node) error {
	if node.Has("press") {
		return c.Act(protocol.ActParams{Window: window, ID: node.ID, Action: "press"})
	}
	if node.Bounds.Width == 0 && node.Bounds.Height == 0 {
		return fmt.Errorf("%q has no box to click", node.Label)
	}
	x, y := node.Bounds.Center()
	return c.Input(window, protocol.InputEvent{Type: "click", X: x, Y: y})
}

// ClickText presses the element showing text.
func (c *Client) ClickText(window int, text string) error {
	tree, err := c.Tree(window)
	if err != nil {
		return err
	}
	node, ok := FindLabel(tree.Nodes, text)
	if !ok {
		return fmt.Errorf("%w %q; labels %q", ErrNotFound, text, Labels(tree.Nodes))
	}
	return c.Press(window, node)
}

// WaitText waits until an element shows text (exactly, or containing it).
func (c *Client) WaitText(window int, text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_ = c.Idle(window, timeout)
		tree, err := c.Tree(window)
		if err != nil {
			return err
		}
		if _, ok := FindLabel(tree.Nodes, text); ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w %q after %v; labels %q", ErrNotFound, text, timeout, Labels(tree.Nodes))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Labels returns the nodes' labels.
func Labels(nodes []protocol.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

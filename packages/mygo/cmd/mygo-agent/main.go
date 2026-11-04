// Command mygo-agent drives a mygo app the way agent-browser drives a web
// page: a headless session (agent/session) or a built app's
// MYGO_AUTOMATION socket, over mygo's automation protocol.
//
//	mygo-agent open [--replace] -- <host> [args]   start a host that serves a session at $MYGO_AGENT_SOCKET
//	mygo-agent open --attach <socket>  use a running session or app socket
//	mygo-agent snapshot [-i] [--json]  elements with @eN refs, role, label, bounds
//	mygo-agent click @e3|"text"|x,y [--right] [--double]
//	mygo-agent fill @e3|"label" <value>
//	mygo-agent type <text>
//	mygo-agent key <accelerator>       Enter, Shift+Tab, CmdOrCtrl+F
//	mygo-agent scroll <dy> [dx] [--at @e3|"text"|x,y]
//	mygo-agent resize <width> <height>
//	mygo-agent dark on|off
//	mygo-agent screenshot <out.png>
//	mygo-agent idle [--timeout ms]
//	mygo-agent wait --text <text> [--timeout ms]
//	mygo-agent windows | menu <item>... | run <scenario.yaml> [--out dir]
//	mygo-agent close
//
// Global flags: --session <name> (default "default") and --socket <path>
// (or $MYGO_AGENT_SOCKET). Refs from the last snapshot are kept beside the
// socket (<socket>.refs.json), so later commands can use them.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/agent/client"
	"github.com/flexdinesh/servef/packages/mygo/agent/demo"
	"github.com/flexdinesh/servef/packages/mygo/agent/protocol"
	"github.com/flexdinesh/servef/packages/mygo/agent/scenario"
	"github.com/flexdinesh/servef/packages/mygo/agent/session"
)

func main() {
	c := &cli{stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv}
	if err := c.run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mygo-agent:", err)
		os.Exit(1)
	}
}

type cli struct {
	stdout, stderr io.Writer
	getenv         func(string) string

	sessionName string
	socket      string
}

// state is a session's record: its socket, and the pid of a host open
// started.
type state struct {
	Socket string `json:"socket"`
	PID    int    `json:"pid,omitempty"`
}

// ref is a snapshot element kept for later commands.
type ref struct {
	Node protocol.Node `json:"node"`
}

const settleTimeout = 10 * time.Second

func (c *cli) run(args []string) error {
	c.sessionName = "default"
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		name, value, ok := strings.Cut(args[0], "=")
		if !ok && len(args) > 1 && (name == "--session" || name == "--socket") {
			value, args = args[1], args[1:]
			ok = true
		}
		switch {
		case name == "--session" && ok:
			c.sessionName = value
		case name == "--socket" && ok:
			c.socket = value
		default:
			return fmt.Errorf("unknown flag %s\n%s", args[0], usage)
		}
		args = args[1:]
	}
	if !sessionName.MatchString(c.sessionName) || strings.Contains(c.sessionName, "..") {
		return fmt.Errorf("--session %q: use letters, digits, '.', '_' and '-' (no path)", c.sessionName)
	}
	if len(args) == 0 {
		return errors.New(usage)
	}
	command, rest := args[0], args[1:]
	switch command {
	case "open":
		return c.open(rest)
	case "demo":
		return c.demo()
	case "help", "-h", "--help":
		_, _ = fmt.Fprintln(c.stdout, usage)
		return nil
	}
	socket, err := c.socketPath()
	if err != nil {
		return err
	}
	conn, err := client.Dial(socket)
	if err != nil {
		return fmt.Errorf("no session at %s (mygo-agent open first): %w", socket, err)
	}
	defer conn.Close()
	switch command {
	case "snapshot":
		return c.snapshot(conn, socket, rest)
	case "click":
		return c.click(conn, socket, rest)
	case "fill":
		if len(rest) != 2 {
			return errors.New("usage: fill @e3|label <value>")
		}
		node, err := c.resolveNode(conn, socket, rest[0])
		if err != nil {
			return err
		}
		if err := conn.Act(protocol.ActParams{ID: node.ID, Action: "setValue", Text: rest[1]}); err != nil {
			return err
		}
		return conn.Idle(0, settleTimeout)
	case "type":
		if len(rest) != 1 {
			return errors.New("usage: type <text>")
		}
		return c.input(conn, protocol.InputEvent{Type: "text", Text: rest[0]})
	case "key":
		if len(rest) != 1 {
			return errors.New("usage: key <accelerator>")
		}
		return c.input(conn, protocol.InputEvent{Type: "key", Key: rest[0]})
	case "scroll":
		return c.scroll(conn, socket, rest)
	case "resize":
		if len(rest) != 2 {
			return errors.New("usage: resize <width> <height>")
		}
		w, errW := strconv.Atoi(rest[0])
		h, errH := strconv.Atoi(rest[1])
		if errW != nil || errH != nil {
			return errors.New("usage: resize <width> <height>")
		}
		return conn.Resize(0, w, h)
	case "dark":
		if len(rest) != 1 || (rest[0] != "on" && rest[0] != "off") {
			return errors.New("usage: dark on|off")
		}
		return conn.SetDark(0, rest[0] == "on")
	case "screenshot":
		if len(rest) != 1 {
			return errors.New("usage: screenshot <out.png>")
		}
		png, err := conn.Capture(0)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(rest[0]), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(rest[0], png, 0o644); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(c.stdout, rest[0])
		return nil
	case "idle":
		timeout := settleTimeout
		if len(rest) == 2 && rest[0] == "--timeout" {
			ms, err := strconv.Atoi(rest[1])
			if err != nil {
				return err
			}
			timeout = time.Duration(ms) * time.Millisecond
		} else if len(rest) != 0 {
			return errors.New("usage: idle [--timeout ms]")
		}
		return conn.Idle(0, timeout)
	case "wait":
		return c.wait(conn, rest)
	case "windows":
		windows, err := conn.Windows()
		if err != nil {
			return err
		}
		return json.NewEncoder(c.stdout).Encode(windows)
	case "menu":
		if len(rest) == 0 {
			return errors.New("usage: menu <item>...")
		}
		if err := conn.Menu(0, rest...); err != nil {
			return err
		}
		return conn.Idle(0, settleTimeout)
	case "run":
		return c.runScenario(conn, rest)
	case "close":
		return c.close(conn, socket)
	}
	return fmt.Errorf("unknown command %q\n%s", command, usage)
}

const usage = `usage: mygo-agent [--session name] [--socket path] <command>
  open [--replace] -- <host> [args] | open --attach <socket>
  snapshot [-i] [--json]
  click @e3|"text"|x,y [--right] [--double]
  fill @e3|"label" <value>
  type <text>
  key <accelerator>
  scroll <dy> [dx] [--at @e3|"text"|x,y]
  resize <width> <height>
  dark on|off
  screenshot <out.png>
  idle [--timeout ms]
  wait --text <text> [--timeout ms]
  windows
  menu <item>...
  run <scenario.yaml> [--out dir]
  close`

// stateDir is the private directory sessions are recorded in: a real
// directory owned by the user, mode 0700 (prepareDir).
func (c *cli) stateDir() (string, error) {
	dir := c.getenv("MYGO_AGENT_DIR")
	if dir == "" {
		if runtime := c.getenv("XDG_RUNTIME_DIR"); runtime != "" {
			dir = filepath.Join(runtime, "mygo-agent")
		} else {
			dir = filepath.Join(os.TempDir(), "mygo-agent-"+strconv.Itoa(os.Getuid()))
		}
	}
	if err := prepareDir(dir, os.Getuid()); err != nil {
		return "", err
	}
	return dir, nil
}

func (c *cli) statePath() (string, error) {
	dir, err := c.stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, c.sessionName+".json"), nil
}

func (c *cli) readState() (state, bool) {
	path, err := c.statePath()
	if err != nil {
		return state{}, false
	}
	b, err := readPrivate(path)
	if err != nil {
		return state{}, false
	}
	var s state
	if json.Unmarshal(b, &s) != nil || s.Socket == "" {
		return state{}, false
	}
	return s, true
}

func (c *cli) writeState(s state) error {
	path, err := c.statePath()
	if err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writePrivate(path, b)
}

// socketPath is --socket, $MYGO_AGENT_SOCKET, the session's record, or
// the session's default socket, in that order.
func (c *cli) socketPath() (string, error) {
	if c.socket != "" {
		return c.socket, nil
	}
	if env := c.getenv("MYGO_AGENT_SOCKET"); env != "" {
		return env, nil
	}
	if s, ok := c.readState(); ok {
		return s.Socket, nil
	}
	return c.defaultSocket()
}

func (c *cli) defaultSocket() (string, error) {
	dir, err := c.stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, c.sessionName+".sock"), nil
}

// serving reports whether something answers the protocol at socket.
func serving(socket string) bool {
	conn, err := client.Dial(socket)
	if err != nil {
		return false
	}
	defer conn.Close()
	_, err = conn.Windows()
	return err == nil
}

func (c *cli) open(args []string) error {
	timeout := 30 * time.Second
	if len(args) == 2 && args[0] == "--attach" {
		conn, err := client.Dial(args[1])
		if err != nil {
			return err
		}
		defer conn.Close()
		windows, err := conn.Windows()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(c.stdout, "attached %s (%d windows)\n", args[1], len(windows))
		return c.writeState(state{Socket: args[1]})
	}
	replace := len(args) > 0 && args[0] == "--replace"
	if replace {
		args = args[1:]
	}
	if len(args) < 2 || args[0] != "--" {
		return errors.New("usage: open [--replace] -- <host> [args] | open --attach <socket>")
	}
	// --socket, then $MYGO_AGENT_SOCKET, then the session's default: a
	// record of an earlier session is not reused.
	socket := c.socket
	if socket == "" {
		socket = c.getenv("MYGO_AGENT_SOCKET")
	}
	if socket == "" {
		var err error
		if socket, err = c.defaultSocket(); err != nil {
			return err
		}
	}
	// A live session at the socket would answer for the new host.
	if serving(socket) {
		if !replace {
			return fmt.Errorf("a session already serves %s: close it (mygo-agent --session %s close) or pass open --replace", socket, c.sessionName)
		}
		conn, err := client.Dial(socket)
		if err != nil {
			return err
		}
		err = c.close(conn, socket)
		_ = conn.Close()
		if err != nil {
			return fmt.Errorf("replace %s: %w", socket, err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for serving(socket) {
			if time.Now().After(deadline) {
				return fmt.Errorf("the session at %s did not stop", socket)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	logPath := socket + ".log"
	log, err := createPrivate(logPath)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(args[1], args[2:]...)
	cmd.Env = append(os.Environ(), "MYGO_AGENT_SOCKET="+socket, "MYGO_AUTOMATION="+socket)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(timeout)
	for {
		// The child must be alive when the socket answers: an answer after
		// it exited came from someone else.
		select {
		case err := <-exited:
			return fmt.Errorf("%s exited (%v) before serving %s; see %s", args[1], err, socket, logPath)
		default:
		}
		if serving(socket) {
			select {
			case err := <-exited:
				return fmt.Errorf("%s exited (%v) before serving %s; see %s", args[1], err, socket, logPath)
			default:
			}
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			return fmt.Errorf("%s did not serve %s in %v; see %s", args[1], socket, timeout, logPath)
		}
		select {
		case err := <-exited:
			return fmt.Errorf("%s exited (%v) before serving %s; see %s", args[1], err, socket, logPath)
		case <-time.After(50 * time.Millisecond):
		}
	}
	_ = os.Remove(socket + ".refs.json")
	_, _ = fmt.Fprintf(c.stdout, "opened %s (pid %d, log %s)\n", socket, cmd.Process.Pid, logPath)
	return c.writeState(state{Socket: socket, PID: cmd.Process.Pid})
}

// demo hosts the demo counter at the session's socket, for trying the CLI.
func (c *cli) demo() error {
	socket, err := c.socketPath()
	if err != nil {
		return err
	}
	s := session.New(session.Options{Width: 480, Height: 360, Title: func() string { return "Counter" }, View: func(s *session.Session) func(*ui.Context) {
		return demo.New(s).View
	}})
	return s.ListenAndServe(socket)
}

func (c *cli) close(conn *client.Client, socket string) error {
	err := conn.Shutdown()
	var e *protocol.Error
	if errors.As(err, &e) && e.Code == protocol.CodeUnknownMethod {
		err = nil
		if s, ok := c.readState(); ok && s.PID > 0 && s.Socket == socket {
			if p, findErr := os.FindProcess(s.PID); findErr == nil {
				err = p.Signal(syscall.SIGTERM)
			}
		}
	}
	_ = os.Remove(socket + ".refs.json")
	if s, ok := c.readState(); ok && s.Socket == socket {
		if path, perr := c.statePath(); perr == nil {
			_ = os.Remove(path)
		}
	}
	return err
}

var sessionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// wait waits until an element shows a text, as fetches finish.
func (c *cli) wait(conn *client.Client, args []string) error {
	text, timeout := "", 10*time.Second
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--text" && i+1 < len(args):
			text = args[i+1]
			i++
		case args[i] == "--timeout" && i+1 < len(args):
			ms, err := strconv.Atoi(args[i+1])
			if err != nil || ms <= 0 {
				return errors.New("usage: wait --text <text> [--timeout ms]")
			}
			timeout = time.Duration(ms) * time.Millisecond
			i++
		default:
			return errors.New("usage: wait --text <text> [--timeout ms]")
		}
	}
	if text == "" {
		return errors.New("usage: wait --text <text> [--timeout ms]")
	}
	return conn.WaitText(0, text, timeout)
}

var interactiveRoles = map[string]bool{
	"button": true, "link": true, "checkBox": true, "radio": true, "switch": true, "slider": true,
	"textField": true, "popUpButton": true, "tab": true, "treeItem": true, "listItem": true,
	"menuButton": true, "toggleButton": true, "comboBox": true, "disclosure": true, "stepper": true,
	"menuItem": true, "menuItemCheckBox": true, "menuItemRadio": true, "cell": true, "row": true,
}

func interactive(n protocol.Node) bool {
	return interactiveRoles[n.Role] || n.Has("press") || n.Has("setValue")
}

func (c *cli) snapshot(conn *client.Client, socket string, args []string) error {
	only, asJSON := false, false
	for _, a := range args {
		switch a {
		case "-i", "--interactive":
			only = true
		case "--json":
			asJSON = true
		default:
			return errors.New("usage: snapshot [-i] [--json]")
		}
	}
	tree, err := conn.Tree(0)
	if err != nil {
		return err
	}
	refs := map[string]ref{}
	type line struct {
		Ref  string        `json:"ref"`
		Node protocol.Node `json:"node"`
	}
	var lines []line
	n := 0
	for _, node := range tree.Nodes {
		if only && !interactive(node) {
			continue
		}
		n++
		name := "e" + strconv.Itoa(n)
		refs[name] = ref{Node: node}
		lines = append(lines, line{Ref: "@" + name, Node: node})
	}
	b, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	if err := writePrivate(socket+".refs.json", b); err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(lines)
	}
	for _, l := range lines {
		b := l.Node.Bounds
		extra := ""
		if len(l.Node.States) > 0 {
			extra = " " + strings.Join(l.Node.States, ",")
		}
		if l.Node.Value != "" {
			extra += " value=" + strconv.Quote(l.Node.Value)
		}
		_, _ = fmt.Fprintf(c.stdout, "%s %s %s [%.0f,%.0f %.0fx%.0f]%s\n", l.Ref, l.Node.Role, strconv.Quote(l.Node.Label), b.X, b.Y, b.Width, b.Height, extra)
	}
	return nil
}

var point = regexp.MustCompile(`^(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)$`)

// target is a parsed @ref, text, or x,y.
type target struct {
	node  *protocol.Node
	x, y  float64
	point bool
}

func (c *cli) resolve(conn *client.Client, socket, s string) (target, error) {
	if m := point.FindStringSubmatch(s); m != nil {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		return target{x: x, y: y, point: true}, nil
	}
	node, err := c.resolveNode(conn, socket, s)
	if err != nil {
		return target{}, err
	}
	x, y := node.Bounds.Center()
	return target{node: &node, x: x, y: y}, nil
}

func (c *cli) resolveNode(conn *client.Client, socket, s string) (protocol.Node, error) {
	if name, ok := strings.CutPrefix(s, "@"); ok {
		b, err := readPrivate(socket + ".refs.json")
		if err != nil {
			return protocol.Node{}, fmt.Errorf("no snapshot refs for %s: run snapshot first", socket)
		}
		var refs map[string]ref
		if err := json.Unmarshal(b, &refs); err != nil {
			return protocol.Node{}, err
		}
		r, ok := refs[name]
		if !ok {
			return protocol.Node{}, fmt.Errorf("no ref @%s in the last snapshot", name)
		}
		return r.Node, nil
	}
	tree, err := conn.Tree(0)
	if err != nil {
		return protocol.Node{}, err
	}
	node, ok := client.FindLabel(tree.Nodes, s)
	if !ok {
		return protocol.Node{}, fmt.Errorf("no element shows %q", s)
	}
	return node, nil
}

func (c *cli) click(conn *client.Client, socket string, args []string) error {
	right, double := false, false
	var spec string
	for _, a := range args {
		switch a {
		case "--right":
			right = true
		case "--double":
			double = true
		default:
			if spec != "" {
				return errors.New("usage: click @e3|text|x,y [--right] [--double]")
			}
			spec = a
		}
	}
	if spec == "" {
		return errors.New("usage: click @e3|text|x,y [--right] [--double]")
	}
	t, err := c.resolve(conn, socket, spec)
	if err != nil {
		return err
	}
	switch {
	case right:
		err = conn.Input(0, protocol.InputEvent{Type: "click", Button: "right", X: t.x, Y: t.y})
	case double:
		err = conn.Input(0, protocol.InputEvent{Type: "click", Clicks: 2, X: t.x, Y: t.y})
	case t.point:
		err = conn.Input(0, protocol.InputEvent{Type: "click", X: t.x, Y: t.y})
	default:
		err = conn.Press(0, *t.node)
	}
	if err != nil {
		return err
	}
	return conn.Idle(0, settleTimeout)
}

func (c *cli) scroll(conn *client.Client, socket string, args []string) error {
	var at string
	var nums []float64
	for i := 0; i < len(args); i++ {
		if args[i] == "--at" && i+1 < len(args) {
			at = args[i+1]
			i++
			continue
		}
		v, err := strconv.ParseFloat(args[i], 64)
		if err != nil {
			return errors.New("usage: scroll <dy> [dx] [--at @e3|text|x,y]")
		}
		nums = append(nums, v)
	}
	if len(nums) == 0 || len(nums) > 2 {
		return errors.New("usage: scroll <dy> [dx] [--at @e3|text|x,y]")
	}
	ev := protocol.InputEvent{Type: "wheel", DY: nums[0], Precise: true}
	if len(nums) == 2 {
		ev.DX = nums[1]
	}
	if at != "" {
		t, err := c.resolve(conn, socket, at)
		if err != nil {
			return err
		}
		ev.X, ev.Y = t.x, t.y
	} else {
		windows, err := conn.Windows()
		if err != nil || len(windows) == 0 {
			return fmt.Errorf("no window: %v", err)
		}
		ev.X, ev.Y = windows[0].ContentBounds.Width/2, windows[0].ContentBounds.Height/2
	}
	return c.input(conn, ev)
}

func (c *cli) input(conn *client.Client, ev protocol.InputEvent) error {
	if err := conn.Input(0, ev); err != nil {
		return err
	}
	return conn.Idle(0, settleTimeout)
}

func (c *cli) runScenario(conn *client.Client, args []string) error {
	out := ".dev/mygo-shots"
	var file string
	for i := 0; i < len(args); i++ {
		if args[i] == "--out" && i+1 < len(args) {
			out = args[i+1]
			i++
			continue
		}
		if file != "" {
			return errors.New("usage: run <scenario.yaml> [--out dir]")
		}
		file = args[i]
	}
	if file == "" {
		return errors.New("usage: run <scenario.yaml> [--out dir]")
	}
	sc, err := scenario.Load(file)
	if err != nil {
		return err
	}
	result, err := scenario.Run(conn, sc, scenario.Options{Out: out, Log: c.stderr})
	for _, shot := range result.Shots {
		_, _ = fmt.Fprintln(c.stdout, shot.Path)
	}
	return err
}

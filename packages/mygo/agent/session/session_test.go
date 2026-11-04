package session_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/agent/client"
	"github.com/flexdinesh/servef/packages/mygo/agent/demo"
	"github.com/flexdinesh/servef/packages/mygo/agent/protocol"
	"github.com/flexdinesh/servef/packages/mygo/agent/session"
)

// start hosts a counter and connects a client over a real socket.
func start(t *testing.T) (*demo.Counter, *session.Session, *client.Client, string) {
	t.Helper()
	var counter *demo.Counter
	s := session.New(session.Options{Width: 400, Height: 360, Title: func() string { return "Counter" }, View: func(s *session.Session) func(*ui.Context) {
		counter = demo.New(s)
		return counter.View
	}})
	t.Cleanup(s.Close)
	dir, err := os.MkdirTemp("", "mygo-session")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "a.sock")
	go func() { _ = s.ListenAndServe(path) }()
	c, err := client.DialWait(path, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return counter, s, c, path
}

func node(t *testing.T, c *client.Client, label string) protocol.Node {
	t.Helper()
	tree, err := c.Tree(0)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := client.FindLabel(tree.Nodes, label)
	if !ok {
		t.Fatalf("no %q in %q", label, client.Labels(tree.Nodes))
	}
	return n
}

func TestWindowsTreeAndSocketMode(t *testing.T) {
	_, _, c, path := start(t)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v, %v", info.Mode().Perm(), err)
	}
	windows, err := c.Windows()
	if err != nil || len(windows) != 1 || windows[0].Title != "Counter" || windows[0].Kind != "content" || windows[0].ContentBounds.Width != 400 {
		t.Fatalf("windows %+v, %v", windows, err)
	}
	add := node(t, c, "Add")
	if add.Role != "text" || !add.Has("press") || add.Bounds.Width == 0 {
		t.Fatalf("add %+v", add)
	}
	// IDs last across frames that show the same texts.
	if again := node(t, c, "Add"); again.ID != add.ID {
		t.Fatalf("id changed: %v, %v", add.ID, again.ID)
	}
	tree, _ := c.Tree(0)
	repeats := 0
	for _, n := range tree.Nodes {
		if n.Label == "Repeat" {
			repeats++
			if repeats == 2 && (n.Has("press") || n.Description == "") {
				t.Fatalf("a repeated text must not be pressable: %+v", n)
			}
		}
	}
	if repeats != 2 {
		t.Fatalf("repeats %d", repeats)
	}
}

func TestActPressByIDOnly(t *testing.T) {
	counter, s, c, _ := start(t)
	add := node(t, c, "Add")
	if err := c.Act(protocol.ActParams{ID: add.ID, Action: "press"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Act(protocol.ActParams{ID: add.ID, Action: "press"}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.Do(func(*ui.Tester) { n = counter.N })
	if n != 2 {
		t.Fatalf("count %d", n)
	}
	node(t, c, "Count: 2")
	// As plugins/automation: unknown ids, unknown actions, and an id with a
	// key fail; headless nodes have no keys, and the params take no label.
	var e *protocol.Error
	for _, tc := range []struct {
		params any
		code   string
	}{
		{protocol.ActParams{ID: 12345, Action: "press"}, protocol.CodeNoElement},
		{protocol.ActParams{ID: add.ID, Action: "fly"}, protocol.CodeInvalid},
		{protocol.ActParams{ID: add.ID, Key: json.RawMessage(`"Add"`), Action: "press"}, protocol.CodeInvalid},
		{protocol.ActParams{Key: json.RawMessage(`"Add"`), Action: "press"}, protocol.CodeUnsupported},
		{map[string]string{"label": "Add", "action": "press"}, protocol.CodeInvalid},
	} {
		if err := c.Call(protocol.MethodAct, tc.params, nil); !errors.As(err, &e) || e.Code != tc.code {
			t.Fatalf("%+v: %v, want %s", tc.params, err, tc.code)
		}
	}
	if tree, _ := c.Tree(0); len(tree.Nodes) > 0 && len(tree.Nodes[0].Key) > 0 {
		t.Fatalf("headless node has a key: %s", tree.Nodes[0].Key)
	}
}

func TestInputClickTypeKeyAndSetValue(t *testing.T) {
	_, _, c, _ := start(t)
	add := node(t, c, "Add")
	x, y := add.Bounds.Center()
	if err := c.Input(0, protocol.InputEvent{Type: "click", X: x, Y: y}); err != nil {
		t.Fatal(err)
	}
	node(t, c, "Count: 1")
	if err := c.Act(protocol.ActParams{ID: node(t, c, "Name").ID, Action: "setValue", Text: "Ada"}); err != nil {
		t.Fatal(err)
	}
	node(t, c, "Hello, Ada")
	if err := c.Input(0, protocol.InputEvent{Type: "key", Key: "CmdOrCtrl+A"}, protocol.InputEvent{Type: "text", Text: "Grace"}); err != nil {
		t.Fatal(err)
	}
	node(t, c, "Hello, Grace")
	var e *protocol.Error
	if err := c.Input(0, protocol.InputEvent{Type: "key", Key: "Hyper+Q"}); !errors.As(err, &e) || e.Code != protocol.CodeInvalid {
		t.Fatalf("bad key: %v", err)
	}
}

func TestIdleWaitsForWorkOnOtherGoroutines(t *testing.T) {
	_, _, c, _ := start(t)
	if err := c.ClickText(0, "Load"); err != nil {
		t.Fatal(err)
	}
	if err := c.Idle(0, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	node(t, c, "Loaded")
}

func TestPumpsRunBeforeFrames(t *testing.T) {
	counter, s, c, _ := start(t)
	pending := 2
	s.AddPump(func() bool {
		if pending == 0 {
			return false
		}
		pending--
		counter.N += 10
		return true
	})
	if err := c.Idle(0, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	node(t, c, "Count: 20")
}

func TestContextMenuItemsAndMenu(t *testing.T) {
	_, _, c, _ := start(t)
	row := node(t, c, "Row")
	x, y := row.Bounds.Center()
	if err := c.Input(0, protocol.InputEvent{Type: "click", Button: "right", X: x, Y: y}); err != nil {
		t.Fatal(err)
	}
	item := node(t, c, "Rename")
	if item.Role != "menuItem" {
		t.Fatalf("item %+v", item)
	}
	if err := c.Menu(0, "Delete"); err != nil {
		t.Fatal(err)
	}
	node(t, c, "Chose Delete")
	var e *protocol.Error
	if err := c.Menu(0, "Delete"); !errors.As(err, &e) || e.Code != protocol.CodeUnsupported {
		t.Fatalf("no menu open: %v", err)
	}
}

func TestCaptureResizeDarkAndErrors(t *testing.T) {
	_, _, c, _ := start(t)
	light, err := c.Capture(0)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(light))
	if err != nil || img.Bounds().Dx() != 400 || img.Bounds().Dy() != 360 {
		t.Fatalf("capture %v, %v", img.Bounds(), err)
	}
	if err := c.SetDark(0, true); err != nil {
		t.Fatal(err)
	}
	dark, _ := c.Capture(0)
	if bytes.Equal(light, dark) {
		t.Fatal("dark capture equals light")
	}
	if err := c.Resize(0, 300, 200); err != nil {
		t.Fatal(err)
	}
	small, _ := c.Capture(0)
	if img, _ := png.Decode(bytes.NewReader(small)); img.Bounds().Dx() != 300 {
		t.Fatalf("resized %v", img.Bounds())
	}
	var e *protocol.Error
	for method, code := range map[string]string{"fly": protocol.CodeUnknownMethod, protocol.MethodEval: protocol.CodeUnsupported} {
		if err := c.Call(method, nil, nil); !errors.As(err, &e) || e.Code != code {
			t.Fatalf("%s: %v", method, err)
		}
	}
	if err := c.Call(protocol.MethodTree, map[string]int{"window": 7}, nil); !errors.As(err, &e) || e.Code != protocol.CodeNoWindow {
		t.Fatalf("window 7: %v", err)
	}
	if err := c.Call(protocol.MethodTree, map[string]int{"windo": 1}, nil); !errors.As(err, &e) || e.Code != protocol.CodeInvalid {
		t.Fatalf("unknown param: %v", err)
	}
}

func TestShutdownStopsAndRemovesSocket(t *testing.T) {
	_, s, c, path := start(t)
	if err := c.Shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session kept running")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("socket left behind")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestListenRefusesLiveSocketAndReplacesStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sock")
	l, err := session.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	if _, err := session.Listen(path); err == nil {
		t.Fatal("listened over a live socket")
	}
	_ = l.Close()
	// Closing a listener removes its socket; leave a stale file in its place.
	if err := os.WriteFile(filepath.Join(dir, "plain"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Listen(filepath.Join(dir, "plain")); err == nil {
		t.Fatal("listened over a plain file")
	}
}

func TestParseKey(t *testing.T) {
	for in, want := range map[string]struct {
		key  ui.Key
		mods ui.Modifiers
	}{
		"Enter": {ui.KeyEnter, 0}, "Shift+Tab": {ui.KeyTab, ui.Shift}, "CmdOrCtrl+Z": {ui.KeyZ, ui.Cmd},
		"a": {ui.KeyA, 0}, "7": {ui.Key7, 0}, "F12": {ui.KeyF12, 0}, "Ctrl+Alt+/": {ui.KeySlash, ui.Ctrl | ui.Alt},
	} {
		key, mods, err := session.ParseKey(in)
		if err != nil || key != want.key || mods != want.mods {
			t.Errorf("%s: %v %v %v", in, key, mods, err)
		}
	}
	for _, bad := range []string{"", "Ctrl+", "F13", "Hyper+A", "Nope"} {
		if _, _, err := session.ParseKey(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

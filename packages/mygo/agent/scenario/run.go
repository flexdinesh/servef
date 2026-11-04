package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/flexdinesh/servef/packages/mygo/agent/client"
	"github.com/flexdinesh/servef/packages/mygo/agent/protocol"
)

// Options configure a run.
type Options struct {
	// Out is the directory scenario directories go in.
	Out string
	// Window is the window to drive; 0 is the focused one.
	Window int
	// Log, when set, gets a line per step.
	Log io.Writer
}

// Shot is a captured shot.
type Shot struct {
	Name  string          `json:"name"`
	Path  string          `json:"path"`
	Nodes []protocol.Node `json:"nodes"`
}

// Result is a run's output.
type Result struct {
	Dir   string `json:"dir"`
	Shots []Shot `json:"shots"`
}

const (
	waitTimeout   = 5 * time.Second
	settleTimeout = 10 * time.Second
)

// Run drives c through sc and writes its shots to <Out>/<sc.Name>/. It
// runs against built apps too: their idle waits for frames only, so steps
// that follow fetches should wait for a text; size, dark, resize, and dark
// steps are skipped where the app lacks them (logged).
// Shots go to <Out>/<sc.Name>/:
// <shot>.png per shot, tree.json (each shot's elements), and scenario.json
// (sc, for the web capture).
func Run(c *client.Client, sc Scenario, opts Options) (Result, error) {
	if err := sc.Validate(); err != nil {
		return Result{}, err
	}
	dir := filepath.Join(opts.Out, sc.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	result := Result{Dir: dir}
	w := opts.Window
	logf := func(format string, args ...any) {
		if opts.Log != nil {
			_, _ = fmt.Fprintf(opts.Log, format+"\n", args...)
		}
	}
	if sc.Size != nil {
		if err := headless(c.Resize(w, sc.Size.Width, sc.Size.Height), logf, sc.Name, "size"); err != nil {
			return result, fmt.Errorf("scenario %s: size: %w", sc.Name, err)
		}
	}
	if sc.Dark {
		if err := headless(c.SetDark(w, true), logf, sc.Name, "dark"); err != nil {
			return result, fmt.Errorf("scenario %s: dark: %w", sc.Name, err)
		}
	}
	if err := c.Idle(w, settleTimeout); err != nil {
		return result, fmt.Errorf("scenario %s: settle: %w", sc.Name, err)
	}
	for i, step := range sc.Steps {
		logf("%s %d %s", sc.Name, i+1, step.Kind())
		shot, err := do(c, w, step, logf)
		if err != nil {
			return result, fmt.Errorf("scenario %s: step %d (%s): %w", sc.Name, i+1, step.Kind(), err)
		}
		if shot == nil {
			continue
		}
		path := filepath.Join(dir, *step.Shot+".png")
		if err := os.WriteFile(path, shot.png, 0o644); err != nil {
			return result, err
		}
		result.Shots = append(result.Shots, Shot{Name: *step.Shot, Path: path, Nodes: shot.nodes})
		logf("%s wrote %s", sc.Name, path)
	}
	if err := writeJSON(filepath.Join(dir, "tree.json"), result.Shots); err != nil {
		return result, err
	}
	return result, writeJSON(filepath.Join(dir, "scenario.json"), sc)
}

// headless passes err through, but skips a headless-only method a built
// app answers with unknown_method (resize, setDark): such scenarios still
// run there, at the window's own size and appearance, and say so.
func headless(err error, logf func(string, ...any), scenario, what string) error {
	var e *protocol.Error
	if errors.As(err, &e) && e.Code == protocol.CodeUnknownMethod {
		logf("%s %s skipped: the app has no %s (a built app; headless sessions do)", scenario, what, what)
		return nil
	}
	return err
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

type captured struct {
	png   []byte
	nodes []protocol.Node
}

// do performs one step; a shot step returns its capture.
func do(c *client.Client, w int, step Step, logf func(string, ...any)) (*captured, error) {
	settle := func() error { return c.Idle(w, settleTimeout) }
	switch {
	case step.Click != nil:
		k := step.Click
		if k.Label != "" && k.Button == "" && k.Clicks <= 1 && len(k.Mods) == 0 {
			if err := c.ClickText(w, k.Label); err != nil {
				return nil, err
			}
			return nil, settle()
		}
		x, y := k.X, k.Y
		if k.Label != "" {
			node, err := find(c, w, k.Label)
			if err != nil {
				return nil, err
			}
			x, y = node.Bounds.Center()
		}
		if err := c.Input(w, protocol.InputEvent{Type: "click", X: x, Y: y, Button: k.Button, Clicks: k.Clicks, Mods: k.Mods}); err != nil {
			return nil, err
		}
		return nil, settle()
	case step.Fill != nil:
		node, err := find(c, w, step.Fill.Label)
		if err != nil {
			return nil, err
		}
		if err := c.Act(protocol.ActParams{Window: w, ID: node.ID, Action: "setValue", Text: step.Fill.Value}); err != nil {
			return nil, err
		}
		return nil, settle()
	case step.Type != nil:
		if err := c.Input(w, protocol.InputEvent{Type: "text", Text: *step.Type}); err != nil {
			return nil, err
		}
		return nil, settle()
	case step.Key != nil:
		if err := c.Input(w, protocol.InputEvent{Type: "key", Key: *step.Key}); err != nil {
			return nil, err
		}
		return nil, settle()
	case step.Scroll != nil:
		x, y := step.Scroll.X, step.Scroll.Y
		if step.Scroll.Label != "" {
			node, err := find(c, w, step.Scroll.Label)
			if err != nil {
				return nil, err
			}
			x, y = node.Bounds.Center()
		} else if x == 0 && y == 0 {
			windows, err := c.Windows()
			if err != nil || len(windows) == 0 {
				return nil, fmt.Errorf("no window to scroll: %v", err)
			}
			x, y = windows[0].ContentBounds.Width/2, windows[0].ContentBounds.Height/2
		}
		if err := c.Input(w, protocol.InputEvent{Type: "wheel", X: x, Y: y, DX: step.Scroll.DX, DY: step.Scroll.DY, Precise: true}); err != nil {
			return nil, err
		}
		return nil, settle()
	case step.Resize != nil:
		if err := headless(c.Resize(w, step.Resize.Width, step.Resize.Height), logf, "", "resize"); err != nil {
			return nil, err
		}
		return nil, settle()
	case step.Dark != nil:
		if err := headless(c.SetDark(w, *step.Dark), logf, "", "dark"); err != nil {
			return nil, err
		}
		return nil, settle()
	case step.Wait != nil:
		if step.Wait.MS > 0 {
			time.Sleep(time.Duration(step.Wait.MS) * time.Millisecond)
			return nil, settle()
		}
		timeout := waitTimeout
		if step.Wait.Timeout > 0 {
			timeout = time.Duration(step.Wait.Timeout) * time.Millisecond
		}
		return nil, c.WaitText(w, step.Wait.Text, timeout)
	case step.Settle != nil:
		timeout := settleTimeout
		if step.Settle.Timeout > 0 {
			timeout = time.Duration(step.Settle.Timeout) * time.Millisecond
		}
		return nil, c.Idle(w, timeout)
	case len(step.Expect) > 0:
		tree, err := c.Tree(w)
		if err != nil {
			return nil, err
		}
		for _, text := range step.Expect {
			if _, ok := client.FindLabel(tree.Nodes, text); !ok {
				return nil, fmt.Errorf("%w %q; labels %q", client.ErrNotFound, text, client.Labels(tree.Nodes))
			}
		}
		return nil, nil
	case step.Shot != nil:
		if err := settle(); err != nil {
			return nil, err
		}
		png, err := c.Capture(w)
		if err != nil {
			return nil, err
		}
		tree, err := c.Tree(w)
		if err != nil {
			return nil, err
		}
		return &captured{png: png, nodes: tree.Nodes}, nil
	}
	return nil, fmt.Errorf("step does nothing")
}

func find(c *client.Client, w int, label string) (protocol.Node, error) {
	tree, err := c.Tree(w)
	if err != nil {
		return protocol.Node{}, err
	}
	node, ok := client.FindLabel(tree.Nodes, label)
	if !ok {
		return protocol.Node{}, fmt.Errorf("%w %q; labels %q", client.ErrNotFound, label, client.Labels(tree.Nodes))
	}
	return node, nil
}

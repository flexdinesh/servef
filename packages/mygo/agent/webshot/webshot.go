// Package webshot captures a scenario's web shots with agent-browser, at
// the scenario's size and appearance, so they compare with the native
// shots: <out>/<scenario>/web/<shot>.png.
package webshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/flexdinesh/servef/packages/mygo/agent/scenario"
)

// Run runs a command, as exec does; tests replace it.
type Run func(ctx context.Context, name string, args ...string) ([]byte, error)

// Exec runs a command and returns its combined output.
func Exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Options configure a capture.
type Options struct {
	// Base is the web app's origin, as http://localhost:5173.
	Base string
	// Out is the directory scenario directories go in.
	Out string
	// Binary is agent-browser's path; "agent-browser" when empty.
	Binary string
	// Session is agent-browser's session name; "mygo-webshot" when empty.
	Session string
	// Size is used when the scenario sets none; 1100×700 when nil.
	Size *scenario.Size
	Run  Run
}

// Capture writes each web shot of sc and returns their paths by name.
func Capture(ctx context.Context, sc scenario.Scenario, opts Options) (map[string]string, error) {
	if sc.Web == nil || len(sc.Web.Shots) == 0 {
		return map[string]string{}, nil
	}
	if opts.Binary == "" {
		opts.Binary = "agent-browser"
	}
	if opts.Session == "" {
		opts.Session = "mygo-webshot"
	}
	if opts.Run == nil {
		opts.Run = Exec
	}
	size := scenario.Size{Width: 1100, Height: 700}
	if opts.Size != nil {
		size = *opts.Size
	}
	if sc.Size != nil {
		size = *sc.Size
	}
	// agent-browser reads a relative path that starts with a dot as a
	// selector.
	dir, err := filepath.Abs(filepath.Join(opts.Out, sc.Name, "web"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ab := func(args ...string) error {
		out, err := opts.Run(ctx, opts.Binary, append([]string{"--session", opts.Session}, args...)...)
		if err != nil {
			return fmt.Errorf("agent-browser %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	paths := map[string]string{}
	for _, name := range sc.Shots() {
		web, ok := sc.Web.Shots[name]
		if !ok {
			continue
		}
		dark := sc.Dark
		for _, step := range sc.Steps {
			if step.Dark != nil {
				dark = *step.Dark
			}
			if step.Shot != nil && *step.Shot == name {
				break
			}
		}
		media := "light"
		if dark {
			media = "dark"
		}
		base := strings.TrimSuffix(opts.Base, "/")
		load := [][]string{
			{"set", "viewport", strconv.Itoa(size.Width), strconv.Itoa(size.Height)},
			{"set", "media", media},
			{"open", base + web.Path},
		}
		// Each shot starts from empty storage (no memory of earlier shots),
		// plus the scenario's entries. Pages that hold event streams never
		// reach networkidle: steps wait for texts instead.
		load = append(load, []string{"eval", storageScript(sc.Web.Storage, media)}, []string{"reload"}, []string{"wait", "--load", "load"})
		for _, args := range load {
			if err := ab(args...); err != nil {
				return paths, err
			}
		}
		for i, step := range web.Steps {
			for _, args := range Commands(step) {
				if err := ab(args...); err != nil {
					return paths, fmt.Errorf("web %s step %d: %w", name, i+1, err)
				}
			}
		}
		path := filepath.Join(dir, name+".png")
		if err := ab("wait", strconv.Itoa(settleMS)); err != nil {
			return paths, err
		}
		if err := ab("screenshot", path); err != nil {
			return paths, err
		}
		paths[name] = path
	}
	return paths, nil
}

// settleMS is the pause before a screenshot, for transitions to finish.
const settleMS = 500

// Commands returns the agent-browser commands of a web step.
func Commands(step scenario.Step) [][]string {
	ms := func(n int) string { return strconv.Itoa(n) }
	switch {
	case step.Click != nil:
		k := step.Click
		if k.Label != "" {
			action := "click"
			if k.Clicks > 1 {
				action = "dblclick"
			}
			by := k.By
			if by == "" {
				by = "text"
			}
			if by == "css" {
				return [][]string{{action, k.Label}}
			}
			return [][]string{{"find", by, k.Label, action}}
		}
		return [][]string{{"mouse", "move", ms(int(k.X)), ms(int(k.Y))}, {"mouse", "down"}, {"mouse", "up"}}
	case step.Fill != nil:
		return [][]string{{"find", "label", step.Fill.Label, "fill", step.Fill.Value}}
	case step.Type != nil:
		return [][]string{{"keyboard", "type", *step.Type}}
	case step.Key != nil:
		return [][]string{{"press", webKey(*step.Key)}}
	case step.Scroll != nil:
		dir, px := "down", step.Scroll.DY
		if px < 0 {
			dir, px = "up", -px
		}
		return [][]string{{"scroll", dir, ms(int(px))}}
	case step.Resize != nil:
		return [][]string{{"set", "viewport", ms(step.Resize.Width), ms(step.Resize.Height)}}
	case step.Dark != nil:
		if *step.Dark {
			return [][]string{{"set", "media", "dark"}}
		}
		return [][]string{{"set", "media", "light"}}
	case step.Wait != nil:
		if step.Wait.MS > 0 {
			return [][]string{{"wait", ms(step.Wait.MS)}}
		}
		return [][]string{{"wait", "--text", step.Wait.Text}}
	case step.Settle != nil:
		return [][]string{{"wait", strconv.Itoa(settleMS)}}
	case len(step.Expect) > 0:
		var out [][]string
		for _, text := range step.Expect {
			out = append(out, []string{"wait", "--text", text})
		}
		return out
	}
	return nil
}

// storageScript clears local and session storage and sets the entries,
// with ${theme} replaced by theme, in keys' order.
func storageScript(storage map[string]string, theme string) string {
	keys := make([]string, 0, len(storage))
	for k := range storage {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("localStorage.clear();sessionStorage.clear();")
	for _, k := range keys {
		key, _ := json.Marshal(k)
		value, _ := json.Marshal(strings.ReplaceAll(storage[k], "${theme}", theme))
		fmt.Fprintf(&b, "localStorage.setItem(%s, %s);", key, value)
	}
	return b.String()
}

// webKey turns an accelerator into a Playwright key: CmdOrCtrl is Control
// (Meta on macOS, where browsers' shortcuts take it).
func webKey(key string) string {
	parts := strings.Split(key, "+")
	for i, p := range parts[:len(parts)-1] {
		switch strings.ToLower(p) {
		case "cmdorctrl", "commandorcontrol":
			parts[i] = "ControlOrMeta"
		case "ctrl", "control":
			parts[i] = "Control"
		case "cmd", "command", "super", "meta":
			parts[i] = "Meta"
		case "alt", "option":
			parts[i] = "Alt"
		case "shift":
			parts[i] = "Shift"
		}
	}
	return strings.Join(parts, "+")
}

// Command mygo-compare puts web and native screenshots side by side.
//
//	mygo-compare compose [-diff] [-labels web,native] -out out.png web.png native.png
//	mygo-compare dir [-diff] <out>/<scenario>    # web/<shot>.png | <shot>.png -> compare/<shot>.png
//	mygo-compare web -base http://localhost:5173 -out <out> scenario.yaml
//
// web captures a scenario's web shots with agent-browser at the scenario's
// size and appearance; dir composes every shot that has both sides.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flexdinesh/servef/packages/mygo/agent/compare"
	"github.com/flexdinesh/servef/packages/mygo/agent/scenario"
	"github.com/flexdinesh/servef/packages/mygo/agent/webshot"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mygo-compare:", err)
		os.Exit(1)
	}
}

const usage = `usage:
  mygo-compare compose [-diff] [-labels web,native] -out out.png left.png right.png
  mygo-compare dir [-diff] <out>/<scenario>
  mygo-compare web -base URL -out <out> scenario.yaml`

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "compose":
		fs := flag.NewFlagSet("compose", flag.ContinueOnError)
		out := fs.String("out", "", "output PNG")
		diff := fs.Bool("diff", false, "add a heat map of differing pixels")
		labels := fs.String("labels", "web,native", "panel labels, comma separated")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *out == "" || fs.NArg() != 2 {
			return errors.New(usage)
		}
		names := strings.SplitN(*labels, ",", 2)
		if len(names) != 2 {
			return errors.New("-labels takes two labels")
		}
		ratio, err := composeFiles(fs.Arg(0), fs.Arg(1), names[0], names[1], *out, *diff)
		if err != nil {
			return err
		}
		report(stdout, *out, ratio, *diff)
		return nil
	case "dir":
		fs := flag.NewFlagSet("dir", flag.ContinueOnError)
		diff := fs.Bool("diff", false, "add a heat map of differing pixels")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New(usage)
		}
		return composeDir(fs.Arg(0), *diff, stdout)
	case "web":
		fs := flag.NewFlagSet("web", flag.ContinueOnError)
		base := fs.String("base", "", "web app origin")
		out := fs.String("out", "", "output directory")
		binary := fs.String("agent-browser", "agent-browser", "agent-browser binary")
		session := fs.String("session", "mygo-webshot", "agent-browser session")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *base == "" || *out == "" || fs.NArg() != 1 {
			return errors.New(usage)
		}
		sc, err := scenario.Load(fs.Arg(0))
		if err != nil {
			return err
		}
		paths, err := webshot.Capture(context.Background(), sc, webshot.Options{Base: *base, Out: *out, Binary: *binary, Session: *session})
		names := make([]string, 0, len(paths))
		for name := range paths {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			_, _ = fmt.Fprintln(stdout, paths[name])
		}
		return err
	}
	return errors.New(usage)
}

func report(w io.Writer, path string, ratio float64, diff bool) {
	if diff {
		_, _ = fmt.Fprintf(w, "%s\t%.1f%% of pixels differ\n", path, ratio*100)
		return
	}
	_, _ = fmt.Fprintln(w, path)
}

func composeDir(dir string, diff bool, stdout io.Writer) error {
	webs, err := filepath.Glob(filepath.Join(dir, "web", "*.png"))
	if err != nil {
		return err
	}
	if len(webs) == 0 {
		return fmt.Errorf("no web shots in %s", filepath.Join(dir, "web"))
	}
	if err := os.MkdirAll(filepath.Join(dir, "compare"), 0o755); err != nil {
		return err
	}
	composed := 0
	for _, web := range webs {
		name := filepath.Base(web)
		native := filepath.Join(dir, name)
		if _, err := os.Stat(native); err != nil {
			continue
		}
		out := filepath.Join(dir, "compare", name)
		ratio, err := composeFiles(web, native, "web", "native", out, diff)
		if err != nil {
			return err
		}
		report(stdout, out, ratio, diff)
		composed++
	}
	if composed == 0 {
		return fmt.Errorf("no shot in %s has both a web and a native side", dir)
	}
	return nil
}

func composeFiles(left, right, leftLabel, rightLabel, out string, diff bool) (float64, error) {
	a, err := readPNG(left)
	if err != nil {
		return 0, err
	}
	b, err := readPNG(right)
	if err != nil {
		return 0, err
	}
	panels := []compare.Panel{{Label: leftLabel, Image: a}, {Label: rightLabel, Image: b}}
	ratio := 0.0
	if diff {
		var heat image.Image
		heat, ratio = compare.Diff(a, b)
		panels = append(panels, compare.Panel{Label: fmt.Sprintf("diff %.1f%%", ratio*100), Image: heat})
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(out)
	if err != nil {
		return 0, err
	}
	if err := png.Encode(f, compare.Compose(panels)); err != nil {
		_ = f.Close()
		return 0, err
	}
	return ratio, f.Close()
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

package scenario_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/packages/mygo/agent/client"
	"github.com/flexdinesh/servef/packages/mygo/agent/internal/agenttest"
	"github.com/flexdinesh/servef/packages/mygo/agent/protocol"
	"github.com/flexdinesh/servef/packages/mygo/agent/scenario"
)

const counter = `
size: {width: 420, height: 380}
steps:
  - shot: start
  - click: Add
  - click: Add
  - expect: "Count: 2"
  - fill: {label: Name, value: Ada}
  - wait: Hello, Ada
  - click: Load
  - wait: {text: Loaded, timeout: 3000}
  - click: {label: Row, button: right}
  - click: Delete
  - expect: [Chose Delete]
  - dark: true
  - settle: true
  - shot: dark
web:
  shots:
    dark:
      path: /counter
      steps: [{click: Add}, {wait: "Count: 1"}]
`

func TestRunWritesShotsTreeAndScenario(t *testing.T) {
	sc, err := scenario.Parse([]byte(counter), "counter")
	if err != nil {
		t.Fatal(err)
	}
	_, c := agenttest.Start(t, 300, 200)
	out := t.TempDir()
	result, err := scenario.Run(c, sc, scenario.Options{Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Shots) != 2 || result.Shots[0].Name != "start" || result.Shots[1].Name != "dark" {
		t.Fatalf("shots %+v", result.Shots)
	}
	start, err := os.ReadFile(filepath.Join(out, "counter", "start.png"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(start))
	if err != nil || img.Bounds().Dx() != 420 || img.Bounds().Dy() != 380 {
		t.Fatalf("start.png %v %v", img.Bounds(), err)
	}
	dark, _ := os.ReadFile(filepath.Join(out, "counter", "dark.png"))
	if bytes.Equal(start, dark) {
		t.Fatal("dark shot equals the start shot")
	}
	var shots []scenario.Shot
	b, _ := os.ReadFile(filepath.Join(out, "counter", "tree.json"))
	if err := json.Unmarshal(b, &shots); err != nil || len(shots) != 2 {
		t.Fatalf("tree.json %s %v", b, err)
	}
	labels := ""
	for _, n := range shots[1].Nodes {
		labels += n.Label + "|"
	}
	if !strings.Contains(labels, "Count: 2|") || !strings.Contains(labels, "Hello, Ada|") || !strings.Contains(labels, "Chose Delete|") {
		t.Fatalf("dark shot tree %s", labels)
	}
	if _, err := os.Stat(filepath.Join(out, "counter", "scenario.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunReportsTheFailingStep(t *testing.T) {
	sc, err := scenario.Parse([]byte("steps:\n  - click: Nope\n  - shot: x\n"), "bad")
	if err != nil {
		t.Fatal(err)
	}
	_, c := agenttest.Start(t, 300, 200)
	_, err = scenario.Run(c, sc, scenario.Options{Out: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "step 1 (click)") || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("err %v", err)
	}
}

func TestParseRejectsInvalidScenarios(t *testing.T) {
	for name, src := range map[string]string{
		"two actions":   "steps:\n  - {click: A, type: b}\n  - shot: x\n",
		"unknown step":  "steps:\n  - jump: A\n  - shot: x\n",
		"unknown field": "steps:\n  - click: {label: A, colour: red}\n  - shot: x\n",
		"no shot":       "steps:\n  - click: A\n",
		"dup shot":      "steps:\n  - shot: x\n  - shot: x\n",
		"bad shot":      "steps:\n  - shot: X Y\n",
		"empty wait":    "steps:\n  - wait: {timeout: 3}\n  - shot: x\n",
		"web orphan":    "steps:\n  - shot: x\nweb:\n  shots: {y: {path: /}}\n",
		"web path":      "steps:\n  - shot: x\nweb:\n  shots: {x: {path: nope}}\n",
		"web shot":      "steps:\n  - shot: x\nweb:\n  shots: {x: {path: /, steps: [{shot: z}]}}\n",
		"bad locator":   "steps:\n  - shot: x\nweb:\n  shots: {x: {path: /, steps: [{click: {label: A, by: xpath}}]}}\n",
		"native by":     "steps:\n  - click: {label: A, by: label}\n  - shot: x\n",
		"bad size":      "size: {width: 0, height: 3}\nsteps:\n  - shot: x\n",
		"settle false":  "steps:\n  - settle: false\n  - shot: x\n",
	} {
		if _, err := scenario.Parse([]byte(src), "t"); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestLoadNamesFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my-flow.yaml")
	if err := os.WriteFile(path, []byte("steps:\n  - shot: a\n  - shot: b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := scenario.Load(path)
	if err != nil || sc.Name != "my-flow" || strings.Join(sc.Shots(), ",") != "a,b" {
		t.Fatalf("%+v %v", sc, err)
	}
}

// builtApp serves the protocol as plugins/automation does in a built app:
// it forwards every request to the session at socket but answers the
// headless-only methods with unknown_method.
func builtApp(t *testing.T, socket string) *client.Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "mygo-built")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "app.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				upstream, err := net.Dial("unix", socket)
				if err != nil {
					return
				}
				defer upstream.Close()
				in, up := bufio.NewReader(conn), bufio.NewReader(upstream)
				for {
					line, err := in.ReadBytes('\n')
					if err != nil {
						return
					}
					var req protocol.Request
					_ = json.Unmarshal(line, &req)
					if req.Method == protocol.MethodResize || req.Method == protocol.MethodSetDark {
						b, _ := json.Marshal(protocol.Response{ID: req.ID, Error: protocol.Errorf(protocol.CodeUnknownMethod, "no method %q", req.Method)})
						_, _ = conn.Write(append(b, '\n'))
						continue
					}
					if _, err := upstream.Write(line); err != nil {
						return
					}
					answer, err := up.ReadBytes('\n')
					if err != nil {
						return
					}
					_, _ = conn.Write(answer)
				}
			}()
		}
	}()
	c, err := client.DialWait(path, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestRunSkipsHeadlessOnlyMethodsOnBuiltApps(t *testing.T) {
	sc, err := scenario.Parse([]byte(`
size: {width: 640, height: 480}
dark: true
steps:
  - click: Add
  - resize: {width: 500, height: 400}
  - dark: false
  - expect: "Count: 1"
  - shot: built
`), "built")
	if err != nil {
		t.Fatal(err)
	}
	socket, _ := agenttest.Start(t, 300, 200)
	app := builtApp(t, socket)
	var log strings.Builder
	result, err := scenario.Run(app, sc, scenario.Options{Out: t.TempDir(), Log: &log})
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if strings.Count(log.String(), "skipped") != 4 {
		t.Fatalf("want 4 skips (size, dark, resize, dark):\n%s", log.String())
	}
	b, _ := os.ReadFile(result.Shots[0].Path)
	if img, err := png.Decode(bytes.NewReader(b)); err != nil || img.Bounds().Dx() != 300 {
		t.Fatalf("the built app keeps its own size: %v %v", img.Bounds(), err)
	}
	// Other failures still fail.
	bad, _ := scenario.Parse([]byte("steps:\n  - resize: {width: 1, height: 1}\n  - click: Nope\n  - shot: x\n"), "bad")
	if _, err := scenario.Run(app, bad, scenario.Options{Out: t.TempDir()}); err == nil {
		t.Fatal("a missing element passed")
	}
}

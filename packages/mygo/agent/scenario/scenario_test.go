package scenario_test

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/packages/mygo/agent/internal/agenttest"
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

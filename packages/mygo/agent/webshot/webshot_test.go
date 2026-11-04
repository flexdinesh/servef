package webshot_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/packages/mygo/agent/scenario"
	"github.com/flexdinesh/servef/packages/mygo/agent/webshot"
)

func TestCaptureDrivesAgentBrowserPerWebShot(t *testing.T) {
	sc, err := scenario.Parse([]byte(`
size: {width: 800, height: 500}
steps:
  - shot: light
  - dark: true
  - shot: dark
  - shot: native-only
web:
  storage: {app-theme: "${theme}"}
  shots:
    light: {path: "/a?x=1", steps: [{click: Add}, {click: {label: Close, by: label}}, {click: {label: "#go", by: css}}, {key: CmdOrCtrl+K}, {fill: {label: Name, value: Ada}}, {wait: Done}, {scroll: {dy: -40}}]}
    dark: {path: /b}
`), "flow")
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if args[2] == "screenshot" {
			return nil, os.WriteFile(args[3], []byte("png"), 0o644)
		}
		return nil, nil
	}
	out := t.TempDir()
	paths, err := webshot.Capture(context.Background(), sc, webshot.Options{Base: "http://localhost:5173/", Out: out, Binary: "ab", Run: run})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths["light"] != filepath.Join(out, "flow", "web", "light.png") {
		t.Fatalf("paths %v", paths)
	}
	want := []string{
		"ab --session mygo-webshot set viewport 800 500",
		"ab --session mygo-webshot set media light",
		"ab --session mygo-webshot open http://localhost:5173/a?x=1",
		`ab --session mygo-webshot eval localStorage.clear();sessionStorage.clear();localStorage.setItem("app-theme", "light");`,
		"ab --session mygo-webshot reload",
		"ab --session mygo-webshot wait --load load",
		"ab --session mygo-webshot find text Add click",
		"ab --session mygo-webshot find label Close click",
		"ab --session mygo-webshot click #go",
		"ab --session mygo-webshot press ControlOrMeta+K",
		"ab --session mygo-webshot find label Name fill Ada",
		"ab --session mygo-webshot wait --text Done",
		"ab --session mygo-webshot scroll up 40",
		"ab --session mygo-webshot wait 500",
		"ab --session mygo-webshot screenshot " + paths["light"],
		"ab --session mygo-webshot set viewport 800 500",
		"ab --session mygo-webshot set media dark",
		"ab --session mygo-webshot open http://localhost:5173/b",
		`ab --session mygo-webshot eval localStorage.clear();sessionStorage.clear();localStorage.setItem("app-theme", "dark");`,
		"ab --session mygo-webshot reload",
		"ab --session mygo-webshot wait --load load",
		"ab --session mygo-webshot wait 500",
		"ab --session mygo-webshot screenshot " + paths["dark"],
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls\n%s\nwant\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

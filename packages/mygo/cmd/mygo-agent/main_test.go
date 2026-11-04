package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary be the host open starts: the demo counter.
func TestMain(m *testing.M) {
	if os.Getenv("MYGO_AGENT_TEST_HOST") == "1" {
		c := &cli{stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv}
		if err := c.run([]string{"demo"}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestOpenSnapshotClickScreenshotClose(t *testing.T) {
	dir, err := os.MkdirTemp("", "mga")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("MYGO_AGENT_TEST_HOST", "1")
	env := map[string]string{"MYGO_AGENT_DIR": dir}
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		c := &cli{stdout: &out, stderr: &out, getenv: func(k string) string { return env[k] }}
		if err := c.run(args); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if out := run("open", "--", self); !strings.Contains(out, "opened "+filepath.Join(dir, "default.sock")) {
		t.Fatalf("open: %s", out)
	}
	t.Cleanup(func() {
		c := &cli{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, getenv: func(k string) string { return env[k] }}
		_ = c.run([]string{"close"})
	})

	snap := run("snapshot", "-i")
	add := regexp.MustCompile(`(?m)^(@e\d+) text "Add" \[\d+,\d+ \d+x\d+\]$`).FindStringSubmatch(snap)
	if add == nil {
		t.Fatalf("no Add ref in\n%s", snap)
	}
	run("click", add[1])
	run("click", "Add")
	if snap := run("snapshot"); !strings.Contains(snap, `"Count: 2"`) {
		t.Fatalf("after clicks\n%s", snap)
	}
	run("fill", "Name", "Ada")
	run("key", "CmdOrCtrl+A")
	run("type", "Grace")
	if snap := run("snapshot"); !strings.Contains(snap, `"Hello, Grace"`) {
		t.Fatalf("after typing\n%s", snap)
	}
	run("click", "Row", "--right")
	run("menu", "Rename")
	run("scroll", "40", "--at", "10,10")
	run("resize", "300", "240")
	run("dark", "on")
	run("idle", "--timeout", "2000")
	if out := run("windows"); !strings.Contains(out, `"title":"Counter"`) {
		t.Fatalf("windows %s", out)
	}
	shot := filepath.Join(dir, "shots", "a.png")
	run("screenshot", shot)
	b, err := os.ReadFile(shot)
	if err != nil {
		t.Fatal(err)
	}
	if img, err := png.Decode(bytes.NewReader(b)); err != nil || img.Bounds().Dx() != 300 {
		t.Fatalf("screenshot %v %v", img.Bounds(), err)
	}
	if snap := run("snapshot"); !strings.Contains(snap, `"Chose Rename"`) {
		t.Fatalf("menu not chosen\n%s", snap)
	}

	scenarioFile := filepath.Join(dir, "flow.yaml")
	if err := os.WriteFile(scenarioFile, []byte("steps:\n  - click: Add\n  - shot: after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := run("run", scenarioFile, "--out", filepath.Join(dir, "out")); !strings.Contains(out, filepath.Join(dir, "out", "flow", "after.png")) {
		t.Fatalf("run %s", out)
	}

	run("close")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "default.sock")); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket left after close")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var out bytes.Buffer
	c := &cli{stdout: &out, stderr: &out, getenv: func(k string) string { return env[k] }}
	if err := c.run([]string{"snapshot"}); err == nil {
		t.Fatal("snapshot after close succeeded")
	}
}

func TestUnknownRefAndUsage(t *testing.T) {
	var out bytes.Buffer
	c := &cli{stdout: &out, stderr: &out, getenv: func(string) string { return "" }}
	if err := c.run(nil); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("no args: %v", err)
	}
	if err := c.run([]string{"--bogus"}); err == nil {
		t.Fatal("bogus flag accepted")
	}
}

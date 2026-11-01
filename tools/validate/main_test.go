//go:build linux || darwin

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "--initial-branch=main"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}} {
		if _, err := git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func put(t *testing.T, root, name, contents string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}
func commit(t *testing.T, root, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "."}, {"-c", "commit.gpgsign=false", "commit", "-qm", msg}} {
		if _, err := git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
}
func snap(t *testing.T, root string) snapshot {
	t.Helper()
	s, err := readSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func key(t *testing.T, s snapshot, group string) string {
	t.Helper()
	_, key, err := manifest(s, group, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestSnapshotIgnoresUntrackedAndPreservesModes(t *testing.T) {
	root := repository(t)
	put(t, root, "web/source.ts", "original")
	put(t, root, "script", "script")
	if err := os.Chmod(filepath.Join(root, "script"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("web/source.ts", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	commit(t, root, "initial")
	put(t, root, ".env", "PERSONAL_SECRET=hidden")
	put(t, root, "node_modules/poison", "bad")
	s := snap(t, root)
	out := t.TempDir()
	if err := materialize(out, s.Entries); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("untracked environment copied")
	}
	info, err := os.Stat(filepath.Join(out, "script"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("executable lost: %v %v", info, err)
	}
	target, err := os.Readlink(filepath.Join(out, "link"))
	if err != nil || target != "web/source.ts" {
		t.Fatalf("link changed: %q %v", target, err)
	}
	put(t, root, "web/source.ts", "dirty")
	if _, err := readSnapshot(root); err == nil {
		t.Fatal("dirty tracked source accepted")
	}
}

func TestHistoryOnlyChangesReuseButRelevantInputsInvalidate(t *testing.T) {
	root := repository(t)
	put(t, root, "web/a.ts", "a")
	put(t, root, "site/page.mdx", "site")
	put(t, root, "pnpm-lock.yaml", "lock")
	put(t, root, "mise.toml", "tasks")
	commit(t, root, "initial")
	original := snap(t, root)
	_, err := git(root, "-c", "commit.gpgsign=false", "commit", "--amend", "-qm", "history only")
	if err != nil {
		t.Fatal(err)
	}
	history := snap(t, root)
	if history.Head == original.Head {
		t.Fatal("HEAD unchanged")
	}
	for _, g := range groups {
		if key(t, original, g) != key(t, history, g) {
			t.Fatalf("history invalidated %s", g)
		}
	}
	put(t, root, "site/page.mdx", "changed site")
	commit(t, root, "site")
	site := snap(t, root)
	if key(t, site, "docs") == key(t, original, "docs") {
		t.Fatal("site did not invalidate docs")
	}
	if key(t, site, "frontend") != key(t, original, "frontend") {
		t.Fatal("site invalidated frontend")
	}
	for _, name := range []string{"pnpm-lock.yaml", "mise.toml"} {
		before := snap(t, root)
		put(t, root, name, "changed "+name)
		commit(t, root, name)
		after := snap(t, root)
		for _, g := range groups {
			if key(t, before, g) == key(t, after, g) {
				t.Fatalf("%s did not invalidate %s", name, g)
			}
		}
	}
}

func TestPartialProofCannotQualifyFullGroup(t *testing.T) {
	s := snapshot{Entries: []entry{{Path: "web/a.ts", Mode: "100644", Blob: "abc"}}}
	_, full, _ := manifest(s, "frontend", "", nil)
	_, partial, _ := manifest(s, "frontend", "_validate:test:web", []string{"filter"})
	if full == partial {
		t.Fatal("partial and full identities match")
	}
	a := app{Cache: t.TempDir()}
	if err := writeJSON(filepath.Join(a.Cache, "receipts", partial+".json"), receipt{policy, partial, "head", "run", time.Now(), false}); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.receipt(full); ok {
		t.Fatal("partial receipt qualifies full group")
	}
	put(t, a.Cache, "receipts/"+full+".json", "{}")
	if _, ok := a.receipt(full); ok {
		t.Fatal("malformed receipt accepted")
	}
}

func TestCapacityCancellationDoesNotReleaseOtherRuns(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireSlot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := acquireSlot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = acquireSlot(ctx, dir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third invocation admitted: %v", err)
	}
	second.Close()
	next, err := acquireSlot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestCancellationStopsOwnedProcessAndPreservesOtherProcess(t *testing.T) {
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var output strings.Builder
	if err := command(ctx, t.TempDir(), os.Environ(), &output, "sleep", "30"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation failed: %v", err)
	}
	if err := other.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("other process stopped: %v", err)
	}
}

func TestPruneRetainsActiveAndReceipts(t *testing.T) {
	a := app{Cache: t.TempDir()}
	old := time.Now().Add(-31 * 24 * time.Hour)
	for _, r := range []record{{ID: "active", Status: "running"}, {ID: "old", Finished: &old, Status: "success"}} {
		if err := writeJSON(filepath.Join(a.Cache, "runs", r.ID, "run.json"), r); err != nil {
			t.Fatal(err)
		}
	}
	put(t, a.Cache, "receipts/proof.json", "proof")
	if err := a.prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Cache, "runs", "old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old run retained")
	}
	for _, path := range []string{"runs/active/run.json", "receipts/proof.json"} {
		if _, err := os.Stat(filepath.Join(a.Cache, path)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSlotSurvivesSupervisorDescriptorClose(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireSlot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := acquireSlot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- commandWithFiles(ctx, t.TempDir(), os.Environ(), io.Discard, []*os.File{first, writer}, "sh", "-c", "printf ready >&4; exec sleep 30")
	}()
	ready := make([]byte, 5)
	if _, err = io.ReadFull(reader, ready); err != nil {
		t.Fatal(err)
	}
	first.Close()
	wait, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if _, err = acquireSlot(wait, dir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("inherited lock released too early: %v", err)
	}
	cancel()
	<-done
	next, err := acquireSlot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestRunnerEnvironmentExcludesHostConfiguration(t *testing.T) {
	t.Setenv("DAGGER_ENGINE", "cloud")
	t.Setenv("DAGGER_CLOUD_TOKEN", "secret")
	t.Setenv("SERVEF_API_TARGET", "http://localhost:9999")
	env := strings.Join(runnerEnvironment("/private-home"), "\n")
	for _, value := range []string{"secret", "http://localhost", "DAGGER_ENGINE=cloud"} {
		if strings.Contains(env, value) {
			t.Fatalf("host configuration leaked: %s", value)
		}
	}
	if !strings.Contains(env, "HOME=/private-home") || !strings.Contains(env, "docker-container://"+engine) {
		t.Fatal("explicit private runner environment missing")
	}
}

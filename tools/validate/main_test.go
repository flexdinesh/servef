//go:build linux || darwin

package main

import (
	"context"
	"errors"
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
	put(t, root, "apps/web/source.ts", "original")
	put(t, root, "script", "script")
	if err := os.Chmod(filepath.Join(root, "script"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("apps/web/source.ts", filepath.Join(root, "link")); err != nil {
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
	if err != nil || target != "apps/web/source.ts" {
		t.Fatalf("link changed: %q %v", target, err)
	}
	put(t, root, "apps/web/source.ts", "dirty")
	if _, err := readSnapshot(root); err == nil {
		t.Fatal("dirty tracked source accepted")
	}
}

func TestHistoryOnlyChangesReuseButRelevantInputsInvalidate(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/a.ts", "a")
	put(t, root, "apps/site/page.mdx", "site")
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
	put(t, root, "apps/site/page.mdx", "changed site")
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

func TestRebaseReusesHistoryOnlyInputsAndInvalidatesUpstreamContent(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/base.ts", "base")
	put(t, root, "apps/site/page.mdx", "docs")
	commit(t, root, "base")
	base := snap(t, root).Head
	if _, err := git(root, "switch", "-qc", "feature"); err != nil {
		t.Fatal(err)
	}
	put(t, root, "apps/web/feature.ts", "feature")
	commit(t, root, "feature")
	before := snap(t, root)
	for _, args := range [][]string{
		{"switch", "-q", "main"},
		{"-c", "commit.gpgsign=false", "commit", "--amend", "-qm", "renamed base"},
		{"switch", "-q", "feature"},
		{"-c", "commit.gpgsign=false", "rebase", "--onto", "main", base},
	} {
		if _, err := git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	history := snap(t, root)
	if before.Head == history.Head || before.Tree != history.Tree {
		t.Fatal("expected new history with identical source tree")
	}
	for _, group := range groups {
		if key(t, before, group) != key(t, history, group) {
			t.Fatalf("history-only rebase invalidated %s", group)
		}
	}
	if _, err := git(root, "switch", "-q", "main"); err != nil {
		t.Fatal(err)
	}
	put(t, root, "apps/web/upstream.ts", "upstream")
	commit(t, root, "upstream content")
	for _, args := range [][]string{{"switch", "-q", "feature"}, {"-c", "commit.gpgsign=false", "rebase", "main"}} {
		if _, err := git(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	upstream := snap(t, root)
	for _, group := range []string{"go", "frontend", "browser"} {
		if key(t, history, group) == key(t, upstream, group) {
			t.Fatalf("upstream content did not invalidate %s", group)
		}
	}
	if key(t, history, "docs") != key(t, upstream, "docs") {
		t.Fatal("application-only upstream change invalidated docs")
	}
}

func TestPartialProofCannotQualifyFullGroup(t *testing.T) {
	s := snapshot{Entries: []entry{{Path: "apps/web/a.ts", Mode: "100644", Blob: "abc"}}}
	_, full, _ := manifest(s, "frontend", "", nil)
	_, empty, _ := manifest(s, "frontend", "", []string{})
	if full != empty {
		t.Fatal("equivalent empty selections have different identities")
	}
	_, partial, _ := manifest(s, "frontend", "_container:test:web", []string{"filter"})
	if full == partial {
		t.Fatal("partial and full identities match")
	}
	a := app{Cache: t.TempDir()}
	if err := writeJSON(filepath.Join(a.Cache, "receipts", partial+".json"), receipt{Policy: policy, Key: partial, Head: "head", Run: "run", Finished: time.Now(), Complete: false}); err != nil {
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

func TestTargetedTasksRequireTheirCompleteSourceGroup(t *testing.T) {
	if !taskBelongsToGroup("_container:test:go", "go") {
		t.Fatal("Go task rejected for Go group")
	}
	for _, task := range []string{"_container:test:go", "_container:test:race", "_container:unknown", "test:web"} {
		if taskBelongsToGroup(task, "frontend") {
			t.Fatalf("task accepted against incomplete frontend source: %s", task)
		}
	}
}

func TestWebKitBrowserTaskBelongsToBrowserGroup(t *testing.T) {
	if !taskBelongsToGroup("_container:test:browser:webkit", "browser") {
		t.Fatal("WebKit browser task rejected for browser group")
	}
	if taskBelongsToGroup("_container:test:browser:webkit", "desktop") {
		t.Fatal("WebKit browser task accepted for desktop group")
	}
}

func TestFrontendIncludesWalkthroughSchemaContract(t *testing.T) {
	if !selected("frontend", "apps/servef/internal/walkthrough/schema.json") {
		t.Fatal("frontend example validation requires server schema")
	}
	if selected("frontend", "apps/servef/internal/walkthrough/store.go") {
		t.Fatal("unrelated Go implementation entered frontend inputs")
	}
}

func TestDesktopGroupsSelectShellAndServerInputs(t *testing.T) {
	if !isHostGroup("desktop-static") || isHostGroup("desktop") {
		t.Fatal("desktop-static runs on the host; desktop tests run in Dagger")
	}
	for _, task := range []string{"_host:typecheck:desktop", "_host:lint:desktop"} {
		if !taskBelongsToGroup(task, "desktop-static") || taskBelongsToGroup(task, "web-static") {
			t.Fatalf("wrong owner for %s", task)
		}
	}
	if !taskBelongsToGroup("_container:test:desktop", "desktop") || taskBelongsToGroup("_container:test:desktop", "browser") {
		t.Fatal("wrong owner for desktop tests")
	}
	for _, name := range []string{"apps/desktop/src/main/proxy.ts", "apps/web/src/api/events-contract.ts"} {
		if !selected("desktop-static", name) {
			t.Fatalf("desktop-static misses %s", name)
		}
	}
	for _, name := range []string{"apps/servef/internal/daemon/daemon.go", "apps/web/src/App.tsx", "apps/servef/main.go"} {
		if selected("desktop-static", name) {
			t.Fatalf("desktop-static selected unrelated %s", name)
		}
	}
	for _, name := range []string{"apps/desktop/test/smoke/app.spec.ts", "apps/servef/internal/daemon/daemon.go", "apps/servef/cmd/daemon.go", "apps/web/src/App.tsx", "testdata/markdown/README.md", "apps/servef/main.go"} {
		if !selected("desktop", name) {
			t.Fatalf("desktop misses %s", name)
		}
	}
	if selected("desktop", "apps/site/src/pages/index.mdx") || selected("desktop", "tests/browser/app.spec.ts") {
		t.Fatal("desktop selected unrelated inputs")
	}
	// Every group installs the whole workspace from the frozen lockfile.
	for _, group := range groups {
		if !selected(group, "apps/desktop/package.json") {
			t.Fatalf("%s lacks the desktop workspace manifest", group)
		}
	}
	for _, name := range []string{"apps/desktop/build/main.mjs", "apps/desktop/test-results/trace.zip", "apps/desktop/node_modules/electron/index.js"} {
		if !excluded(name) {
			t.Fatalf("generated %s entered snapshots", name)
		}
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
	child := exec.Command("sleep", "30")
	child.ExtraFiles = []*os.File{first}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	// Start synchronously duplicates ExtraFiles before closing the parent copy.
	// Pipe I/O across processes alone is not a Go memory synchronization edge.
	first.Close()
	wait, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if _, err = acquireSlot(wait, dir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("inherited lock released too early: %v", err)
	}
	_ = child.Process.Kill()
	_ = child.Wait()
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

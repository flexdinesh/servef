//go:build linux || darwin

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRerunRevokesSuccessUntilNewCertification(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/a.ts", "source")
	commit(t, root, "initial")
	s := snap(t, root)
	key := key(t, s, "frontend")
	a := app{Root: root, Cache: t.TempDir(), Global: t.TempDir()}
	proof := receipt{Policy: policy, Key: key, Head: s.Head, Run: "old-pass", Finished: time.Now(), Complete: true}
	if err := a.certify(proof); err != nil {
		t.Fatal(err)
	}
	if err := a.status(options{Group: "frontend"}); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireFile(context.Background(), filepath.Join(a.Cache, "locks", key+".lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err = a.invalidate(key, "new-attempt"); err != nil {
		t.Fatal(err)
	}
	// Failed/cancelled/crashed executions do not call certify. Even the old
	// receipt restored by an interrupted writer must remain unusable.
	if err = writeJSON(filepath.Join(a.Cache, "receipts", key+".json"), proof); err != nil {
		t.Fatal(err)
	}
	if err = a.status(options{Group: "frontend"}); err == nil {
		t.Fatal("pending/failed rerun reused prior success")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = acquireFile(ctx, filepath.Join(a.Cache, "locks", key+".lock")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent attempt bypassed certification lock: %v", err)
	}
	proof.Run = "new-pass"
	if err = a.certify(proof); err != nil {
		t.Fatal(err)
	}
	lock.Close()
	if r, ok := a.receipt(key); !ok || r.Run != "new-pass" {
		t.Fatal("new successful run not authoritative")
	}
}

func TestCancelledValidationCannotReuseCertificate(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/a.ts", "source")
	commit(t, root, "initial")
	a := app{Root: root, Cache: t.TempDir(), Global: t.TempDir()}
	s := snap(t, root)
	proof := receipt{Policy: policy, Key: key(t, s, "frontend"), Head: s.Head, Run: "pass", Finished: time.Now(), Complete: true}
	if err := a.certify(proof); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.validate(ctx, options{Group: "frontend"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled run certified: %v", err)
	}
	if _, ok := a.receipt(proof.Key); !ok {
		t.Fatal("cancel before execution revoked existing evidence")
	}
}

func TestChangedHEADCannotFinishAggregate(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/a.ts", "source")
	commit(t, root, "initial")
	s := snap(t, root)
	if _, err := git(root, "-c", "commit.gpgsign=false", "commit", "--amend", "-qm", "history"); err != nil {
		t.Fatal(err)
	}
	if err := unchangedSnapshot(root, s); err == nil {
		t.Fatal("changed HEAD accepted")
	}
}

func TestPruneProtectsActiveArtifactsAndBoundsCompletedBytes(t *testing.T) {
	a := app{Root: t.TempDir(), Cache: t.TempDir()}
	old := time.Now().Add(-31 * 24 * time.Hour)
	now := time.Now()
	for _, r := range []record{{ID: "active", Started: old}, {ID: "old", Started: old, Finished: &old}, {ID: "recent", Started: now, Finished: &now}} {
		if err := writeJSON(filepath.Join(a.artifactRoot(), r.ID, "run.json"), r); err != nil {
			t.Fatal(err)
		}
		put(t, a.artifactRoot(), r.ID+"/payload", "artifact")
	}
	active, err := acquireFile(context.Background(), filepath.Join(a.artifactRoot(), "active", "active.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	put(t, a.Cache, "receipts/proof.json", "proof")
	if err = a.pruneArtifacts(30*24*time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old", "recent"} {
		if _, err = os.Stat(filepath.Join(a.artifactRoot(), name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("completed artifact %s retained", name)
		}
	}
	if _, err = os.Stat(filepath.Join(a.artifactRoot(), "active", "payload")); err != nil {
		t.Fatal("active artifact pruned")
	}
	if _, err = os.Stat(filepath.Join(a.Cache, "receipts/proof.json")); err != nil {
		t.Fatal("certificate pruned")
	}
	active.Close()
	if err = a.pruneArtifacts(30*24*time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(a.artifactRoot(), "active")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("abandoned execution not recovered")
	}
}

func TestPruneIgnoresChangingActiveFiles(t *testing.T) {
	a := app{Root: t.TempDir(), Cache: t.TempDir()}
	dir := filepath.Join(a.artifactRoot(), "active")
	lease, err := acquireFile(context.Background(), filepath.Join(dir, "active.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err = writeJSON(filepath.Join(dir, "run.json"), record{ID: "active", Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			tmp, e := os.MkdirTemp(dir, "changing-")
			if e != nil {
				done <- e
				return
			}
			if e = os.WriteFile(filepath.Join(tmp, "output"), []byte("private"), 0600); e != nil {
				done <- e
				return
			}
			if e = os.RemoveAll(tmp); e != nil {
				done <- e
				return
			}
		}
	}()
	defer func() {
		close(stop)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	for range 20 {
		if err = a.pruneArtifacts(0, 0); err != nil {
			t.Fatalf("active file churn interrupted pruning: %v", err)
		}
	}
}

//go:build linux || darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

// All certificate mutations hold the corresponding input lock. A pending
// marker also makes readers fail closed if writing/revoking a receipt fails.
func (a app) invalidate(key, run string) error {
	if err := writeJSON(filepath.Join(a.Cache, "pending", key+".json"), map[string]string{"run": run}); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(a.Cache, "receipts", key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (a app) certify(proof receipt) error {
	if err := writeJSON(filepath.Join(a.Cache, "receipts", proof.Key+".json"), proof); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(a.Cache, "pending", proof.Key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func acquireFile(ctx context.Context, path string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err = ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Status never waits for execution. Holding shared locks across its read keeps
// the reported coverage consistent while another invocation requests a rerun.
func tryReadCertificateLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (a app) artifactRoot() string { return filepath.Join(a.Root, ".dev", "validation", "runs") }

func (a app) prune() error { return a.pruneArtifacts(30*24*time.Hour, 10*1024*1024*1024) }

// Active execution descriptors are inherited by children. Even after a
// supervisor dies, pruning cannot remove files still owned by its execution.
func (a app) pruneArtifacts(age time.Duration, budget int64) error {
	root := a.artifactRoot()
	lock, err := acquireFile(context.Background(), filepath.Join(root, ".prune.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type candidate struct {
		dir      string
		size     int64
		finished time.Time
		lock     *os.File
	}
	var candidates []candidate
	defer func() {
		for _, c := range candidates {
			c.lock.Close()
		}
	}()
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		f, e := os.OpenFile(filepath.Join(dir, "active.lock"), os.O_CREATE|os.O_RDWR, 0600)
		if e != nil {
			return e
		}
		if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
			f.Close()
			if errors.Is(e, syscall.EWOULDBLOCK) {
				continue
			}
			return e
		}
		var size int64
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !d.IsDir() {
				info, e := d.Info()
				if e != nil {
					return e
				}
				size += info.Size()
			}
			return nil
		})
		if err != nil {
			f.Close()
			return err
		}
		total += size
		var r record
		raw, e := os.ReadFile(filepath.Join(dir, "run.json"))
		if e != nil || json.Unmarshal(raw, &r) != nil {
			f.Close()
			continue
		}
		finished := r.Started
		if r.Finished != nil {
			finished = *r.Finished
		}
		if finished.IsZero() {
			f.Close()
			continue
		}
		candidates = append(candidates, candidate{dir, size, finished, f})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].finished.Before(candidates[j].finished) })
	cutoff := time.Now().Add(-age)
	for _, c := range candidates {
		if !c.finished.Before(cutoff) && total <= budget {
			continue
		}
		if err = os.RemoveAll(c.dir); err != nil {
			return err
		}
		total -= c.size
		fmt.Println("pruned", filepath.Base(c.dir))
	}
	return nil
}

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// prepareDir creates dir 0700 or validates an existing one, as servef's
// daemon does its socket directory: a real directory (not a symlink) owned
// by uid; one open to group or others is tightened to 0700.
func prepareDir(dir string, uid int) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	var stat syscall.Stat_t
	if err := syscall.Lstat(dir, &stat); err != nil {
		return fmt.Errorf("inspect state directory: %w", err)
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return fmt.Errorf("state directory %s is not a directory", dir)
	}
	if int(stat.Uid) != uid {
		return fmt.Errorf("state directory %s is owned by another user", dir)
	}
	if stat.Mode&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("restrict state directory: %w", err)
		}
	}
	return nil
}

// writePrivate replaces path with data, mode 0600, through a new file in
// path's directory renamed over it, so a symlink at path is replaced, never
// followed.
func writePrivate(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// readPrivate reads path without following a symlink there.
func readPrivate(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// createPrivate truncates or creates path, mode 0600, without following a
// symlink there.
func createPrivate(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
}

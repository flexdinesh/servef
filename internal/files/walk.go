package files

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WalkUnignored calls visit with the slash-separated root-relative path of
// every regular file and symlink below root that rules do not ignore, in
// directory order. Symlinks are reported, never followed. Ignored
// directories, .git entries, and nested repositories (directories other than
// root holding a .git entry) are pruned. No depth, filename, or exclusion
// scope applies.
//
// root must be absolute and symlink-evaluated. Failure to read root, a visit
// error, or ctx ending stops the walk with that error; unreadable directories
// below root are returned as warnings.
func WalkUnignored(ctx context.Context, root string, rules *IgnoreRules, visit func(rel string, entry fs.DirEntry) error) ([]Warning, error) {
	var warnings []Warning
	var walk func(dir string) error
	walk = func(dir string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			if dir == "" {
				return err
			}
			warnings = append(warnings, Warning{Path: dir, Err: err})
			return nil
		}
		for _, entry := range entries {
			if entry.Name() == ".git" {
				continue
			}
			rel := joinSlash(dir, entry.Name())
			switch kind := entry.Type(); {
			case kind.IsDir():
				if rules.ignoredIn(dir, rel, true) {
					continue
				}
				if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel), ".git")); err == nil {
					continue
				}
				if err := walk(rel); err != nil {
					return err
				}
			case kind.IsRegular() || kind&fs.ModeSymlink != 0:
				if rules.ignoredIn(dir, rel, false) {
					continue
				}
				if err := visit(rel, entry); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return warnings, err
	}
	return warnings, nil
}

// ResolveFile returns the absolute path of the regular file at the
// slash-separated path rel below root, after evaluating symlinks. It rejects
// absolute paths and paths that leave root, before or after evaluation, with
// ErrPathEscape. Unlike Index.Resolve, any file below root resolves; callers
// decide which paths may be read.
func ResolveFile(root, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(filepath.ToSlash(rel), "/") {
		return "", ErrPathEscape
	}
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrPathEscape
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(canonicalRoot, filepath.FromSlash(clean)))
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", clean, err)
	}
	if !Within(canonicalRoot, resolved) {
		return "", ErrPathEscape
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect %q: %w", clean, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("resolve %q: not a regular file", clean)
	}
	return resolved, nil
}

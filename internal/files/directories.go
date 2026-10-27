package files

import (
	"fmt"
	"os"
	"path/filepath"
)

// Directories lists the absolute paths of the directories a Scan with the
// same maxDepth and excluded reads, root first, including directories that
// hold no in-scope files, and counts the entries they hold other than .git.
// Watching them sees every change a rescan would. Symlinked directories are
// not followed. Failure to read root is fatal; unreadable directories below
// it are skipped.
func Directories(root string, maxDepth int, excluded []string) ([]string, int, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve scan root: %w", err)
	}
	excludedNames := excludedNameSet(excluded)
	var dirs []string
	entries := 0
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		children, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		dirs = append(dirs, dir)
		for _, child := range children {
			if child.Name() == ".git" {
				continue
			}
			entries++
			if !child.IsDir() {
				continue
			}
			if _, skip := excludedNames[child.Name()]; skip || maxDepth >= 0 && depth >= maxDepth {
				continue
			}
			_ = walk(filepath.Join(dir, child.Name()), depth+1)
		}
		return nil
	}
	if err := walk(canonicalRoot, 0); err != nil {
		return nil, 0, fmt.Errorf("read scan root: %w", err)
	}
	return dirs, entries, nil
}

// excludedNameSet normalizes --exclude values to the base names Scan skips.
func excludedNameSet(excluded []string) map[string]struct{} {
	names := make(map[string]struct{}, len(excluded))
	for _, name := range excluded {
		name = filepath.Base(filepath.Clean(name))
		if name != "." && name != string(filepath.Separator) && name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

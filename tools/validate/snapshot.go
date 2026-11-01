//go:build linux || darwin

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const policy = "servef-validation-v1-linux-amd64-dagger-0.21.10"

var groups = []string{"go", "frontend", "tooling", "docs", "browser"}

type entry struct {
	Path    string
	Mode    string
	Blob    string
	Content []byte `json:"-"`
}
type snapshot struct {
	Head    string
	Tree    string
	Entries []entry
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = gitEnvironment()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func gitEnvironment() []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
}

func readSnapshot(root string) (snapshot, error) {
	var s snapshot
	dirty, err := git(root, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return s, err
	}
	if dirty != "" {
		return s, fmt.Errorf("validation requires a clean tracked working tree; commit changes first")
	}
	s.Head, err = git(root, "rev-parse", "HEAD")
	if err != nil {
		return s, err
	}
	s.Tree, err = git(root, "rev-parse", s.Head+"^{tree}")
	if err != nil {
		return s, err
	}
	cmd := exec.Command("git", "-C", root, "ls-tree", "-rz", "--full-tree", s.Head)
	cmd.Env = gitEnvironment()
	raw, err := cmd.Output()
	if err != nil {
		return s, err
	}
	for _, record := range bytes.Split(raw, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		header, name, ok := strings.Cut(string(record), "\t")
		if !ok {
			return s, fmt.Errorf("invalid git tree entry")
		}
		parts := strings.Fields(header)
		if len(parts) != 3 {
			return s, fmt.Errorf("invalid git tree header")
		}
		if parts[1] != "blob" {
			return s, fmt.Errorf("unsupported git entry %s (%s)", name, parts[1])
		}
		if filepath.IsAbs(name) || strings.HasPrefix(name, "../") || excluded(name) {
			continue
		}
		cmd := exec.Command("git", "-C", root, "cat-file", "blob", parts[2])
		cmd.Env = gitEnvironment()
		content, err := cmd.Output()
		if err != nil {
			return s, err
		}
		s.Entries = append(s.Entries, entry{name, parts[0], parts[2], content})
	}
	return s, nil
}

func excluded(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == ".git" || part == "node_modules" {
			return true
		}
	}
	return name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasPrefix(name, ".scratch/") || strings.HasPrefix(name, ".dev/") || strings.HasPrefix(name, "internal/web/dist/") || strings.HasPrefix(name, "site/dist/") || strings.HasPrefix(name, "test-results/")
}

func selected(group, name string) bool {
	if group == "frontend" && name == "internal/walkthrough/schema.json" {
		return true
	}
	switch name {
	case "mise.toml", "mise.lock", ".node-version", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "web/package.json", "site/package.json", "dagger.json", "go.mod", "go.sum", ".gitignore", ".npmrc", ".pnpmfile.cjs":
		return true
	}
	for _, prefix := range []string{".dagger/", "patches/", "tools/validate/"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	// Shared runner imports remain available to each group without broad app inputs.
	if name == "scripts/check.ts" || name == "scripts/browser-process.ts" {
		return true
	}
	prefixes := map[string][]string{
		"go":       {"cmd/", "internal/", "tools/", "testdata/", "web/"},
		"frontend": {"web/", "testdata/"},
		"tooling":  {"scripts/", "tests/", "tools/validate/", "testdata/"},
		"docs":     {"site/"},
		"browser":  {"cmd/", "internal/", "testdata/", "web/", "tests/", "scripts/"},
	}
	for _, prefix := range prefixes[group] {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	if group == "go" || group == "browser" {
		if !strings.Contains(name, "/") && strings.HasSuffix(name, ".go") {
			return true
		}
	}
	if group == "tooling" || group == "browser" {
		return name == "tsconfig.json" || name == "playwright.config.ts"
	}
	return false
}

type inputManifest struct {
	Policy string
	Group  string
	Task   string
	Args   []string
	Files  []entry
}

func manifest(s snapshot, group, task string, args []string) (inputManifest, string, error) {
	// Flag parsing and programmatic PR calls can represent no arguments as
	// either nil or an empty slice. They describe identical check coverage.
	if len(args) == 0 {
		args = nil
	}
	m := inputManifest{Policy: policy, Group: group, Task: task, Args: args}
	for _, e := range s.Entries {
		if selected(group, e.Path) {
			m.Files = append(m.Files, e)
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return m, "", err
	}
	digest := sha256.Sum256(raw)
	return m, hex.EncodeToString(digest[:]), nil
}

func materialize(dir string, entries []entry) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, e := range entries {
		dest := filepath.Join(dir, filepath.FromSlash(e.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		if e.Mode == "120000" {
			target := string(e.Content)
			resolved := filepath.Clean(filepath.Join(filepath.Dir(e.Path), target))
			if filepath.IsAbs(target) || resolved == ".." || strings.HasPrefix(resolved, "../") {
				return fmt.Errorf("snapshot symlink escapes source: %s", e.Path)
			}
			if err := os.Symlink(target, dest); err != nil {
				return err
			}
		} else {
			mode := os.FileMode(0644)
			if e.Mode == "100755" {
				mode = 0755
			}
			if err := os.WriteFile(dest, e.Content, mode); err != nil {
				return err
			}
		}
	}
	return nil
}

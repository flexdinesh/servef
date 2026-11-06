//go:build linux || darwin

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const policy = "srcz-validation-v3-suites-host-and-linux-amd64-dagger-0.21.10"

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
	var blobs []string
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
		blobs = append(blobs, parts[2])
		s.Entries = append(s.Entries, entry{Path: name, Mode: parts[0]})
	}
	err = readCommittedBlobs(root, blobs, s.Entries)
	return s, err
}

// Request object IDs, never paths: filenames may contain newlines or tabs.
// Stream output while Git consumes requests to avoid pipe backpressure and
// retaining a second full copy of the snapshot in memory.
func readCommittedBlobs(root string, blobs []string, entries []entry) error {
	if len(blobs) == 0 {
		return nil
	}
	cmd := exec.Command("git", "-C", root, "cat-file", "--batch")
	cmd.Env = gitEnvironment()
	cmd.Stdin = strings.NewReader(strings.Join(blobs, "\n") + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	for i, blob := range blobs {
		header, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read git blob %s: %w", blob, err)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != blob || fields[1] != "blob" {
			return fmt.Errorf("invalid git batch header: %q", header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 {
			return fmt.Errorf("invalid git blob size: %q", fields[2])
		}
		content := make([]byte, size)
		if _, err := io.ReadFull(reader, content); err != nil {
			return fmt.Errorf("read git blob %s content: %w", blob, err)
		}
		if delimiter, err := reader.ReadByte(); err != nil || delimiter != '\n' {
			return fmt.Errorf("invalid git blob %s delimiter", blob)
		}
		entries[i].Content, entries[i].Blob = content, contentIdentity(content)
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		return fmt.Errorf("unexpected git batch output: %v", err)
	}
	err = cmd.Wait()
	waited = true
	if err != nil {
		return fmt.Errorf("git cat-file --batch: %w: %s", err, &stderr)
	}
	return nil
}

func contentIdentity(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

// Read twice: reject a source tree changing during capture instead of silently
// combining different edits. Neither read follows source symlinks.
func readWorkingSnapshot(root string) (snapshot, error) {
	first, err := workingSnapshot(root)
	if err != nil {
		return first, err
	}
	second, err := workingSnapshot(root)
	if err != nil {
		return second, err
	}
	if !reflect.DeepEqual(first, second) {
		return snapshot{}, fmt.Errorf("working tree changed during snapshot capture; retry")
	}
	return first, nil
}

func workingSnapshot(root string) (snapshot, error) {
	var s snapshot
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		return s, err
	}
	s.Head, err = git(root, "rev-parse", "HEAD")
	if err != nil {
		return s, err
	}
	index, err := gitList(root, "--stage", "-z")
	if err != nil {
		return s, err
	}
	for _, item := range strings.Split(index, "\x00") {
		if item == "" {
			continue
		}
		header, name, ok := strings.Cut(item, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || fields[0] == "160000" || fields[2] != "0" {
			return s, fmt.Errorf("unsupported or unmerged index entry: %s", name)
		}
	}
	names, err := gitList(root, "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return s, err
	}
	paths := strings.Split(names, "\x00")
	sort.Strings(paths)
	seen := map[string]bool{}
	for _, name := range paths {
		if name == "" || seen[name] || excluded(name) {
			continue
		}
		seen[name] = true
		path := filepath.Join(root, name)
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return s, err
		}
		relative, err := filepath.Rel(root, parent)
		if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
			return s, fmt.Errorf("source parent escapes repository: %s", name)
		}
		if parent != filepath.Clean(filepath.Dir(path)) {
			return s, fmt.Errorf("working source path traverses a symlink directory: %s", name)
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return s, err
		}
		mode := "100644"
		var content []byte
		if info.Mode()&os.ModeSymlink != 0 {
			mode = "120000"
			target, err := os.Readlink(path)
			if err != nil {
				return s, err
			}
			if err := validateLink(name, target); err != nil {
				return s, err
			}
			content = []byte(target)
		} else if info.Mode().IsRegular() {
			if info.Mode().Perm()&0111 != 0 {
				mode = "100755"
			}
			content, err = os.ReadFile(path)
			if err != nil {
				return s, err
			}
		} else {
			return s, fmt.Errorf("unsupported working tree entry: %s", name)
		}
		s.Entries = append(s.Entries, entry{name, mode, contentIdentity(content), content})
	}
	raw, err := json.Marshal(s.Entries)
	if err != nil {
		return s, err
	}
	s.Tree = contentIdentity(raw)
	return s, nil
}

func gitList(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root, "ls-files"}, args...)...)
	cmd.Env = gitEnvironment()
	output, err := cmd.Output()
	return string(output), err
}

func excluded(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == ".git" || part == "node_modules" || part == ".env" || strings.HasPrefix(part, ".env.") {
			return true
		}
	}
	return strings.HasPrefix(name, ".scratch/") || strings.HasPrefix(name, ".dev/") || strings.HasPrefix(name, "apps/srcz/internal/web/dist/") || strings.HasPrefix(name, "apps/site/dist/") || strings.HasPrefix(name, "test-results/") || strings.HasPrefix(name, "playwright-report/")
}

func selected(group, name string) bool {
	// Extended groups consume the same application and fixture inputs, while
	// their distinct group names keep certificates separate.
	switch group {
	case "browser-extended", "webkit":
		group = "browser"
	case "desktop-extended":
		group = "desktop"
	}
	if strings.HasPrefix(name, "tools/validate/") && strings.HasSuffix(name, "_test.go") {
		return group == "go" || group == "repo"
	}
	if group == "go" || group == "browser" || group == "desktop" {
		if strings.HasPrefix(name, "apps/web/") && (strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx")) {
			return false
		}
	}
	// Desktop runs tidy against its replaced srcz module: dependency tests
	// affect the required sums. Only the browser consumes production Go alone.
	if group == "browser" && strings.HasSuffix(name, "_test.go") {
		return false
	}
	if group == "frontend" && name == "apps/srcz/internal/walkthrough/schema.json" {
		return true
	}
	switch name {
	case "mise.toml", "mise.lock", ".node-version", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "apps/web/package.json", "apps/site/package.json", "packages/contracts/package.json", "dagger.json", "go.work", "apps/srcz/go.mod", "apps/srcz/go.sum", "tools/go.mod", ".gitignore", ".npmrc", ".pnpmfile.cjs":
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
		"repo":           {"apps/srcz/", "apps/desktop/", "tools/", "packages/contracts/"},
		"web-static":     {"apps/web/", "packages/contracts/"},
		"tooling-static": {"scripts/", "tests/"},
		"site-static":    {"apps/site/"},
		"go":             {"apps/srcz/", "tools/", "testdata/", "apps/web/", "packages/contracts/"},
		"frontend":       {"apps/web/", "testdata/", "packages/contracts/"},
		"tooling":        {"scripts/", "tests/", "tools/validate/", "testdata/"},
		"docs":           {"apps/site/"},
		"browser":        {"apps/srcz/", "testdata/", "apps/web/", "tests/", "scripts/", "packages/contracts/"},
		"desktop":        {"apps/desktop/", "apps/srcz/", "apps/web/", "packages/contracts/"},
	}
	for _, prefix := range prefixes[group] {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	if group == "tooling" || group == "browser" || group == "tooling-static" {
		return name == "tsconfig.json" || name == "playwright.config.ts"
	}
	return false
}

type inputManifest struct {
	Policy    string
	Platform  string
	HostTools string `json:",omitempty"`
	Group     string
	Task      string
	Args      []string
	Files     []entry
	host      *hostProfile
}

func manifest(s snapshot, group, task string, args []string, host *hostProfile) (inputManifest, string, error) {
	// Flag parsing and programmatic PR calls can represent no arguments as
	// either nil or an empty slice. They describe identical check coverage.
	if len(args) == 0 {
		args = nil
	}
	m := inputManifest{Policy: policy, Group: group, Task: task, Args: args}
	m.Platform = "linux/amd64"
	if isHostGroup(group) {
		m.Platform = runtime.GOOS + "/" + runtime.GOARCH
	}
	for _, e := range s.Entries {
		if selected(group, e.Path) {
			m.Files = append(m.Files, e)
		}
	}
	if isHostGroup(group) {
		if host != nil {
			m.HostTools, m.host = host.Identity, host
		} else if len(lockedHostVersions(m.Files)) != 0 {
			return m, "", fmt.Errorf("host tool profile required")
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
			if err := validateLink(e.Path, target); err != nil {
				return err
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

func validateLink(name, target string) error {
	resolved := filepath.Clean(filepath.Join(filepath.Dir(name), target))
	if filepath.IsAbs(target) || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("snapshot symlink escapes source: %s", name)
	}
	return nil
}

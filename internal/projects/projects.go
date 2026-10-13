// Package projects discovers git repositories and worktrees below a root and
// resolves project directories within it. It knows the .git marker and nothing
// about file formats.
package projects

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flexdinesh/servef/internal/files"
)

// ErrUnavailable is returned when a project ID does not name a servable
// directory below the root.
var ErrUnavailable = errors.New("project unavailable")

// RootID identifies the root directory itself.
const RootID = "."

// Kind describes why a directory is listed as a project.
type Kind string

const (
	// KindFolder is the root when it is not itself a repository.
	KindFolder Kind = "folder"
	// KindRepo is a repository's main worktree or a submodule.
	KindRepo Kind = "repo"
	// KindWorktree is a linked worktree created by git worktree add.
	KindWorktree Kind = "worktree"
)

// Project is a directory that can be served as a workspace.
//
// ID is the slash-separated path relative to the root ("." for the root); it
// is empty when the directory lies outside the root and cannot be opened. Path
// is the ID for available projects and the absolute path otherwise. Members of
// one repository with linked worktrees share Group, the Path of the group's
// main worktree (or of its first member when the main worktree is missing).
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      Kind   `json:"kind"`
	Group     string `json:"group"`
	Available bool   `json:"available"`
}

// Directory is a child directory listed while browsing for a project.
type Directory struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsProject bool   `json:"isProject"`
}

// Discovery is the result of Discover. Warnings hold recoverable errors below
// the root.
type Discovery struct {
	Projects []Project
	Warnings []files.Warning
}

// Discover finds repositories up to maxDepth directories below root (the root
// is depth zero) and every worktree of those repositories, wherever it lives.
// It stops descending at a repository, skips hidden and excluded directory
// names, and does not follow symlinks. A root that is not a repository is
// listed as a folder project.
func Discover(root string, maxDepth int, excluded []string) (Discovery, error) {
	canonicalRoot, err := canonicalDir(root)
	if err != nil {
		return Discovery{}, err
	}
	d := discoverer{root: canonicalRoot, excluded: excludedNames(excluded), projects: make(map[string]Project), groups: make(map[string]bool)}
	if err := d.walk(canonicalRoot, 0, maxDepth); err != nil {
		return Discovery{}, fmt.Errorf("read discovery root: %w", err)
	}
	if _, ok := d.projects[canonicalRoot]; !ok {
		d.projects[canonicalRoot] = Project{ID: RootID, Name: filepath.Base(canonicalRoot), Path: RootID, Kind: KindFolder, Available: true}
	}

	result := Discovery{Projects: make([]Project, 0, len(d.projects)), Warnings: d.warnings}
	for _, project := range d.projects {
		result.Projects = append(result.Projects, project)
	}
	sort.Slice(result.Projects, func(a, b int) bool {
		left, right := result.Projects[a], result.Projects[b]
		if left.Available != right.Available {
			return left.Available
		}
		if (left.ID == RootID) != (right.ID == RootID) {
			return left.ID == RootID
		}
		leftFold, rightFold := strings.ToLower(left.Path), strings.ToLower(right.Path)
		if leftFold != rightFold {
			return leftFold < rightFold
		}
		return left.Path < right.Path
	})
	return result, nil
}

// Resolve returns the absolute, symlink-evaluated directory for a project ID.
// Any directory below root is a project unless a path segment is .git or an
// excluded name; discovery decides listing, not access.
func Resolve(root, id string, excluded []string) (string, error) {
	rel, err := cleanID(id, excludedNames(excluded))
	if err != nil {
		return "", err
	}
	canonicalRoot, err := canonicalDir(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(canonicalRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !files.Within(canonicalRoot, resolved) {
		return "", files.ErrPathEscape
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: not a directory", ErrUnavailable)
	}
	return resolved, nil
}

// Directories lists the child directories of a project ID for browsing. Hidden
// directories are listed; .git, excluded names, and symlinks are not.
func Directories(root, id string, excluded []string) ([]Directory, error) {
	dir, err := Resolve(root, id, excluded)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	skip := excludedNames(excluded)
	rel, _ := cleanID(id, skip)
	result := make([]Directory, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == ".git" || skip[name] {
			continue
		}
		childPath := name
		if rel != RootID {
			childPath = rel + "/" + name
		}
		result = append(result, Directory{Name: name, Path: childPath, IsProject: hasGit(filepath.Join(dir, name))})
	}
	sort.Slice(result, func(a, b int) bool {
		left, right := strings.ToLower(result[a].Name), strings.ToLower(result[b].Name)
		if left != right {
			return left < right
		}
		return result[a].Name < result[b].Name
	})
	return result, nil
}

type discoverer struct {
	root     string
	excluded map[string]bool
	projects map[string]Project
	groups   map[string]bool
	warnings []files.Warning
}

func (d *discoverer) walk(dir string, depth, maxDepth int) error {
	if hasGit(dir) {
		d.addRepository(dir)
		return nil
	}
	if depth >= maxDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if depth == 0 {
			return err
		}
		d.warn(dir, err)
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		// DirEntry types come from Lstat, so symlinked directories are not IsDir.
		if !entry.IsDir() || strings.HasPrefix(name, ".") || d.excluded[name] {
			continue
		}
		if err := d.walk(filepath.Join(dir, name), depth+1, maxDepth); err != nil {
			return err
		}
	}
	return nil
}

// addRepository lists a repository and every verified worktree of it.
func (d *discoverer) addRepository(dir string) {
	common, err := commonDir(dir)
	if err != nil {
		d.warn(dir, err)
		d.add(dir, "")
		return
	}
	if d.groups[common] {
		if _, ok := d.projects[dir]; !ok {
			d.add(dir, "")
		}
		return
	}
	d.groups[common] = true
	members := worktrees(common)
	if !containsPath(members, dir) {
		members = append(members, dir)
	}
	group := ""
	if len(members) > 1 {
		group = d.displayPath(members[0])
	}
	for _, member := range members {
		d.add(member, group)
	}
}

func (d *discoverer) add(dir, group string) {
	kind := KindRepo
	if isLinkedWorktree(dir) {
		kind = KindWorktree
	}
	project := Project{Name: filepath.Base(dir), Path: d.displayPath(dir), Kind: kind, Group: group}
	if files.Within(d.root, dir) {
		project.ID = project.Path
		project.Available = true
	}
	d.projects[dir] = project
}

func (d *discoverer) displayPath(dir string) string {
	if !files.Within(d.root, dir) {
		return dir
	}
	rel, err := filepath.Rel(d.root, dir)
	if err != nil {
		return dir
	}
	return filepath.ToSlash(rel)
}

func (d *discoverer) warn(dir string, err error) {
	d.warnings = append(d.warnings, files.Warning{Path: d.displayPath(dir), Err: err})
}

// commonDir returns the shared git directory of a worktree.
func commonDir(worktree string) (string, error) {
	gitPath := filepath.Join(worktree, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return canonical(gitPath), nil
	}
	admin, err := readGitFile(gitPath)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(admin, "commondir"))
	if errors.Is(err, fs.ErrNotExist) {
		return admin, nil
	}
	if err != nil {
		return "", err
	}
	return canonical(resolveFrom(admin, strings.TrimSpace(string(content)))), nil
}

// worktrees returns the main worktree, when the common directory is a .git
// directory, followed by every linked worktree whose .git file links back.
func worktrees(common string) []string {
	var members []string
	if filepath.Base(common) == ".git" {
		main := filepath.Dir(common)
		if info, err := os.Lstat(filepath.Join(main, ".git")); err == nil && info.IsDir() {
			members = append(members, main)
		}
	}
	entries, err := os.ReadDir(filepath.Join(common, "worktrees"))
	if err != nil {
		return members
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		admin := filepath.Join(common, "worktrees", entry.Name())
		content, err := os.ReadFile(filepath.Join(admin, "gitdir"))
		if err != nil {
			continue
		}
		gitFile := resolveFrom(admin, strings.TrimSpace(string(content)))
		back, err := readGitFile(gitFile)
		if err != nil || back != canonical(admin) {
			continue
		}
		members = append(members, canonical(filepath.Dir(gitFile)))
	}
	return members
}

// readGitFile reads a "gitdir: <path>" file and returns the canonical path.
func readGitFile(name string) (string, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a gitdir file", name)
	}
	content, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
	if !ok {
		return "", fmt.Errorf("%s is not a gitdir file", name)
	}
	return canonical(resolveFrom(filepath.Dir(name), strings.TrimSpace(target))), nil
}

// isLinkedWorktree reports whether dir's .git file names an admin directory
// that shares a common git directory, as linked worktrees do and submodules
// do not.
func isLinkedWorktree(dir string) bool {
	admin, err := readGitFile(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(admin, "commondir"))
	return err == nil
}

func hasGit(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

func cleanID(id string, excluded map[string]bool) (string, error) {
	if id == "" || filepath.IsAbs(id) || strings.Contains(id, "\\") {
		return "", fmt.Errorf("%w: invalid project id %q", ErrUnavailable, id)
	}
	rel := path.Clean(id)
	if rel != id || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", files.ErrPathEscape
	}
	if rel == RootID {
		return rel, nil
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == ".git" || excluded[segment] {
			return "", fmt.Errorf("%w: %q is excluded", ErrUnavailable, id)
		}
	}
	return rel, nil
}

func canonicalDir(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root %q is not a directory", root)
	}
	return resolved, nil
}

// canonical evaluates symlinks when the path exists and cleans it otherwise.
func canonical(name string) string {
	if resolved, err := filepath.EvalSymlinks(name); err == nil {
		return resolved
	}
	return filepath.Clean(name)
}

func resolveFrom(base, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(base, name)
}

func containsPath(paths []string, name string) bool {
	for _, candidate := range paths {
		if candidate == name {
			return true
		}
	}
	return false
}

func excludedNames(excluded []string) map[string]bool {
	names := make(map[string]bool, len(excluded))
	for _, name := range excluded {
		name = filepath.Base(filepath.Clean(name))
		if name != "." && name != string(filepath.Separator) && name != "" {
			names[name] = true
		}
	}
	return names
}

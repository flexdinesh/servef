// Package projects discovers git repositories and worktrees below served roots
// and resolves project directories within them. It knows the .git marker and
// git's worktree and HEAD files, and nothing about file formats.
package projects

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flexdinesh/servef/internal/files"
)

// ErrUnavailable is returned when a project ID does not name a servable
// directory.
var ErrUnavailable = errors.New("project unavailable")

// ErrInsideRepository is returned for a directory inside a repository that is
// not itself a repository or worktree: repositories open whole.
var ErrInsideRepository = fmt.Errorf("%w: inside a repository", ErrUnavailable)

// Kind describes why a directory is listed as a project.
type Kind string

const (
	// KindFolder is a served root that is not itself a repository.
	KindFolder Kind = "folder"
	// KindRepo is a repository's main worktree or a submodule.
	KindRepo Kind = "repo"
	// KindWorktree is a linked worktree created by git worktree add.
	KindWorktree Kind = "worktree"
)

// Root is a served root directory.
type Root struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	IsRepo bool   `json:"isRepo"`
}

// Project is a directory that can be served as a workspace.
//
// ID is the canonical absolute path; it is empty when the directory cannot be
// opened. Path is always the absolute path. Members of one repository with
// linked worktrees share Group, the Path of the group's main worktree (or of
// its first member when the main worktree is missing). Root is the served root
// the project, or its repository, lives in; empty when none. Branch is the
// checked-out branch, a short commit for a detached HEAD, or empty.
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      Kind   `json:"kind"`
	Group     string `json:"group"`
	Root      string `json:"root"`
	Branch    string `json:"branch"`
	Available bool   `json:"available"`
}

// Directory is a child directory listed while browsing for a project.
type Directory struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsProject bool   `json:"isProject"`
}

// Discovery is the result of Discover. Warnings hold recoverable errors below
// the roots.
type Discovery struct {
	Projects []Project
	Warnings []files.Warning
}

// CanonicalRoots resolves served roots to absolute, symlink-evaluated
// directories in order, drops duplicates, and rejects roots nested in another.
func CanonicalRoots(paths []string) ([]string, error) {
	roots := make([]string, 0, len(paths))
	for _, name := range paths {
		root, err := canonicalDir(name)
		if err != nil {
			return nil, err
		}
		if containsPath(roots, root) {
			continue
		}
		for _, other := range roots {
			if files.Within(other, root) || files.Within(root, other) {
				return nil, fmt.Errorf("roots %q and %q overlap; pass only the outer one", other, root)
			}
		}
		roots = append(roots, root)
	}
	if len(roots) == 0 {
		return nil, errors.New("no root to serve")
	}
	return roots, nil
}

// Roots describes served roots for display.
func Roots(roots []string) []Root {
	result := make([]Root, len(roots))
	for i, root := range roots {
		result[i] = Root{Path: root, Name: filepath.Base(root), IsRepo: hasGit(root)}
	}
	return result
}

// Discover finds repositories up to maxDepth directories below each root (a
// root is depth zero) and every worktree of those repositories, wherever it
// lives. It stops descending at a repository, skips hidden and excluded
// directory names, and does not follow symlinks. A root that is not a
// repository is listed as a folder project.
func Discover(roots []string, maxDepth int, excluded []string) (Discovery, error) {
	canonicalRoots := make([]string, len(roots))
	for i, root := range roots {
		canonicalRoot, err := canonicalDir(root)
		if err != nil {
			return Discovery{}, err
		}
		canonicalRoots[i] = canonicalRoot
	}
	d := discoverer{roots: canonicalRoots, excluded: excludedNames(excluded), projects: make(map[string]Project), groups: make(map[string]bool)}
	for _, root := range canonicalRoots {
		if err := d.walk(root, 0, maxDepth); err != nil {
			return Discovery{}, fmt.Errorf("read discovery root: %w", err)
		}
		if _, ok := d.projects[root]; !ok {
			d.projects[root] = Project{ID: root, Name: filepath.Base(root), Path: root, Kind: KindFolder, Root: root, Available: true}
		}
	}

	result := Discovery{Projects: make([]Project, 0, len(d.projects)), Warnings: d.warnings}
	for _, project := range d.projects {
		result.Projects = append(result.Projects, project)
	}
	rank := func(project Project) int {
		for i, root := range canonicalRoots {
			if project.Root == root {
				return i
			}
		}
		return len(canonicalRoots)
	}
	sort.Slice(result.Projects, func(a, b int) bool {
		left, right := result.Projects[a], result.Projects[b]
		if left.Available != right.Available {
			return left.Available
		}
		if leftRank, rightRank := rank(left), rank(right); leftRank != rightRank {
			return leftRank < rightRank
		}
		if (left.Path == left.Root) != (right.Path == right.Root) {
			return left.Path == left.Root
		}
		leftFold, rightFold := strings.ToLower(left.Path), strings.ToLower(right.Path)
		if leftFold != rightFold {
			return leftFold < rightFold
		}
		return left.Path < right.Path
	})
	return result, nil
}

// Resolve returns the symlink-evaluated directory for a project ID, an
// absolute path. Below a root, a directory opens when it is a repository or
// worktree, or when no directory from the root down to it is one; .git and
// excluded names never open. Outside every root, only a linked worktree whose
// repository lies in a root opens.
func Resolve(roots []string, id string, excluded []string) (string, error) {
	resolved, root, err := locate(roots, id, excludedNames(excluded))
	if err != nil {
		return "", err
	}
	if root == "" || hasGit(resolved) {
		return resolved, nil
	}
	if insideRepository(root, resolved) {
		return "", ErrInsideRepository
	}
	return resolved, nil
}

// Directories lists the child directories of a folder for browsing; an empty
// ID lists the roots. Repositories and their contents are not browsed: they
// open whole. Hidden directories are listed; .git, excluded names, and
// symlinks are not.
func Directories(roots []string, id string, excluded []string) ([]Directory, error) {
	if id == "" {
		result := make([]Directory, 0, len(roots))
		for _, root := range Roots(roots) {
			result = append(result, Directory{Name: root.Name, Path: root.Path, IsProject: root.IsRepo})
		}
		return result, nil
	}
	skip := excludedNames(excluded)
	dir, root, err := locate(roots, id, skip)
	if err != nil {
		return nil, err
	}
	if root == "" || hasGit(dir) || insideRepository(root, dir) {
		return nil, ErrInsideRepository
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make([]Directory, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == ".git" || skip[name] {
			continue
		}
		child := filepath.Join(dir, name)
		result = append(result, Directory{Name: name, Path: child, IsProject: hasGit(child)})
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

// locate resolves an ID to an existing directory and the root containing it.
// The root is empty for a linked worktree outside every root whose repository
// lies in one; any other directory outside the roots escapes.
func locate(roots []string, id string, excluded map[string]bool) (string, string, error) {
	if id == "" || !filepath.IsAbs(id) || filepath.Clean(id) != id {
		return "", "", fmt.Errorf("%w: invalid project id %q", ErrUnavailable, id)
	}
	resolved, err := filepath.EvalSymlinks(id)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%w: not a directory", ErrUnavailable)
	}
	for _, root := range roots {
		canonicalRoot := canonical(root)
		if !files.Within(canonicalRoot, resolved) {
			continue
		}
		rel, err := filepath.Rel(canonicalRoot, resolved)
		if err != nil {
			return "", "", files.ErrPathEscape
		}
		for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
			if segment == ".git" || excluded[segment] {
				return "", "", fmt.Errorf("%w: %q is excluded", ErrUnavailable, id)
			}
		}
		return resolved, canonicalRoot, nil
	}
	if common, ok := worktreeCommonDir(resolved); ok && withinAny(roots, common) {
		return resolved, "", nil
	}
	return "", "", files.ErrPathEscape
}

// insideRepository reports whether a directory from root down to dir's parent
// is a repository.
func insideRepository(root, dir string) bool {
	for parent := filepath.Dir(dir); files.Within(root, parent); parent = filepath.Dir(parent) {
		if hasGit(parent) {
			return true
		}
		if parent == root {
			break
		}
	}
	return false
}

type discoverer struct {
	roots    []string
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
		d.add(dir, "", "")
		return
	}
	if d.groups[common] {
		if _, ok := d.projects[dir]; !ok {
			d.add(dir, "", common)
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
		group = members[0]
	}
	for _, member := range members {
		d.add(member, group, common)
	}
}

// add lists a repository member. A member outside the roots opens only as a
// linked worktree of a repository whose common directory is in a root.
func (d *discoverer) add(dir, group, common string) {
	kind := KindRepo
	if isLinkedWorktree(dir) {
		kind = KindWorktree
	}
	project := Project{Name: filepath.Base(dir), Path: dir, Kind: kind, Group: group, Root: d.rootOf(dir), Branch: Branch(dir)}
	if project.Root != "" {
		project.Available = true
	} else if common != "" {
		project.Root = d.rootOf(common)
		project.Available = kind == KindWorktree && project.Root != ""
	}
	if project.Available {
		project.ID = dir
	}
	d.projects[dir] = project
}

func (d *discoverer) rootOf(dir string) string {
	for _, root := range d.roots {
		if files.Within(root, dir) {
			return root
		}
	}
	return ""
}

func (d *discoverer) warn(dir string, err error) {
	d.warnings = append(d.warnings, files.Warning{Path: dir, Err: err})
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

// worktreeCommonDir returns the common git directory of a linked worktree
// whose admin directory links back to it, as git worktree add creates.
func worktreeCommonDir(dir string) (string, bool) {
	gitFile := filepath.Join(dir, ".git")
	admin, err := readGitFile(gitFile)
	if err != nil {
		return "", false
	}
	content, err := os.ReadFile(filepath.Join(admin, "gitdir"))
	if err != nil || canonical(resolveFrom(admin, strings.TrimSpace(string(content)))) != canonical(gitFile) {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(admin, "commondir")); err != nil {
		return "", false
	}
	common, err := commonDir(dir)
	return common, err == nil
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

// Branch reads a worktree's HEAD: the branch name, another ref's name, or a
// short commit when detached. It is empty when dir is not a worktree or HEAD
// cannot be read.
func Branch(dir string) string {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return ""
	}
	gitDir := gitPath
	if !info.IsDir() {
		if gitDir, err = readGitFile(gitPath); err != nil {
			return ""
		}
	}
	content, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(content))
	if ref, ok := strings.CutPrefix(head, "ref:"); ok {
		ref = strings.TrimSpace(ref)
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return name
		}
		return ref
	}
	if len(head) > 7 {
		return head[:7]
	}
	return head
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

func withinAny(roots []string, dir string) bool {
	for _, root := range roots {
		if files.Within(canonical(root), dir) {
			return true
		}
	}
	return false
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

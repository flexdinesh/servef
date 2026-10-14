package projects

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/flexdinesh/servef/internal/files"
)

var defaultExcluded = []string{".git", "node_modules", "vendor"}

func TestDiscoverFindsRepositoriesAndStopsAtThem(t *testing.T) {
	root := canonicalTemp(t)
	makeRepo(t, filepath.Join(root, "alpha"))
	makeRepo(t, filepath.Join(root, "alpha", "nested"))
	makeRepo(t, filepath.Join(root, "work", "Beta"))
	makeRepo(t, filepath.Join(root, "work", "deep", "deeper", "too-deep"))
	makeRepo(t, filepath.Join(root, ".cache", "hidden"))
	makeRepo(t, filepath.Join(root, "node_modules", "pkg"))
	writeFile(t, filepath.Join(root, "notes.md"), "notes")
	if err := os.Symlink(filepath.Join(root, "alpha"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	discovery, err := Discover(root, 3, defaultExcluded)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []Project{
		{ID: ".", Name: filepath.Base(root), Path: ".", Kind: KindFolder, Available: true},
		{ID: "alpha", Name: "alpha", Path: "alpha", Kind: KindRepo, Available: true},
		{ID: "work/Beta", Name: "Beta", Path: "work/Beta", Kind: KindRepo, Available: true},
	}
	if !reflect.DeepEqual(discovery.Projects, want) {
		t.Fatalf("Projects = %#v\nwant %#v", discovery.Projects, want)
	}
}

func TestDiscoverScanDepthZeroChecksRootOnly(t *testing.T) {
	root := canonicalTemp(t)
	makeRepo(t, filepath.Join(root, "alpha"))

	discovery, err := Discover(root, 0, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(discovery.Projects); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("IDs = %v, want root only", got)
	}
}

func TestDiscoverRootRepositoryIsTheOnlyProject(t *testing.T) {
	root := canonicalTemp(t)
	makeRepo(t, root)
	makeRepo(t, filepath.Join(root, "vendored"))

	discovery, err := Discover(root, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	want := []Project{{ID: ".", Name: filepath.Base(root), Path: ".", Kind: KindRepo, Available: true}}
	if !reflect.DeepEqual(discovery.Projects, want) {
		t.Fatalf("Projects = %#v, want %#v", discovery.Projects, want)
	}
}

func TestDiscoverGroupsWorktreesWherever(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "root")
	main := filepath.Join(root, "app", "main")
	makeRepo(t, main)
	linkWorktree(t, main, "sibling", filepath.Join(root, "app", "sibling"))
	linkWorktree(t, main, "inside", filepath.Join(main, ".worktrees", "inside"))
	linkWorktree(t, main, "outside", filepath.Join(parent, "elsewhere", "outside"))

	discovery, err := Discover(root, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "elsewhere", "outside")
	want := []Project{
		{ID: ".", Name: "root", Path: ".", Kind: KindFolder, Available: true},
		{ID: "app/main", Name: "main", Path: "app/main", Kind: KindRepo, Group: "app/main", Available: true},
		{ID: "app/main/.worktrees/inside", Name: "inside", Path: "app/main/.worktrees/inside", Kind: KindWorktree, Group: "app/main", Available: true},
		{ID: "app/sibling", Name: "sibling", Path: "app/sibling", Kind: KindWorktree, Group: "app/main", Available: true},
		{Name: "outside", Path: outside, Kind: KindWorktree, Group: "app/main"},
	}
	if !reflect.DeepEqual(discovery.Projects, want) {
		t.Fatalf("Projects = %#v\nwant %#v", discovery.Projects, want)
	}

	// A root holding only a linked worktree still finds its out-of-root main.
	worktreeOnly, err := Discover(filepath.Join(root, "app", "sibling"), 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(worktreeOnly.Projects); got != 4 {
		t.Fatalf("worktree-root projects = %#v, want 4 group members", worktreeOnly.Projects)
	}
	if first := worktreeOnly.Projects[0]; first.ID != "." || first.Kind != KindWorktree || first.Group != main {
		t.Fatalf("worktree root = %#v", first)
	}
}

func TestDiscoverIgnoresWorktreesWithoutBacklink(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "root")
	main := filepath.Join(root, "main")
	makeRepo(t, main)
	secret := filepath.Join(parent, "secret")
	writeFile(t, filepath.Join(secret, "key"), "private")
	admin := filepath.Join(main, ".git", "worktrees", "forged")
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(secret, ".git")+"\n")
	writeFile(t, filepath.Join(admin, "commondir"), "../..\n")

	discovery, err := Discover(root, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(discovery.Projects); !reflect.DeepEqual(got, []string{".", "main"}) {
		t.Fatalf("IDs = %v, want forged worktree ignored", got)
	}
	if discovery.Projects[1].Group != "" {
		t.Fatalf("lone repository group = %q, want empty", discovery.Projects[1].Group)
	}
}

func TestDiscoverWithGitWorktrees(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "root")
	main := filepath.Join(root, "main")
	runGit(t, gitPath, parent, "init", "-q", main)
	runGit(t, gitPath, main, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	runGit(t, gitPath, main, "worktree", "add", "-q", filepath.Join(root, "feature"))
	runGit(t, gitPath, main, "worktree", "add", "-q", filepath.Join(parent, "outside"))

	discovery, err := Discover(root, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	var available, unavailable []string
	for _, project := range discovery.Projects {
		if project.ID != RootID && project.Group != "main" {
			t.Fatalf("project %#v not grouped under main", project)
		}
		if project.Available {
			available = append(available, project.ID)
		} else {
			unavailable = append(unavailable, project.Path)
		}
	}
	if !reflect.DeepEqual(available, []string{".", "feature", "main"}) {
		t.Fatalf("available = %v", available)
	}
	if !reflect.DeepEqual(unavailable, []string{filepath.Join(parent, "outside")}) {
		t.Fatalf("unavailable = %v", unavailable)
	}
}

func TestResolveConfinesProjectsToRoot(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "root")
	writeFile(t, filepath.Join(root, "docs", "readme.md"), "docs")
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "")
	writeFile(t, filepath.Join(root, "file.txt"), "")
	writeFile(t, filepath.Join(parent, "outside", "secret"), "")
	if err := os.Symlink(filepath.Join(parent, "outside"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[string]string{
		".":    root,
		"docs": filepath.Join(root, "docs"),
	} {
		got, err := Resolve(root, id, defaultExcluded)
		if err != nil || got != want {
			t.Fatalf("Resolve(%q) = %q, %v; want %q", id, got, err, want)
		}
	}
	for id, want := range map[string]error{
		"":                 ErrUnavailable,
		"..":               files.ErrPathEscape,
		"docs/../..":       files.ErrPathEscape,
		"./docs":           files.ErrPathEscape,
		parent:             ErrUnavailable,
		"node_modules/pkg": ErrUnavailable,
		".git":             ErrUnavailable,
		"missing":          ErrUnavailable,
		"file.txt":         ErrUnavailable,
		"escape":           files.ErrPathEscape,
	} {
		if _, err := Resolve(root, id, defaultExcluded); !errors.Is(err, want) {
			t.Errorf("Resolve(%q) error = %v, want %v", id, err, want)
		}
	}
}

func TestDirectoriesListsBrowsableChildren(t *testing.T) {
	root := canonicalTemp(t)
	makeRepo(t, filepath.Join(root, "work", "api"))
	writeFile(t, filepath.Join(root, "work", ".config", "settings"), "")
	writeFile(t, filepath.Join(root, "work", "Apps", "readme.md"), "")
	writeFile(t, filepath.Join(root, "work", "node_modules", "pkg", "index.js"), "")
	writeFile(t, filepath.Join(root, "work", "file.txt"), "")
	if err := os.Symlink(filepath.Join(root, "work", "Apps"), filepath.Join(root, "work", "link")); err != nil {
		t.Fatal(err)
	}

	got, err := Directories(root, "work", defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	want := []Directory{
		{Name: ".config", Path: "work/.config"},
		{Name: "api", Path: "work/api", IsProject: true},
		{Name: "Apps", Path: "work/Apps"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Directories() = %#v\nwant %#v", got, want)
	}
	top, err := Directories(root, RootID, defaultExcluded)
	if err != nil || !reflect.DeepEqual(top, []Directory{{Name: "work", Path: "work"}}) {
		t.Fatalf("root Directories() = %#v, %v", top, err)
	}
	if _, err := Directories(root, "..", defaultExcluded); !errors.Is(err, files.ErrPathEscape) {
		t.Fatalf("Directories(..) error = %v", err)
	}
}

func TestDiscoveredProjectsResolve(t *testing.T) {
	root := canonicalTemp(t)
	makeRepo(t, filepath.Join(root, "a"))
	makeRepo(t, filepath.Join(root, "b", "c"))
	linkWorktree(t, filepath.Join(root, "a"), "wt", filepath.Join(root, "a-wt"))

	discovery, err := Discover(root, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range discovery.Projects {
		if _, err := Resolve(root, project.ID, defaultExcluded); err != nil {
			t.Errorf("Resolve(%q) error = %v", project.ID, err)
		}
	}
}

func TestDiscoverRejectsMissingRoot(t *testing.T) {
	if _, err := Discover(filepath.Join(t.TempDir(), "missing"), 10, nil); err == nil {
		t.Fatal("Discover() error = nil, want missing root error")
	}
}

func canonicalTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func makeRepo(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main\n")
}

// linkWorktree writes the files git worktree add creates for a linked worktree.
func linkWorktree(t *testing.T, main, name, dir string) {
	t.Helper()
	admin := filepath.Join(main, ".git", "worktrees", name)
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(dir, ".git")+"\n")
	writeFile(t, filepath.Join(admin, "commondir"), "../..\n")
	writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+admin+"\n")
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, gitPath, dir string, args ...string) {
	t.Helper()
	command := exec.Command(gitPath, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func ids(projects []Project) []string {
	result := make([]string, len(projects))
	for i, project := range projects {
		result[i] = project.ID
	}
	return result
}

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

	discovery, err := Discover([]string{root}, 3, defaultExcluded)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	alpha, beta := filepath.Join(root, "alpha"), filepath.Join(root, "work", "Beta")
	want := []Project{
		{ID: root, Name: filepath.Base(root), Path: root, Kind: KindFolder, Root: root, Available: true},
		{ID: alpha, Name: "alpha", Path: alpha, Kind: KindRepo, Root: root, Branch: "main", Repository: "alpha", Available: true},
		{ID: beta, Name: "Beta", Path: beta, Kind: KindRepo, Root: root, Branch: "main", Repository: "Beta", Available: true},
	}
	if !reflect.DeepEqual(discovery.Projects, want) {
		t.Fatalf("Projects = %#v\nwant %#v", discovery.Projects, want)
	}
}

func TestDiscoverScanDepthZeroChecksRootOnly(t *testing.T) {
	root := canonicalTemp(t)
	makeRepo(t, filepath.Join(root, "alpha"))

	discovery, err := Discover([]string{root}, 0, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(discovery.Projects); !reflect.DeepEqual(got, []string{root}) {
		t.Fatalf("IDs = %v, want root only", got)
	}
}

func TestDiscoverRootRepositoryIsTheOnlyProject(t *testing.T) {
	root := canonicalTemp(t)
	makeRepo(t, root)
	makeRepo(t, filepath.Join(root, "vendored"))

	discovery, err := Discover([]string{root}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	want := []Project{{ID: root, Name: filepath.Base(root), Path: root, Kind: KindRepo, Root: root, Branch: "main", Repository: filepath.Base(root), Available: true}}
	if !reflect.DeepEqual(discovery.Projects, want) {
		t.Fatalf("Projects = %#v, want %#v", discovery.Projects, want)
	}
}

func TestDiscoverListsEveryRootInOrder(t *testing.T) {
	parent := canonicalTemp(t)
	work, personal := filepath.Join(parent, "work"), filepath.Join(parent, "personal")
	makeRepo(t, filepath.Join(work, "api"))
	makeRepo(t, personal)
	makeRepo(t, filepath.Join(parent, "between"))

	discovery, err := Discover([]string{work, personal}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(discovery.Projects), []string{work, filepath.Join(work, "api"), personal}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs = %v, want %v", got, want)
	}
	if got := discovery.Projects[2]; got.Kind != KindRepo || got.Root != personal {
		t.Fatalf("repository root project = %#v", got)
	}
}

func TestDiscoverGroupsWorktreesWherever(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "root")
	main := filepath.Join(root, "app", "main")
	makeRepo(t, main)
	sibling := linkWorktree(t, main, "sibling", filepath.Join(root, "app", "sibling"))
	inside := linkWorktree(t, main, "inside", filepath.Join(main, ".worktrees", "inside"))
	outside := linkWorktree(t, main, "outside", filepath.Join(parent, "elsewhere", "outside"))

	discovery, err := Discover([]string{root}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	want := []Project{
		{ID: root, Name: "root", Path: root, Kind: KindFolder, Root: root, Available: true},
		{ID: outside, Name: "outside", Path: outside, Kind: KindWorktree, Group: main, Root: root, Branch: "outside", Repository: "main", Available: true},
		{ID: main, Name: "main", Path: main, Kind: KindRepo, Group: main, Root: root, Branch: "main", Repository: "main", Available: true},
		{ID: inside, Name: "inside", Path: inside, Kind: KindWorktree, Group: main, Root: root, Branch: "inside", Repository: "main", Available: true},
		{ID: sibling, Name: "sibling", Path: sibling, Kind: KindWorktree, Group: main, Root: root, Branch: "sibling", Repository: "main", Available: true},
	}
	if !reflect.DeepEqual(discovery.Projects, want) {
		t.Fatalf("Projects = %#v\nwant %#v", discovery.Projects, want)
	}

	// A root holding only a linked worktree still lists its group; members
	// outside that root, and repositories outside every root, do not open.
	worktreeOnly, err := Discover([]string{sibling}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(worktreeOnly.Projects); got != 4 {
		t.Fatalf("worktree-root projects = %#v, want 4 group members", worktreeOnly.Projects)
	}
	if first := worktreeOnly.Projects[0]; first.ID != sibling || first.Kind != KindWorktree || first.Group != main {
		t.Fatalf("worktree root = %#v", first)
	}
	for _, project := range worktreeOnly.Projects[1:] {
		if project.Available || project.ID != "" {
			t.Errorf("member outside its root = %#v, want unavailable", project)
		}
	}
}

func TestDiscoverGroupsWorktreesAcrossRoots(t *testing.T) {
	parent := canonicalTemp(t)
	first, second := filepath.Join(parent, "first"), filepath.Join(parent, "second")
	main := filepath.Join(first, "main")
	makeRepo(t, main)
	feature := linkWorktree(t, main, "feature", filepath.Join(second, "feature"))

	discovery, err := Discover([]string{second, first}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range discovery.Projects {
		if project.Kind == KindFolder {
			continue
		}
		if project.Group != main || !project.Available {
			t.Errorf("project = %#v, want available member of %s", project, main)
		}
	}
	if got := len(discovery.Projects); got != 4 {
		t.Fatalf("projects = %#v, want two roots and two members", discovery.Projects)
	}
	if _, err := Resolve([]string{first, second}, feature, defaultExcluded); err != nil {
		t.Fatalf("Resolve(feature) error = %v", err)
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

	discovery, err := Discover([]string{root}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(discovery.Projects); !reflect.DeepEqual(got, []string{root, main}) {
		t.Fatalf("IDs = %v, want forged worktree ignored", got)
	}
	if discovery.Projects[1].Group != "" {
		t.Fatalf("lone repository group = %q, want empty", discovery.Projects[1].Group)
	}
	if _, err := Resolve([]string{root}, secret, defaultExcluded); !errors.Is(err, files.ErrPathEscape) {
		t.Fatalf("Resolve(forged) error = %v, want escape", err)
	}
	// A .git file claiming another worktree's admin directory, which does not
	// link back, does not open either.
	linkWorktree(t, main, "real", filepath.Join(root, "real"))
	writeFile(t, filepath.Join(secret, ".git"), "gitdir: "+filepath.Join(main, ".git", "worktrees", "real")+"\n")
	if _, err := Resolve([]string{root}, secret, defaultExcluded); !errors.Is(err, files.ErrPathEscape) {
		t.Fatalf("Resolve(claimed worktree) error = %v, want escape", err)
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
	runGit(t, gitPath, parent, "init", "-q", "-b", "trunk", main)
	runGit(t, gitPath, main, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	runGit(t, gitPath, main, "worktree", "add", "-q", "-b", "feature-x", filepath.Join(root, "feature"))
	runGit(t, gitPath, main, "worktree", "add", "-q", "--detach", filepath.Join(parent, "outside"))

	discovery, err := Discover([]string{root}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	branches := map[string]string{}
	for _, project := range discovery.Projects {
		if project.ID != root && project.Group != main {
			t.Fatalf("project %#v not grouped under main", project)
		}
		if !project.Available {
			t.Fatalf("project %#v unavailable", project)
		}
		branches[project.Name] = project.Branch
	}
	if branches["main"] != "trunk" || branches["feature"] != "feature-x" || len(branches["outside"]) != 7 {
		t.Fatalf("branches = %v, want trunk, feature-x, and a short detached commit", branches)
	}
	if _, err := Resolve([]string{root}, filepath.Join(parent, "outside"), defaultExcluded); err != nil {
		t.Fatalf("Resolve(outside worktree) error = %v", err)
	}
}

func TestResolveOpensReposAndFoldersOutsideRepos(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "root")
	repo := filepath.Join(root, "repo")
	makeRepo(t, repo)
	makeRepo(t, filepath.Join(repo, "packages", "submodule"))
	writeFile(t, filepath.Join(repo, "packages", "app", "index.js"), "")
	writeFile(t, filepath.Join(root, "docs", "guides", "readme.md"), "docs")
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "")
	writeFile(t, filepath.Join(root, "file.txt"), "")
	writeFile(t, filepath.Join(parent, "outside", "secret"), "")
	if err := os.Symlink(filepath.Join(parent, "outside"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	roots := []string{root}

	for _, id := range []string{
		root,
		filepath.Join(root, "docs"),
		filepath.Join(root, "docs", "guides"),
		repo,
		filepath.Join(repo, "packages", "submodule"),
	} {
		got, err := Resolve(roots, id, defaultExcluded)
		if err != nil || got != id {
			t.Errorf("Resolve(%q) = %q, %v; want it", id, got, err)
		}
	}
	for id, want := range map[string]error{
		"":                                     ErrUnavailable,
		".":                                    ErrUnavailable,
		"docs":                                 ErrUnavailable,
		root + "/docs/..":                      ErrUnavailable,
		parent:                                 files.ErrPathEscape,
		filepath.Join(parent, "outside"):       files.ErrPathEscape,
		filepath.Join(root, "escape"):          files.ErrPathEscape,
		filepath.Join(root, "node_modules"):    ErrUnavailable,
		filepath.Join(repo, ".git"):            ErrUnavailable,
		filepath.Join(root, "missing"):         ErrUnavailable,
		filepath.Join(root, "file.txt"):        ErrUnavailable,
		filepath.Join(repo, "packages"):        ErrInsideRepository,
		filepath.Join(repo, "packages", "app"): ErrInsideRepository,
	} {
		if _, err := Resolve(roots, id, defaultExcluded); !errors.Is(err, want) {
			t.Errorf("Resolve(%q) error = %v, want %v", id, err, want)
		}
	}

	// A repository root opens only whole.
	makeRepo(t, root)
	if _, err := Resolve(roots, filepath.Join(root, "docs"), defaultExcluded); !errors.Is(err, ErrInsideRepository) {
		t.Fatalf("Resolve(folder in repository root) error = %v", err)
	}
	if _, err := Resolve(roots, root, defaultExcluded); err != nil {
		t.Fatalf("Resolve(repository root) error = %v", err)
	}
}

func TestDirectoriesListsBrowsableChildren(t *testing.T) {
	root := canonicalTemp(t)
	work := filepath.Join(root, "work")
	makeRepo(t, filepath.Join(work, "api"))
	writeFile(t, filepath.Join(work, ".config", "settings"), "")
	writeFile(t, filepath.Join(work, "Apps", "readme.md"), "")
	writeFile(t, filepath.Join(work, "node_modules", "pkg", "index.js"), "")
	writeFile(t, filepath.Join(work, "file.txt"), "")
	if err := os.Symlink(filepath.Join(work, "Apps"), filepath.Join(work, "link")); err != nil {
		t.Fatal(err)
	}

	got, err := Directories([]string{root}, work, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	want := []Directory{
		{Name: ".config", Path: filepath.Join(work, ".config")},
		{Name: "api", Path: filepath.Join(work, "api"), IsProject: true},
		{Name: "Apps", Path: filepath.Join(work, "Apps")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Directories() = %#v\nwant %#v", got, want)
	}
	top, err := Directories([]string{root}, root, defaultExcluded)
	if err != nil || !reflect.DeepEqual(top, []Directory{{Name: "work", Path: work}}) {
		t.Fatalf("root Directories() = %#v, %v", top, err)
	}
	roots, err := Directories([]string{root}, "", defaultExcluded)
	if err != nil || !reflect.DeepEqual(roots, []Directory{{Name: filepath.Base(root), Path: root}}) {
		t.Fatalf("roots Directories() = %#v, %v", roots, err)
	}
	for id, want := range map[string]error{
		filepath.Dir(root):                 files.ErrPathEscape,
		"work":                             ErrUnavailable,
		filepath.Join(work, "api"):         ErrInsideRepository,
		filepath.Join(work, "api", ".git"): ErrUnavailable,
	} {
		if _, err := Directories([]string{root}, id, defaultExcluded); !errors.Is(err, want) {
			t.Errorf("Directories(%q) error = %v, want %v", id, err, want)
		}
	}
}

func TestDiscoveredProjectsResolve(t *testing.T) {
	parent := canonicalTemp(t)
	root := filepath.Join(parent, "root")
	makeRepo(t, filepath.Join(root, "a"))
	makeRepo(t, filepath.Join(root, "b", "c"))
	linkWorktree(t, filepath.Join(root, "a"), "wt", filepath.Join(root, "a-wt"))
	linkWorktree(t, filepath.Join(root, "a"), "far", filepath.Join(parent, "far"))

	discovery, err := Discover([]string{root}, 10, defaultExcluded)
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range discovery.Projects {
		if _, err := Resolve([]string{root}, project.ID, defaultExcluded); err != nil {
			t.Errorf("Resolve(%q) error = %v", project.ID, err)
		}
	}
}

func TestCanonicalRoots(t *testing.T) {
	parent := canonicalTemp(t)
	a, b := filepath.Join(parent, "a"), filepath.Join(parent, "b")
	writeFile(t, filepath.Join(a, "inner", "file"), "")
	writeFile(t, filepath.Join(b, "file"), "")
	if err := os.Symlink(a, filepath.Join(parent, "link")); err != nil {
		t.Fatal(err)
	}

	got, err := CanonicalRoots([]string{b, filepath.Join(parent, "link"), a, b + "/"})
	if err != nil || !reflect.DeepEqual(got, []string{b, a}) {
		t.Fatalf("CanonicalRoots() = %v, %v; want deduplicated [b a]", got, err)
	}
	for _, paths := range [][]string{
		{a, filepath.Join(a, "inner")},
		{filepath.Join(a, "inner"), a},
		{filepath.Join(parent, "missing")},
		{filepath.Join(b, "file")},
		{},
	} {
		if _, err := CanonicalRoots(paths); err == nil {
			t.Errorf("CanonicalRoots(%v) error = nil", paths)
		}
	}
}

func TestBranchReadsHead(t *testing.T) {
	root := canonicalTemp(t)
	for dir, head := range map[string]string{
		"named":    "ref: refs/heads/feat/x\n",
		"other":    "ref: refs/remotes/origin/main\n",
		"detached": "0123456789abcdef0123456789abcdef01234567\n",
	} {
		writeFile(t, filepath.Join(root, dir, ".git", "HEAD"), head)
	}
	writeFile(t, filepath.Join(root, "plain", "file"), "")
	for dir, want := range map[string]string{
		"named":    "feat/x",
		"other":    "refs/remotes/origin/main",
		"detached": "0123456",
		"plain":    "",
		"missing":  "",
	} {
		if got := Branch(filepath.Join(root, dir)); got != want {
			t.Errorf("Branch(%s) = %q, want %q", dir, got, want)
		}
	}
}

func TestInspectDescribesCheckout(t *testing.T) {
	root := canonicalTemp(t)
	main := filepath.Join(root, "servef", "main")
	makeRepo(t, main)
	writeFile(t, filepath.Join(main, ".git", "config"), "[core]\n\tbare = false\n[remote \"upstream\"]\n\turl = git@github.com:other/fork.git\n[remote \"origin\"]\n\turl = git@github.com:owner/servef.git\n")
	feature := linkWorktree(t, main, "feature", filepath.Join(root, "servef", "wt-feature"))
	solo := filepath.Join(root, "solo")
	makeRepo(t, solo)
	folder := filepath.Join(root, "notes")
	writeFile(t, filepath.Join(folder, "a.md"), "")

	for dir, want := range map[string]Checkout{
		main:    {Kind: KindRepo, Branch: "main", Repository: "servef", Worktrees: 2},
		feature: {Kind: KindWorktree, Branch: "feature", Repository: "servef", Worktrees: 2},
		solo:    {Kind: KindRepo, Branch: "main", Repository: "solo", Worktrees: 1},
		folder:  {Kind: KindFolder},
	} {
		if got := Inspect(dir); got != want {
			t.Errorf("Inspect(%s) = %#v, want %#v", dir, got, want)
		}
	}
}

func TestInspectNamesRepositoryByOriginURL(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:owner/servef.git":  "servef",
		"https://github.com/owner/servef/": "servef",
		"ssh://host/srv/git/servef.git":    "servef",
		"../servef":                        "servef",
		`"/srv/servef.git"`:                "servef",
	} {
		dir := filepath.Join(canonicalTemp(t), "checkout")
		makeRepo(t, dir)
		writeFile(t, filepath.Join(dir, ".git", "config"), "[remote \"origin\"]\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n\turl = "+url+"\n")
		if got := Inspect(dir).Repository; got != want {
			t.Errorf("Inspect(origin %q).Repository = %q, want %q", url, got, want)
		}
	}
}

func TestDiscoverRejectsMissingRoot(t *testing.T) {
	if _, err := Discover([]string{filepath.Join(t.TempDir(), "missing")}, 10, nil); err == nil {
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

// linkWorktree writes the files git worktree add creates for a linked worktree
// checked out on a branch named after it, and returns its directory.
func linkWorktree(t *testing.T, main, name, dir string) string {
	t.Helper()
	admin := filepath.Join(main, ".git", "worktrees", name)
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(dir, ".git")+"\n")
	writeFile(t, filepath.Join(admin, "commondir"), "../..\n")
	writeFile(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/"+name+"\n")
	writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+admin+"\n")
	return dir
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

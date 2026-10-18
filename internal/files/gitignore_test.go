package files

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeContent(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// isolateGitConfig points git's system, XDG, and global config at an empty
// home so the host's excludes do not leak into scans. It returns the home.
func isolateGitConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home
}

func ignoredFiles(t *testing.T, root string) []string {
	t.Helper()
	index, err := Scan(root, -1, []string{".git"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	ignored := []string{}
	for _, name := range index.Files {
		if index.Ignored(name) {
			ignored = append(ignored, name)
		}
	}
	return ignored
}

func TestScanMarksGitignoredFilesButKeepsThemBrowsable(t *testing.T) {
	isolateGitConfig(t)
	root := t.TempDir()
	writeContent(t, filepath.Join(root, ".gitignore"), "# build output\ndist/\n*.gen.md\n!keep.gen.md\n")
	writeContent(t, filepath.Join(root, ".git", "info", "exclude"), "local.md\n")
	writeContent(t, filepath.Join(root, "docs", ".gitignore"), "draft.md\n")
	for _, name := range []string{
		"README.md", "local.md", "api.gen.md", "keep.gen.md",
		"dist/out.md", "dist/nested/deep.md",
		"docs/guide.md", "docs/draft.md", "draft.md",
	} {
		writeFile(t, filepath.Join(root, name))
	}

	index, err := Scan(root, -1, []string{".git"}, markdownFile)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(index.Files) != 9 {
		t.Fatalf("Files = %v, want all nine files browsable", index.Files)
	}
	var ignored []string
	for _, name := range index.Files {
		if index.Ignored(name) {
			ignored = append(ignored, name)
		}
	}
	want := []string{"dist/nested/deep.md", "dist/out.md", "docs/draft.md", "api.gen.md", "local.md"}
	if !reflect.DeepEqual(ignored, want) {
		t.Fatalf("ignored = %v, want %v", ignored, want)
	}
}

func TestScanSkipsExcludeFileOutsideRepository(t *testing.T) {
	isolateGitConfig(t)
	root := t.TempDir()
	// A .git file whose git directory is missing names no repository.
	writeContent(t, filepath.Join(root, ".git"), "gitdir: /elsewhere\n")
	writeFile(t, filepath.Join(root, "notes.md"))

	index, err := Scan(root, -1, nil, markdownFile)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if index.Ignored("notes.md") {
		t.Fatal("notes.md ignored without any gitignore rule")
	}
}

func TestScanAppliesDefaultGlobalExcludesInRepository(t *testing.T) {
	home := isolateGitConfig(t)
	writeContent(t, filepath.Join(home, ".config", "git", "ignore"), "*.local.md\n.claude/settings.local.json\n")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeContent(t, filepath.Join(root, ".gitignore"), "!keep.local.md\n")
	for _, name := range []string{"README.md", "notes.local.md", "keep.local.md", ".claude/settings.local.json", ".claude/settings.json"} {
		writeFile(t, filepath.Join(root, name))
	}

	want := []string{".claude/settings.local.json", "notes.local.md"}
	if got := ignoredFiles(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("ignored = %v, want %v", got, want)
	}
}

func TestScanAppliesConfiguredExcludesFileWithRepositoryPrecedence(t *testing.T) {
	home := isolateGitConfig(t)
	writeContent(t, filepath.Join(home, ".config", "git", "ignore"), "default.md\n")
	writeContent(t, filepath.Join(home, ".config", "git", "config"), "[core]\n\texcludesFile = ~/xdg-ignore\n")
	writeContent(t, filepath.Join(home, ".gitconfig"), "[core]\n\texcludesfile = ~/global-ignore\n")
	writeContent(t, filepath.Join(home, "xdg-ignore"), "xdg.md\n")
	writeContent(t, filepath.Join(home, "global-ignore"), "global.md\n")
	writeContent(t, filepath.Join(home, "repo-ignore"), "repo.md\n")

	root := t.TempDir()
	for _, name := range []string{"default.md", "xdg.md", "global.md", "repo.md"} {
		writeFile(t, filepath.Join(root, name))
	}
	writeContent(t, filepath.Join(root, ".git", "config"), "[user]\n\tname = x\n")
	if got, want := ignoredFiles(t, root), []string{"global.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("global config: ignored = %v, want %v", got, want)
	}

	writeContent(t, filepath.Join(root, ".git", "config"), "[core]\n\tExcludesFile = "+filepath.Join(home, "repo-ignore")+"\n")
	if got, want := ignoredFiles(t, root), []string{"repo.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("repository config: ignored = %v, want %v", got, want)
	}
}

func TestScanSkipsGlobalExcludesOutsideRepository(t *testing.T) {
	home := isolateGitConfig(t)
	writeContent(t, filepath.Join(home, ".config", "git", "ignore"), "*.md\n")
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "notes.md"))

	if got := ignoredFiles(t, root); len(got) != 0 {
		t.Fatalf("ignored = %v, want none outside a repository", got)
	}
}

func TestScanAppliesCommonExcludesInLinkedWorktree(t *testing.T) {
	home := isolateGitConfig(t)
	writeContent(t, filepath.Join(home, ".config", "git", "ignore"), "global.md\n")
	parent := t.TempDir()
	common := filepath.Join(parent, "main", ".git")
	admin := filepath.Join(common, "worktrees", "feature")
	worktree := filepath.Join(parent, "feature")
	writeContent(t, filepath.Join(common, "info", "exclude"), "local.md\n")
	writeContent(t, filepath.Join(admin, "commondir"), "../..\n")
	writeContent(t, filepath.Join(admin, "gitdir"), filepath.Join(worktree, ".git")+"\n")
	writeContent(t, filepath.Join(worktree, ".git"), "gitdir: ../main/.git/worktrees/feature\n")
	for _, name := range []string{"README.md", "local.md", "global.md"} {
		writeFile(t, filepath.Join(worktree, name))
	}

	// The .git file is never tracked, so search skips it too.
	want := []string{".git", "global.md", "local.md"}
	if got := ignoredFiles(t, worktree); !reflect.DeepEqual(got, want) {
		t.Fatalf("ignored = %v, want %v", got, want)
	}
}

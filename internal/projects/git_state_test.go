package projects

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadGitStateOfARepository(t *testing.T) {
	repo := canonicalTemp(t)
	makeRepo(t, repo)
	writeFile(t, filepath.Join(repo, ".git", "refs", "heads", "feature", "x"), "abc\n")
	gitDir := filepath.Join(repo, ".git")

	state, ok := ReadGitState(repo)
	if !ok {
		t.Fatal("ReadGitState() = false, want a repository")
	}
	want := []string{gitDir, filepath.Join(gitDir, "refs"), filepath.Join(gitDir, "refs", "heads"), filepath.Join(gitDir, "refs", "heads", "feature")}
	if !reflect.DeepEqual(state.Directories, want) {
		t.Fatalf("Directories = %v, want %v", state.Directories, want)
	}
	for name, want := range map[string]bool{
		filepath.Join(gitDir, "HEAD"):                          true,
		filepath.Join(gitDir, "index"):                         true,
		filepath.Join(gitDir, "packed-refs"):                   true,
		filepath.Join(gitDir, "refs", "heads", "feature", "x"): true,
		filepath.Join(gitDir, "index.lock"):                    false,
		filepath.Join(gitDir, "refs", "heads", "main.lock"):    false,
		filepath.Join(gitDir, "FETCH_HEAD"):                    false,
		filepath.Join(gitDir, "objects", "ab"):                 false,
		filepath.Join(repo, "HEAD"):                            false,
	} {
		if got := state.Records(name); got != want {
			t.Errorf("Records(%s) = %v, want %v", name, got, want)
		}
	}
}

func TestReadGitStateOfALinkedWorktree(t *testing.T) {
	main := canonicalTemp(t)
	makeRepo(t, main)
	worktree := linkWorktree(t, main, "feature", filepath.Join(canonicalTemp(t), "feature"))
	admin := filepath.Join(main, ".git", "worktrees", "feature")
	common := filepath.Join(main, ".git")

	state, ok := ReadGitState(worktree)
	if !ok {
		t.Fatal("ReadGitState() = false, want a worktree")
	}
	if want := []string{admin, common}; !reflect.DeepEqual(state.Directories, want) {
		t.Fatalf("Directories = %v, want %v", state.Directories, want)
	}
	// HEAD and the index are the worktree's own; refs are shared.
	for name, want := range map[string]bool{
		filepath.Join(admin, "HEAD"):                  true,
		filepath.Join(admin, "index"):                 true,
		filepath.Join(common, "HEAD"):                 false,
		filepath.Join(common, "packed-refs"):          true,
		filepath.Join(common, "refs", "heads", "dev"): true,
	} {
		if got := state.Records(name); got != want {
			t.Errorf("Records(%s) = %v, want %v", name, got, want)
		}
	}
}

func TestReadGitStateOfAFolder(t *testing.T) {
	if _, ok := ReadGitState(canonicalTemp(t)); ok {
		t.Fatal("ReadGitState() = true for a folder")
	}
}

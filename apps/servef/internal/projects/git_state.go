package projects

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/servef/internal/files"
)

// GitState names where a checkout's git state lives: HEAD and the index in
// its admin directory (.git, or a linked worktree's directory under the
// common one), and packed-refs and refs in the common directory. Watching
// Directories sees every change to them.
type GitState struct {
	// Directories are the admin and common directories and every directory
	// below refs, absolute.
	Directories []string
	admin       string
	common      string
}

// ReadGitState locates a checkout's git state. It reports false for a folder
// that is not a worktree.
func ReadGitState(dir string) (GitState, bool) {
	admin, err := adminDir(dir)
	if err != nil {
		return GitState{}, false
	}
	common, err := commonDir(dir)
	if err != nil {
		return GitState{}, false
	}
	state := GitState{admin: admin, common: common, Directories: []string{admin}}
	if common != admin {
		state.Directories = append(state.Directories, common)
	}
	_ = filepath.WalkDir(filepath.Join(common, "refs"), func(name string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			state.Directories = append(state.Directories, name)
		}
		return nil
	})
	return state, true
}

// Records reports whether the absolute path name is one of the state's
// files: HEAD, the index, packed-refs, or a ref. Lock files git writes
// before renaming them into place are not.
func (s GitState) Records(name string) bool {
	if s.common == "" || strings.HasSuffix(name, ".lock") {
		return false
	}
	if base := filepath.Base(name); filepath.Dir(name) == s.admin && (base == "HEAD" || base == "index") {
		return true
	}
	return name == filepath.Join(s.common, "packed-refs") || files.Within(filepath.Join(s.common, "refs"), name)
}

// adminDir returns a worktree's own git directory: .git itself, or the
// directory a .git file names.
func adminDir(dir string) (string, error) {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return canonical(gitPath), nil
	}
	return readGitFile(gitPath)
}

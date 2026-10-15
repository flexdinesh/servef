package files

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// ignoreRules are the gitignore patterns in effect for a directory: the
// root's .git/info/exclude, then each .gitignore from the root down. Later
// patterns take precedence, as in git.
type ignoreRules struct {
	patterns []gitignore.Pattern
	matcher  gitignore.Matcher
}

// rootIgnoreRules loads the repository-local exclude file when root holds a
// .git directory. Linked worktrees (a .git file) and global excludes are not
// read.
func rootIgnoreRules(root string) ignoreRules {
	info, err := os.Stat(filepath.Join(root, ".git"))
	if err != nil || !info.IsDir() {
		return ignoreRules{}
	}
	return ignoreRules{}.with(filepath.Join(root, ".git", "info", "exclude"), nil)
}

// with returns rules extended by the patterns in file, scoped to the
// slash-separated directory components in domain. Unreadable files add
// nothing.
func (r ignoreRules) with(file string, domain []string) ignoreRules {
	content, err := os.ReadFile(file)
	if err != nil {
		return r
	}
	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	if len(patterns) == 0 {
		return r
	}
	combined := append(append([]gitignore.Pattern{}, r.patterns...), patterns...)
	return ignoreRules{patterns: combined, matcher: gitignore.NewMatcher(combined)}
}

// ignores reports whether the slash-separated root-relative path is ignored.
func (r ignoreRules) ignores(rel string, isDir bool) bool {
	return r.matcher != nil && r.matcher.Match(strings.Split(rel, "/"), isDir)
}

func domainOf(rel string) []string {
	if rel == "" {
		return nil
	}
	return strings.Split(rel, "/")
}

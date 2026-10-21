package files

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// ignoreRules are the gitignore patterns in effect for a directory: the
// root repository's core.excludesFile and info/exclude, then each .gitignore
// from the root down. Later patterns take precedence, as in git.
type ignoreRules struct {
	patterns []gitignore.Pattern
	matcher  gitignore.Matcher
}

// IgnoreRules answers git ignore questions for paths below one root. It reads
// the sources git uses (see rootIgnoreRules), loading each directory's
// .gitignore on first use. It is not safe for concurrent use.
type IgnoreRules struct {
	root string
	base ignoreRules
	dirs map[string]ignoreRules
}

// NewIgnoreRules returns the ignore rules for the absolute, symlink-evaluated
// directory root.
func NewIgnoreRules(root string) *IgnoreRules {
	return &IgnoreRules{root: root, base: rootIgnoreRules(root), dirs: map[string]ignoreRules{}}
}

// Ignored reports whether the slash-separated root-relative path is ignored,
// itself or through an ignored ancestor directory, which git never re-includes
// below. A .git entry is always ignored.
func (r *IgnoreRules) Ignored(rel string, isDir bool) bool {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if part == ".git" {
			return true
		}
		last := i == len(parts)-1
		if r.ignoredIn(strings.Join(parts[:i], "/"), strings.Join(parts[:i+1], "/"), isDir || !last) {
			return true
		}
	}
	return false
}

// ignoredIn reports whether rel, a child of the directory parent, is ignored by
// the rules in effect in parent. Callers check parent's ancestors.
func (r *IgnoreRules) ignoredIn(parent, rel string, isDir bool) bool {
	return r.rulesFor(parent).ignores(rel, isDir)
}

// rulesFor returns the patterns in effect in the root-relative directory dir:
// its ancestors' patterns, then its own .gitignore when that is a regular file.
func (r *IgnoreRules) rulesFor(dir string) ignoreRules {
	if rules, ok := r.dirs[dir]; ok {
		return rules
	}
	rules := r.base
	if dir != "" {
		parent := ""
		if i := strings.LastIndex(dir, "/"); i >= 0 {
			parent = dir[:i]
		}
		rules = r.rulesFor(parent)
	}
	name := filepath.Join(r.root, filepath.FromSlash(dir), ".gitignore")
	if info, err := os.Lstat(name); err == nil && info.Mode().IsRegular() {
		rules = rules.with(name, domainOf(dir))
	}
	r.dirs[dir] = rules
	return rules
}

// rootIgnoreRules loads the patterns git reads before any .gitignore when
// root is a repository or linked worktree: core.excludesFile, then the
// common git directory's info/exclude. Outside a repository only .gitignore
// files apply.
func rootIgnoreRules(root string) ignoreRules {
	common, ok := commonGitDir(root)
	if !ok {
		return ignoreRules{}
	}
	var rules ignoreRules
	if file := excludesFile(root, common); file != "" {
		rules = rules.with(file, nil)
	}
	return rules.with(filepath.Join(common, "info", "exclude"), nil)
}

// commonGitDir returns the git directory shared by root's worktrees: root's
// .git directory, or the directory a .git file names, followed through its
// commondir file when root is a linked worktree.
func commonGitDir(root string) (string, bool) {
	gitPath := filepath.Join(root, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return gitPath, true
	}
	if !info.Mode().IsRegular() {
		return "", false
	}
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
	if !ok {
		return "", false
	}
	admin := resolveFrom(root, strings.TrimSpace(target))
	if common, err := os.ReadFile(filepath.Join(admin, "commondir")); err == nil {
		admin = resolveFrom(admin, strings.TrimSpace(string(common)))
	}
	info, err = os.Stat(admin)
	return admin, err == nil && info.IsDir()
}

// excludesFile returns the core.excludesFile git uses for the repository
// whose git directory is common: the last value set in the system, XDG,
// global, then repository config, or $XDG_CONFIG_HOME/git/ignore when none
// is set. Config includes are not followed.
func excludesFile(root, common string) string {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" && home != "" {
		xdg = filepath.Join(home, ".config")
	}
	var configs []string
	if !envBool("GIT_CONFIG_NOSYSTEM") {
		configs = append(configs, "/etc/gitconfig")
	}
	if xdg != "" {
		configs = append(configs, filepath.Join(xdg, "git", "config"))
	}
	if home != "" {
		configs = append(configs, filepath.Join(home, ".gitconfig"))
	}
	configs = append(configs, filepath.Join(common, "config"))

	value, set := "", false
	for _, name := range configs {
		if v, ok := coreOption(name, "excludesFile"); ok {
			value, set = v, true
		}
	}
	switch {
	case !set && xdg != "":
		return filepath.Join(xdg, "git", "ignore")
	case !set || value == "":
		return ""
	case value == "~" || strings.HasPrefix(value, "~/"):
		if home == "" {
			return ""
		}
		return filepath.Join(home, value[1:])
	default:
		return resolveFrom(root, value)
	}
}

// coreOption reads key from the [core] section of the git config file name.
func coreOption(name, key string) (string, bool) {
	content, err := os.ReadFile(name)
	if err != nil {
		return "", false
	}
	raw := config.New()
	if err := config.NewDecoder(bytes.NewReader(content)).Decode(raw); err != nil {
		return "", false
	}
	if !raw.HasSection("core") || !raw.Section("core").HasOption(key) {
		return "", false
	}
	return raw.Section("core").Option(key), true
}

func envBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value != "" && value != "0" && value != "false" && value != "no" && value != "off"
}

func resolveFrom(base, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(base, name)
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

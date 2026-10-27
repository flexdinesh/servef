// Package git reads a checkout's uncommitted changes from git's own files:
// HEAD, refs, objects, and the index. It never runs git and knows no HTTP or
// document policy.
//
// Changes compare HEAD, or a branch's merge base, with the working tree. Clean and smudge filters are not
// applied, submodules and nested repositories are skipped, and renames are
// exact content matches only.
package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"

	"github.com/flexdinesh/servef/internal/files"
)

// Status describes how a path differs from HEAD.
type Status string

const (
	Modified  Status = "modified"
	Added     Status = "added"
	Untracked Status = "untracked"
	Deleted   Status = "deleted"
	Renamed   Status = "renamed"
)

var (
	// ErrNotRepository is returned when a directory has no .git entry naming
	// a git directory.
	ErrNotRepository = errors.New("not a git repository")
	// ErrUnchanged is returned for a tracked path identical to HEAD.
	ErrUnchanged = errors.New("no changes since HEAD")
	// ErrUnavailable is returned for a path that is not a change.
	ErrUnavailable = errors.New("not a change")
)

const (
	// objectCacheSize bounds decoded objects kept while a repository is open.
	objectCacheSize = 8 * cache.MiByte
	// largeObjectSize is the size above which packed objects stream instead
	// of being read into memory.
	largeObjectSize = 1 << 20
	// cancelCheck is how many paths are compared between context checks.
	cancelCheck = 256
)

// Change is one changed path. Paths are slash-separated and relative to the
// worktree root; OldPath is set for renames only.
type Change struct {
	Path    string `json:"path"`
	OldPath string `json:"oldPath,omitempty"`
	Status  Status `json:"status"`
}

// StatusOptions bound a status read.
type StatusOptions struct {
	// Limit caps the changes returned; zero or less returns all.
	Limit int
}

// Changes is a worktree's changes since HEAD, sorted by path in byte order.
type Changes struct {
	Files []Change
	// Truncated reports that Limit dropped changes.
	Truncated bool
	// Warnings are directories the untracked walk could not read.
	Warnings []files.Warning
}

// Entry is a validated change and what reading its worktree side needs.
type Entry struct {
	Change
	// Symlink reports that the worktree side is a symbolic link; Link is its
	// target text, which is the content git records.
	Symlink bool
	Link    string
}

// Blob is a HEAD file's content, sized before it is read.
type Blob struct {
	Size int64

	hash    plumbing.Hash
	storage *filesystem.Storage
}

// Hash returns the blob's 40-hexadecimal-digit git hash.
func (b Blob) Hash() string { return b.hash.String() }

// BlobHash returns the git blob hash content would be stored under.
func BlobHash(content []byte) string {
	return plumbing.ComputeHash(plumbing.BlobObject, content).String()
}

// Open returns the blob's content.
func (b Blob) Open() (io.ReadCloser, error) {
	obj, err := b.storage.EncodedObject(plumbing.BlobObject, b.hash)
	if err != nil {
		return nil, err
	}
	return obj.Reader()
}

// Repository is an open worktree. Close releases its files.
type Repository struct {
	dir     string
	gitDir  string
	storage *filesystem.Storage
}

// Open opens the worktree whose root is the absolute, symlink-evaluated
// directory dir, following a linked worktree's .git file and commondir to the
// shared git directory.
func Open(dir string) (*Repository, error) {
	gitDir, err := gitDirectory(dir)
	if err != nil {
		return nil, err
	}
	var common billy.Filesystem
	if content, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		name := resolveFrom(gitDir, strings.TrimSpace(string(content)))
		if info, err := os.Stat(name); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("common git directory %s: %w", name, ErrNotRepository)
		}
		common = osfs.New(name)
	}
	storage := filesystem.NewStorageWithOptions(
		dotgit.NewRepositoryFilesystem(osfs.New(gitDir), common),
		cache.NewObjectLRU(objectCacheSize),
		filesystem.Options{LargeObjectThreshold: largeObjectSize},
	)
	return &Repository{dir: dir, gitDir: gitDir, storage: storage}, nil
}

// gitDirectory returns the git directory of the worktree at dir: its .git
// directory, or the directory its .git file names.
func gitDirectory(dir string) (string, error) {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", ErrNotRepository
	}
	if info.IsDir() {
		return gitPath, nil
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotRepository
	}
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
	if !ok {
		return "", ErrNotRepository
	}
	gitDir := resolveFrom(dir, strings.TrimSpace(target))
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return "", ErrNotRepository
	}
	return gitDir, nil
}

func resolveFrom(base, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(base, name)
}

// Close releases the repository's open files.
func (r *Repository) Close() error { return r.storage.Close() }

// Branch is the checked-out branch, or empty when HEAD is detached or
// unreadable.
func (r *Repository) Branch() string {
	head, err := r.storage.Reference(plumbing.HEAD)
	if err != nil || head.Type() != plumbing.SymbolicReference || !head.Target().IsBranch() {
		return ""
	}
	return head.Target().Short()
}

// Status compares HEAD, the index, and the working tree. Tracked paths are
// reported even when ignore rules match them; untracked files are found by
// walking the worktree without ignored directories. A deleted path and an
// added or untracked path with the same content pair as a rename.
func (r *Repository) Status(ctx context.Context, options StatusOptions) (Changes, error) {
	tree, err := r.headTree()
	if err != nil {
		return Changes{}, err
	}
	return r.status(ctx, tree, options)
}

// status compares tree, nil being empty, with the index and the working
// tree as Status does with HEAD's.
func (r *Repository) status(ctx context.Context, tree *object.Tree, options StatusOptions) (Changes, error) {
	head, err := r.treeFiles(ctx, tree)
	if err != nil {
		return Changes{}, err
	}
	staged, err := r.readIndex()
	if err != nil {
		return Changes{}, err
	}
	worktree := r.newWorktree(staged)

	paths := make([]string, 0, len(head)+len(staged.entries))
	for name := range head {
		paths = append(paths, name)
	}
	for name := range staged.entries {
		if _, ok := head[name]; !ok {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)

	var changes []Change
	var deleted []string
	var created []string
	for i, name := range paths {
		if i%cancelCheck == 0 {
			if err := ctx.Err(); err != nil {
				return Changes{}, err
			}
		}
		file, err := worktree.file(name)
		if err != nil {
			return Changes{}, err
		}
		committed, inHead := head[name]
		switch {
		case inHead && !file.exists:
			deleted = append(deleted, name)
		case inHead:
			same, err := worktree.matches(name, file, committed)
			if err != nil {
				return Changes{}, err
			}
			if !same {
				changes = append(changes, Change{Path: name, Status: Modified})
			}
		case file.exists:
			created = append(created, name)
		}
	}

	var untracked []string
	warnings, err := files.WalkUnignored(ctx, r.dir, files.NewIgnoreRules(r.dir), func(name string, _ fs.DirEntry) error {
		_, tracked := head[name]
		if _, staged := staged.entries[name]; !tracked && !staged {
			untracked = append(untracked, name)
		}
		return nil
	})
	if err != nil {
		return Changes{}, err
	}

	renamed, err := r.pairRenames(ctx, worktree, head, deleted, append(append([]string{}, created...), untracked...))
	if err != nil {
		return Changes{}, err
	}
	for _, name := range deleted {
		if _, ok := renamed.from[name]; !ok {
			changes = append(changes, Change{Path: name, Status: Deleted})
		}
	}
	for _, group := range []struct {
		names  []string
		status Status
	}{{created, Added}, {untracked, Untracked}} {
		for _, name := range group.names {
			if old, ok := renamed.to[name]; ok {
				changes = append(changes, Change{Path: name, OldPath: old, Status: Renamed})
				continue
			}
			changes = append(changes, Change{Path: name, Status: group.status})
		}
	}
	sort.Slice(changes, func(a, b int) bool { return changes[a].Path < changes[b].Path })

	result := Changes{Files: changes, Warnings: warnings}
	if result.Files == nil {
		result.Files = []Change{}
	}
	if options.Limit > 0 && len(result.Files) > options.Limit {
		result.Files = result.Files[:options.Limit]
		result.Truncated = true
	}
	return result, nil
}

type renames struct {
	// from maps an old path to its new path; to maps back.
	from map[string]string
	to   map[string]string
}

// pairRenames pairs deleted HEAD files with new worktree files of identical
// content and mode, both in path order. Only new files whose size matches a
// deleted blob are hashed.
func (r *Repository) pairRenames(ctx context.Context, worktree *worktree, head map[string]recorded, deleted, candidates []string) (renames, error) {
	result := renames{from: map[string]string{}, to: map[string]string{}}
	if len(deleted) == 0 || len(candidates) == 0 {
		return result, nil
	}
	sizes := map[int64]bool{}
	byContent := map[recorded][]string{}
	for _, name := range deleted {
		committed := head[name]
		size, err := r.storage.EncodedObjectSize(committed.hash)
		if err != nil {
			return result, err
		}
		sizes[size] = true
		key := recorded{hash: committed.hash, mode: normalMode(committed.mode)}
		byContent[key] = append(byContent[key], name)
	}
	sort.Strings(candidates)
	for i, name := range candidates {
		if i%cancelCheck == 0 {
			if err := ctx.Err(); err != nil {
				return result, err
			}
		}
		file, err := worktree.file(name)
		if err != nil {
			return result, err
		}
		if !file.exists || !sizes[file.size] {
			continue
		}
		hash, err := worktree.hash(name, file)
		if err != nil {
			return result, err
		}
		key := recorded{hash: hash, mode: file.mode}
		if olds := byContent[key]; len(olds) > 0 {
			result.from[olds[0]] = name
			result.to[name] = olds[0]
			byContent[key] = olds[1:]
		}
	}
	return result, nil
}

// Lookup classifies one path without walking the worktree. With oldPath it
// validates a rename: oldPath is in HEAD and gone from the worktree, and path
// is a new file with oldPath's content. It returns ErrUnchanged for a tracked
// path identical to HEAD and ErrUnavailable for anything else that is not a
// change.
func (r *Repository) Lookup(ctx context.Context, name, oldPath string) (Entry, error) {
	if !validPath(name) || (oldPath != "" && (!validPath(oldPath) || oldPath == name)) {
		return Entry{}, ErrUnavailable
	}
	tree, err := r.headTree()
	if err != nil {
		return Entry{}, err
	}
	return r.lookup(ctx, tree, name, oldPath)
}

// lookup classifies one valid path against tree, nil being empty, as Lookup
// does against HEAD's.
func (r *Repository) lookup(ctx context.Context, tree *object.Tree, name, oldPath string) (Entry, error) {
	staged, err := r.readIndex()
	if err != nil {
		return Entry{}, err
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	worktree := r.newWorktree(staged)
	file, err := worktree.file(name)
	if err != nil {
		return Entry{}, err
	}
	committed, inHead, err := treeFile(r.storage, tree, name)
	if err != nil {
		return Entry{}, err
	}
	_, inIndex := staged.entries[name]
	entry := Entry{Change: Change{Path: name}, Symlink: file.mode == filemode.Symlink, Link: file.link}
	isNew := !inHead && file.exists && (inIndex || r.untracked(name))

	if oldPath != "" {
		old, oldInHead, err := treeFile(r.storage, tree, oldPath)
		if err != nil {
			return Entry{}, err
		}
		oldFile, err := worktree.file(oldPath)
		if err != nil {
			return Entry{}, err
		}
		if !oldInHead || oldFile.exists || !isNew {
			return Entry{}, ErrUnavailable
		}
		hash, err := worktree.hash(name, file)
		if err != nil {
			return Entry{}, err
		}
		if hash != old.hash || file.mode != normalMode(old.mode) {
			return Entry{}, ErrUnavailable
		}
		entry.OldPath, entry.Status = oldPath, Renamed
		return entry, nil
	}

	switch {
	case inHead && !file.exists:
		entry.Status = Deleted
	case inHead:
		same, err := worktree.matches(name, file, committed)
		if err != nil {
			return Entry{}, err
		}
		if same {
			return Entry{}, ErrUnchanged
		}
		entry.Status = Modified
	case isNew && inIndex:
		entry.Status = Added
	case isNew:
		entry.Status = Untracked
	default:
		return Entry{}, ErrUnavailable
	}
	return entry, nil
}

// Entries completes changes Status listed with what reading their worktree
// sides needs: whether each path is now a symbolic link, and its target
// text. Deleted changes have no worktree side and are returned as they are.
func (r *Repository) Entries(changes []Change) ([]Entry, error) {
	worktree := r.newWorktree(stagedFiles{entries: map[string]stagedEntry{}})
	entries := make([]Entry, len(changes))
	for i, change := range changes {
		entries[i].Change = change
		if change.Status == Deleted || !validPath(change.Path) {
			continue
		}
		file, err := worktree.file(change.Path)
		if err != nil {
			return nil, err
		}
		entries[i].Symlink, entries[i].Link = file.mode == filemode.Symlink, file.link
	}
	return entries, nil
}

// untracked reports whether git would list name as untracked if it is not
// in the index: no ignore rule matches it and no directory above it is a
// nested repository.
func (r *Repository) untracked(name string) bool {
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		if _, err := os.Lstat(filepath.Join(r.dir, filepath.Join(parts[:i]...), ".git")); err == nil {
			return false
		}
	}
	return !files.NewIgnoreRules(r.dir).Ignored(name, false)
}

// HeadBlob returns the file name has in HEAD. It returns ErrUnavailable when
// HEAD has no file there.
func (r *Repository) HeadBlob(name string) (Blob, error) {
	if !validPath(name) {
		return Blob{}, ErrUnavailable
	}
	tree, err := r.headTree()
	if err != nil {
		return Blob{}, err
	}
	committed, ok, err := treeFile(r.storage, tree, name)
	if err != nil {
		return Blob{}, err
	}
	if !ok {
		return Blob{}, ErrUnavailable
	}
	size, err := r.storage.EncodedObjectSize(committed.hash)
	if err != nil {
		return Blob{}, err
	}
	return Blob{Size: size, hash: committed.hash, storage: r.storage}, nil
}

// validPath reports whether name is a clean, slash-separated relative path
// outside any .git directory.
func validPath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".git" || part == "." {
			return false
		}
	}
	return true
}

// recorded is a blob as git records it: content hash and file mode.
type recorded struct {
	hash plumbing.Hash
	mode filemode.FileMode
}

// normalMode folds the deprecated group-writable mode into Regular.
func normalMode(mode filemode.FileMode) filemode.FileMode {
	if mode == filemode.Deprecated {
		return filemode.Regular
	}
	return mode
}

// headTree returns HEAD's tree, or nil for an unborn branch.
func (r *Repository) headTree() (*object.Tree, error) {
	ref, err := storer.ResolveReference(r.storage, plumbing.HEAD)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read HEAD: %w", err)
	}
	commit, err := object.GetCommit(r.storage, ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("read HEAD commit: %w", err)
	}
	return commit.Tree()
}

// treeFile finds the file name in tree, a nil tree being empty. Directories
// and submodules are not files, and nothing lies below a submodule.
func treeFile(storage *filesystem.Storage, tree *object.Tree, name string) (recorded, bool, error) {
	parts := strings.Split(name, "/")
	for i, part := range parts {
		if tree == nil {
			return recorded{}, false, nil
		}
		var found *object.TreeEntry
		for j := range tree.Entries {
			if tree.Entries[j].Name == part {
				found = &tree.Entries[j]
				break
			}
		}
		switch {
		case found == nil:
			return recorded{}, false, nil
		case i == len(parts)-1:
			if !found.Mode.IsFile() {
				return recorded{}, false, nil
			}
			return recorded{hash: found.Hash, mode: found.Mode}, true, nil
		case found.Mode != filemode.Dir:
			return recorded{}, false, nil
		}
		child, err := object.GetTree(storage, found.Hash)
		if err != nil {
			return recorded{}, false, fmt.Errorf("read tree %s: %w", strings.Join(parts[:i+1], "/"), err)
		}
		tree = child
	}
	return recorded{}, false, nil
}

// treeFiles lists a tree's files by path, skipping submodules; a nil tree
// is empty.
func (r *Repository) treeFiles(ctx context.Context, tree *object.Tree) (map[string]recorded, error) {
	result := map[string]recorded{}
	if tree == nil {
		return result, nil
	}
	var walk func(tree *object.Tree, prefix string) error
	walk = func(tree *object.Tree, prefix string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, entry := range tree.Entries {
			name := prefix + entry.Name
			switch {
			case entry.Mode == filemode.Dir:
				child, err := object.GetTree(r.storage, entry.Hash)
				if err != nil {
					return fmt.Errorf("read tree %s: %w", name, err)
				}
				if err := walk(child, name+"/"); err != nil {
					return err
				}
			case entry.Mode.IsFile():
				result[name] = recorded{hash: entry.Hash, mode: entry.Mode}
			}
		}
		return nil
	}
	return result, walk(tree, "")
}

// stagedFiles are the index's file entries by path and the index file's
// modification time, which bounds which entries' metadata can be trusted.
type stagedFiles struct {
	entries  map[string]stagedEntry
	modified time.Time
}

type stagedEntry struct {
	entry *index.Entry
	// trusted reports a single stage-0 entry whose hash describes the file
	// when its stat data still matches.
	trusted bool
}

// readIndex reads the index; a missing index is empty. Submodule entries are
// skipped.
func (r *Repository) readIndex() (stagedFiles, error) {
	staged := stagedFiles{entries: map[string]stagedEntry{}}
	idx, err := r.storage.Index()
	if err != nil {
		return staged, fmt.Errorf("read index: %w", err)
	}
	if info, err := os.Stat(filepath.Join(r.gitDir, "index")); err == nil {
		staged.modified = info.ModTime()
	}
	for _, entry := range idx.Entries {
		if !entry.Mode.IsFile() {
			continue
		}
		_, seen := staged.entries[entry.Name]
		// Unconflicted entries are stage 0; go-git's index.Merged constant is 1.
		trusted := !seen && entry.Stage == 0 && !entry.IntentToAdd
		staged.entries[entry.Name] = stagedEntry{entry: entry, trusted: trusted}
	}
	return staged, nil
}

// worktree inspects working-tree files, caching which directories are real
// directories so a path behind a symlinked directory reads as missing, as in
// git.
type worktree struct {
	dir    string
	staged stagedFiles
	dirs   map[string]bool
}

func (r *Repository) newWorktree(staged stagedFiles) *worktree {
	return &worktree{dir: r.dir, staged: staged, dirs: map[string]bool{"": true}}
}

// worktreeFile is a path's state on disk. hash is set when the index entry's
// stat data proves it.
type worktreeFile struct {
	exists bool
	mode   filemode.FileMode
	size   int64
	link   string
	hash   plumbing.Hash
}

func (w *worktree) file(name string) (worktreeFile, error) {
	if staged, ok := w.staged.entries[name]; ok && staged.trusted && staged.entry.SkipWorktree {
		// Sparse checkouts leave skip-worktree paths out; git compares their
		// index state instead.
		entry := staged.entry
		return worktreeFile{exists: true, mode: entry.Mode, size: int64(entry.Size), hash: entry.Hash}, nil
	}
	if !w.directory(path.Dir(name)) {
		return worktreeFile{}, nil
	}
	info, err := os.Lstat(filepath.Join(w.dir, filepath.FromSlash(name)))
	if errors.Is(err, fs.ErrNotExist) {
		return worktreeFile{}, nil
	}
	if err != nil {
		return worktreeFile{}, err
	}
	mode, err := filemode.NewFromOSFileMode(info.Mode())
	if err != nil || !mode.IsFile() {
		// Directories, devices, and sockets are not files git tracks.
		return worktreeFile{}, nil
	}
	file := worktreeFile{exists: true, mode: mode, size: info.Size()}
	if mode == filemode.Symlink {
		if file.link, err = os.Readlink(filepath.Join(w.dir, filepath.FromSlash(name))); err != nil {
			return worktreeFile{}, err
		}
		file.size = int64(len(file.link))
	}
	if staged, ok := w.staged.entries[name]; ok && staged.trusted {
		entry := staged.entry
		// Racy git: an entry modified no earlier than the index was written
		// may have changed within the same timestamp, so it is rehashed.
		if entry.Mode == mode && int64(entry.Size) == file.size&0xffffffff &&
			entry.ModifiedAt.Equal(info.ModTime()) && entry.ModifiedAt.Before(w.staged.modified) {
			file.hash = entry.Hash
		}
	}
	return file, nil
}

// directory reports whether the slash-separated dir is a real directory, not
// a symlink, all the way down from the root.
func (w *worktree) directory(dir string) bool {
	if dir == "." {
		dir = ""
	}
	if known, ok := w.dirs[dir]; ok {
		return known
	}
	real := w.directory(path.Dir(dir))
	if real {
		info, err := os.Lstat(filepath.Join(w.dir, filepath.FromSlash(dir)))
		real = err == nil && info.IsDir()
	}
	w.dirs[dir] = real
	return real
}

// matches reports whether an existing worktree file equals committed.
func (w *worktree) matches(name string, file worktreeFile, committed recorded) (bool, error) {
	if file.mode != normalMode(committed.mode) {
		return false, nil
	}
	if file.hash.IsZero() {
		// The index's size tells a changed file from HEAD's without hashing
		// when the index still holds HEAD's content.
		if staged, ok := w.staged.entries[name]; ok && staged.trusted && staged.entry.Hash == committed.hash && int64(staged.entry.Size) != file.size&0xffffffff {
			return false, nil
		}
	}
	hash, err := w.hash(name, file)
	if err != nil {
		return false, err
	}
	return hash == committed.hash, nil
}

// hash returns the git blob hash of an existing worktree file: its content,
// or a symlink's target text.
func (w *worktree) hash(name string, file worktreeFile) (plumbing.Hash, error) {
	if !file.hash.IsZero() {
		return file.hash, nil
	}
	if file.mode == filemode.Symlink {
		return plumbing.ComputeHash(plumbing.BlobObject, []byte(file.link)), nil
	}
	handle, err := os.Open(filepath.Join(w.dir, filepath.FromSlash(name)))
	if err != nil {
		return plumbing.ZeroHash, err
	}
	defer func() { _ = handle.Close() }()
	hasher := plumbing.NewHasher(plumbing.BlobObject, file.size)
	if _, err := io.CopyN(hasher, handle, file.size); err != nil {
		if errors.Is(err, io.EOF) {
			// The file shrank while it was read, so it differs from any
			// recorded content of its old size.
			return plumbing.ZeroHash, nil
		}
		return plumbing.ZeroHash, err
	}
	return hasher.Sum(), nil
}

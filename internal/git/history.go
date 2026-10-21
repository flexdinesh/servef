package git

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// Range selects which commits Log lists.
type Range string

const (
	// RangeBranch lists commits reachable from the head and not from the
	// base branch, as git log base..head does.
	RangeBranch Range = "branch"
	// RangeAll lists the head's whole history.
	RangeAll Range = "all"
)

// walkSlop is how many more commits a branch walk takes once every queued
// commit is reachable from the base, as git does to absorb clock skew.
const walkSlop = 5

// Branch walk bounds; variables so tests can lower them.
var (
	// branchLimit caps the commits a branch range may hold. A range beyond it,
	// such as an unrelated history, is an error rather than a slow page.
	branchLimit = 10_000
	// walkLimit caps the commits a branch walk visits, counting the base
	// side's, so a head far behind its base still finishes.
	walkLimit = 100_000
)

// ErrRangeTooLarge is returned when a branch range exceeds branchLimit or its
// walk exceeds walkLimit.
var ErrRangeTooLarge = errors.New("too many commits to compare")

// rangeTooLarge names the base a range was too large to compare with.
type rangeTooLarge struct{ base string }

func (e rangeTooLarge) Error() string {
	return "the branch has too many commits to compare with " + e.base
}

func (e rangeTooLarge) Is(target error) bool { return target == ErrRangeTooLarge }

// LogOptions select and page a history read.
type LogOptions struct {
	Range Range
	// Head is the commit to walk from; zero resolves HEAD. Pinning it keeps
	// pages stable while new commits arrive.
	Head plumbing.Hash
	// Skip and Limit page the commits; a Limit of zero or less returns all.
	Skip, Limit int
}

// Author is who wrote a commit.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Commit is one commit's summary. Shallow commits have no parents.
type Commit struct {
	Hash    string
	Parents []string
	// Subject is the message's first line; Body is the rest without leading
	// blank lines or trailing whitespace.
	Subject    string
	Body       string
	Author     Author
	AuthorTime time.Time
}

// History is one page of commits, newest committer date first.
type History struct {
	// Head is the commit walked; zero when HEAD is unborn.
	Head plumbing.Hash
	// Base is the base branch's short name, or empty when none exists.
	Base string
	// Ahead counts the commits in base..Head; zero without a base.
	Ahead   int
	Commits []Commit
	// More reports that commits follow this page.
	More bool
}

// CommitChanges are a commit's changes against its first parent, sorted by
// path in byte order.
type CommitChanges struct {
	Files []Change
	// Truncated reports that the limit dropped changes.
	Truncated bool
}

// ParseHash parses a full commit hash: 40 lowercase hexadecimal digits.
func ParseHash(value string) (plumbing.Hash, bool) {
	if len(value) != 40 {
		return plumbing.ZeroHash, false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return plumbing.ZeroHash, false
		}
	}
	return plumbing.NewHash(value), true
}

// baseCandidates are the refs tried, in order, as the base branch after
// origin's HEAD.
var baseCandidates = []plumbing.ReferenceName{
	plumbing.NewBranchReferenceName("main"),
	plumbing.NewBranchReferenceName("master"),
	plumbing.NewRemoteReferenceName("origin", "main"),
	plumbing.NewRemoteReferenceName("origin", "master"),
}

// base finds the base branch: origin's HEAD target, then main, master,
// origin/main, and origin/master. It returns an empty name when none
// resolves.
func (r *Repository) base() (string, plumbing.Hash, error) {
	candidates := baseCandidates
	if ref, err := r.storage.Reference(plumbing.NewRemoteHEADReferenceName("origin")); err == nil {
		name := ref.Name()
		if ref.Type() == plumbing.SymbolicReference {
			name = ref.Target()
		}
		candidates = append([]plumbing.ReferenceName{name}, candidates...)
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return "", plumbing.ZeroHash, fmt.Errorf("read origin HEAD: %w", err)
	}
	for _, name := range candidates {
		ref, err := storer.ResolveReference(r.storage, name)
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			continue
		}
		if err != nil {
			return "", plumbing.ZeroHash, fmt.Errorf("read %s: %w", name.Short(), err)
		}
		return name.Short(), ref.Hash(), nil
	}
	return "", plumbing.ZeroHash, nil
}

// Log lists one page of history from options.Head. Commits are ordered like
// git log: newest committer date first, from a priority queue, so a parent
// can precede a child only under clock skew. It returns ErrUnavailable when a
// given Head is not a commit, and ErrRangeTooLarge when the base..Head range
// exceeds its bounds. RangeAll still lists a page then, with Ahead zero.
func (r *Repository) Log(ctx context.Context, options LogOptions) (History, error) {
	history := History{Commits: []Commit{}}
	baseName, baseHash, err := r.base()
	if err != nil {
		return History{}, err
	}
	history.Base = baseName

	head := options.Head
	if head.IsZero() {
		ref, err := storer.ResolveReference(r.storage, plumbing.HEAD)
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return history, nil
		}
		if err != nil {
			return History{}, fmt.Errorf("read HEAD: %w", err)
		}
		head = ref.Hash()
	} else if _, err := r.commit(head); err != nil {
		return History{}, err
	}
	history.Head = head

	w, err := r.newWalker(ctx)
	if err != nil {
		return History{}, err
	}
	var branch []plumbing.Hash
	if baseName != "" {
		branch, err = w.branch(head, baseHash)
		switch {
		case errors.Is(err, ErrRangeTooLarge) && options.Range == RangeAll:
			// The full history does not need the range; only Ahead is lost.
		case errors.Is(err, ErrRangeTooLarge):
			return History{}, rangeTooLarge{base: baseName}
		case err != nil:
			return History{}, err
		}
		history.Ahead = len(branch)
	}
	if options.Range == RangeAll {
		history.Commits, history.More, err = w.all(head, options.Skip, options.Limit)
		if err != nil {
			return History{}, err
		}
		return history, nil
	}

	page := branch[min(options.Skip, len(branch)):]
	if options.Limit > 0 && len(page) > options.Limit {
		page, history.More = page[:options.Limit], true
	}
	for _, hash := range page {
		commit, err := w.load(hash)
		if err != nil {
			return History{}, err
		}
		history.Commits = append(history.Commits, w.summary(commit))
	}
	return history, nil
}

// commit reads a commit, mapping a missing object or another type to
// ErrUnavailable.
func (r *Repository) commit(hash plumbing.Hash) (*object.Commit, error) {
	commit, err := object.GetCommit(r.storage, hash)
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("read commit %s: %w", hash, err)
	}
	return commit, nil
}

// walker walks commits by committer date. Commits the shallow file lists
// have no parents, as in git.
type walker struct {
	ctx     context.Context
	repo    *Repository
	shallow map[plumbing.Hash]bool
	steps   int
}

func (r *Repository) newWalker(ctx context.Context) (*walker, error) {
	shallow, err := r.storage.Shallow()
	if err != nil {
		return nil, fmt.Errorf("read shallow commits: %w", err)
	}
	w := &walker{ctx: ctx, repo: r, shallow: map[plumbing.Hash]bool{}}
	for _, hash := range shallow {
		w.shallow[hash] = true
	}
	return w, nil
}

func (w *walker) load(hash plumbing.Hash) (*object.Commit, error) {
	commit, err := object.GetCommit(w.repo.storage, hash)
	if err != nil {
		return nil, fmt.Errorf("read commit %s: %w", hash, err)
	}
	return commit, nil
}

func (w *walker) parents(commit *object.Commit) []plumbing.Hash {
	if w.shallow[commit.Hash] {
		return nil
	}
	return commit.ParentHashes
}

// step counts one visited commit and checks the context every cancelCheck.
func (w *walker) step() error {
	w.steps++
	if w.steps%cancelCheck == 0 {
		return w.ctx.Err()
	}
	return nil
}

func (w *walker) summary(commit *object.Commit) Commit {
	parents := []string{}
	for _, parent := range w.parents(commit) {
		parents = append(parents, parent.String())
	}
	subject, body, _ := strings.Cut(commit.Message, "\n")
	for {
		line, rest, found := strings.Cut(body, "\n")
		if !found || strings.TrimSpace(line) != "" {
			break
		}
		body = rest
	}
	if strings.TrimSpace(body) == "" {
		body = ""
	}
	return Commit{
		Hash:       commit.Hash.String(),
		Parents:    parents,
		Subject:    strings.TrimRight(subject, " \t\r"),
		Body:       strings.TrimRight(body, " \t\r\n"),
		Author:     Author{Name: commit.Author.Name, Email: commit.Author.Email},
		AuthorTime: commit.Author.When,
	}
}

// all lists head's history, skipping skip commits and taking limit.
func (w *walker) all(head plumbing.Hash, skip, limit int) ([]Commit, bool, error) {
	if err := w.ctx.Err(); err != nil {
		return nil, false, err
	}
	page := []Commit{}
	queue := &commitQueue{}
	seen := map[plumbing.Hash]bool{head: true}
	first, err := w.load(head)
	if err != nil {
		return nil, false, err
	}
	queue.add(first)
	for taken := 0; queue.Len() > 0; taken++ {
		if err := w.step(); err != nil {
			return nil, false, err
		}
		if limit > 0 && taken >= skip+limit {
			return page, true, nil
		}
		commit := queue.pop()
		if taken >= skip {
			page = append(page, w.summary(commit))
		}
		for _, parent := range w.parents(commit) {
			if seen[parent] {
				continue
			}
			seen[parent] = true
			loaded, err := w.load(parent)
			if err != nil {
				return nil, false, err
			}
			queue.add(loaded)
		}
	}
	return page, false, nil
}

// branch lists the commits reachable from head and not from base, newest
// committer date first. Like git's limit_list it paints commits reachable
// from base uninteresting while walking both sides by date, and stops once
// only uninteresting commits are queued, after walkSlop more.
func (w *walker) branch(head, base plumbing.Hash) ([]plumbing.Hash, error) {
	if err := w.ctx.Err(); err != nil {
		return nil, err
	}
	queue := &commitQueue{}
	// parents holds every queued or visited commit's parents.
	parents := map[plumbing.Hash][]plumbing.Hash{}
	uninteresting := map[plumbing.Hash]bool{}
	push := func(hash plumbing.Hash) error {
		if _, ok := parents[hash]; ok {
			return nil
		}
		commit, err := w.load(hash)
		if err != nil {
			return err
		}
		parents[hash] = w.parents(commit)
		queue.add(commit)
		return nil
	}
	// paint marks hash and its already-seen ancestors uninteresting; unseen
	// ones inherit the mark when they are visited.
	paint := func(hash plumbing.Hash) {
		stack := []plumbing.Hash{hash}
		for len(stack) > 0 {
			next := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if uninteresting[next] {
				continue
			}
			uninteresting[next] = true
			stack = append(stack, parents[next]...)
		}
	}

	uninteresting[base] = true
	if err := push(base); err != nil {
		return nil, err
	}
	if err := push(head); err != nil {
		return nil, err
	}
	var listed []plumbing.Hash
	slop := walkSlop
	for queue.Len() > 0 {
		if err := w.step(); err != nil {
			return nil, err
		}
		if w.steps > walkLimit {
			return nil, ErrRangeTooLarge
		}
		commit := queue.pop()
		if uninteresting[commit.Hash] {
			for _, parent := range parents[commit.Hash] {
				paint(parent)
			}
		} else {
			listed = append(listed, commit.Hash)
			if len(listed) > branchLimit {
				return nil, ErrRangeTooLarge
			}
		}
		for _, parent := range parents[commit.Hash] {
			if err := push(parent); err != nil {
				return nil, err
			}
		}
		if queue.onlyUninteresting(uninteresting) {
			if slop--; slop == 0 {
				break
			}
		} else {
			slop = walkSlop
		}
	}
	// Commits listed before a later path reached them from base drop out.
	result := []plumbing.Hash{}
	for _, hash := range listed {
		if !uninteresting[hash] {
			result = append(result, hash)
		}
	}
	return result, nil
}

// commitQueue pops the newest committer date first, then the earliest
// queued, so equal dates keep a stable order.
type commitQueue struct {
	commits []*object.Commit
	order   []int
	next    int
}

func (q *commitQueue) add(commit *object.Commit) { heap.Push(q, commit) }

func (q *commitQueue) pop() *object.Commit {
	commit, _ := heap.Pop(q).(*object.Commit)
	return commit
}

func (q *commitQueue) Len() int { return len(q.commits) }

func (q *commitQueue) Less(a, b int) bool {
	ta, tb := q.commits[a].Committer.When, q.commits[b].Committer.When
	if !ta.Equal(tb) {
		return ta.After(tb)
	}
	return q.order[a] < q.order[b]
}

func (q *commitQueue) Swap(a, b int) {
	q.commits[a], q.commits[b] = q.commits[b], q.commits[a]
	q.order[a], q.order[b] = q.order[b], q.order[a]
}

func (q *commitQueue) Push(value any) {
	commit, ok := value.(*object.Commit)
	if !ok {
		return
	}
	q.commits = append(q.commits, commit)
	q.order = append(q.order, q.next)
	q.next++
}

func (q *commitQueue) Pop() any {
	last := len(q.commits) - 1
	commit := q.commits[last]
	q.commits, q.order = q.commits[:last], q.order[:last]
	return commit
}

func (q *commitQueue) onlyUninteresting(uninteresting map[plumbing.Hash]bool) bool {
	for _, commit := range q.commits {
		if !uninteresting[commit.Hash] {
			return false
		}
	}
	return true
}

// CommitFiles compares a commit's tree with its first parent's, or with an
// empty tree for a root commit. Statuses are modified, added, deleted, and
// renamed; a deleted and an added file of identical content and mode pair
// as a rename, both in path order. Submodules are skipped. A limit of zero
// or less returns all changes. It returns ErrUnavailable for an unknown
// commit.
func (r *Repository) CommitFiles(ctx context.Context, hash plumbing.Hash, limit int) (CommitChanges, error) {
	changes, err := r.commitChanges(ctx, hash)
	if err != nil {
		return CommitChanges{}, err
	}
	result := CommitChanges{Files: changes}
	if limit > 0 && len(result.Files) > limit {
		result.Files = result.Files[:limit]
		result.Truncated = true
	}
	return result, nil
}

// CommitLookup validates one change of a commit under CommitFiles's rules.
// Without oldPath, a path a rename created reports the rename. A deleted
// path a rename consumed, or anything else that is not a change, is
// ErrUnavailable.
func (r *Repository) CommitLookup(ctx context.Context, hash plumbing.Hash, name, oldPath string) (Change, error) {
	if !validPath(name) || (oldPath != "" && (!validPath(oldPath) || oldPath == name)) {
		return Change{}, ErrUnavailable
	}
	changes, err := r.commitChanges(ctx, hash)
	if err != nil {
		return Change{}, err
	}
	at := sort.Search(len(changes), func(i int) bool { return changes[i].Path >= name })
	if at == len(changes) || changes[at].Path != name || (oldPath != "" && changes[at].OldPath != oldPath) {
		return Change{}, ErrUnavailable
	}
	return changes[at], nil
}

// CommitBlob returns the file name has in a commit. It returns
// ErrUnavailable when the commit is unknown or has no file there.
func (r *Repository) CommitBlob(hash plumbing.Hash, name string) (Blob, error) {
	commit, err := r.commit(hash)
	if err != nil {
		return Blob{}, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return Blob{}, fmt.Errorf("read tree of %s: %w", hash, err)
	}
	return r.treeBlob(tree, name)
}

// ParentBlob returns the file name has in a commit's first parent. It
// returns ErrUnavailable when the commit is unknown, has no parent, or the
// parent has no file there.
func (r *Repository) ParentBlob(hash plumbing.Hash, name string) (Blob, error) {
	commit, err := r.commit(hash)
	if err != nil {
		return Blob{}, err
	}
	tree, err := r.parentTree(commit)
	if err != nil {
		return Blob{}, err
	}
	return r.treeBlob(tree, name)
}

func (r *Repository) treeBlob(tree *object.Tree, name string) (Blob, error) {
	if !validPath(name) {
		return Blob{}, ErrUnavailable
	}
	file, ok, err := treeFile(r.storage, tree, name)
	if err != nil {
		return Blob{}, err
	}
	if !ok {
		return Blob{}, ErrUnavailable
	}
	size, err := r.storage.EncodedObjectSize(file.hash)
	if err != nil {
		return Blob{}, err
	}
	return Blob{Size: size, hash: file.hash, storage: r.storage}, nil
}

// parentTree returns a commit's first parent's tree, or nil for a root or
// shallow commit.
func (r *Repository) parentTree(commit *object.Commit) (*object.Tree, error) {
	if len(commit.ParentHashes) == 0 {
		return nil, nil
	}
	shallow, err := r.storage.Shallow()
	if err != nil {
		return nil, fmt.Errorf("read shallow commits: %w", err)
	}
	for _, hash := range shallow {
		if hash == commit.Hash {
			return nil, nil
		}
	}
	parent, err := object.GetCommit(r.storage, commit.ParentHashes[0])
	if err != nil {
		return nil, fmt.Errorf("read parent of %s: %w", commit.Hash, err)
	}
	return parent.Tree()
}

// commitChanges lists every change of a commit against its first parent,
// sorted by path.
func (r *Repository) commitChanges(ctx context.Context, hash plumbing.Hash) ([]Change, error) {
	commit, err := r.commit(hash)
	if err != nil {
		return nil, err
	}
	parent, err := r.parentTree(commit)
	if err != nil {
		return nil, err
	}
	var parentHash plumbing.Hash
	if parent != nil {
		parentHash = parent.Hash
	}
	diff := &treeDiff{ctx: ctx, repo: r}
	if err := diff.compare(parentHash, commit.TreeHash, ""); err != nil {
		return nil, err
	}

	// Deleted files pair with added files of the same content, both in path
	// order, as Status pairs renames.
	sort.Slice(diff.deleted, func(a, b int) bool { return diff.deleted[a].name < diff.deleted[b].name })
	sort.Slice(diff.added, func(a, b int) bool { return diff.added[a].name < diff.added[b].name })
	byContent := map[recorded][]string{}
	for _, file := range diff.deleted {
		key := recorded{hash: file.hash, mode: normalMode(file.mode)}
		byContent[key] = append(byContent[key], file.name)
	}
	renamedFrom := map[string]bool{}
	changes := make([]Change, 0, len(diff.modified)+len(diff.deleted)+len(diff.added))
	for _, name := range diff.modified {
		changes = append(changes, Change{Path: name, Status: Modified})
	}
	for _, file := range diff.added {
		key := recorded{hash: file.hash, mode: normalMode(file.mode)}
		if olds := byContent[key]; len(olds) > 0 {
			byContent[key] = olds[1:]
			renamedFrom[olds[0]] = true
			changes = append(changes, Change{Path: file.name, OldPath: olds[0], Status: Renamed})
			continue
		}
		changes = append(changes, Change{Path: file.name, Status: Added})
	}
	for _, file := range diff.deleted {
		if !renamedFrom[file.name] {
			changes = append(changes, Change{Path: file.name, Status: Deleted})
		}
	}
	sort.Slice(changes, func(a, b int) bool { return changes[a].Path < changes[b].Path })
	return changes, nil
}

// treeDiff collects the file differences between two trees.
type treeDiff struct {
	ctx      context.Context
	repo     *Repository
	trees    int
	modified []string
	deleted  []namedFile
	added    []namedFile
}

type namedFile struct {
	name string
	recorded
}

// compare diffs the trees oldHash and newHash, either zero for an empty
// tree, below prefix. Identical subtrees are skipped unread.
func (d *treeDiff) compare(oldHash, newHash plumbing.Hash, prefix string) error {
	if oldHash == newHash {
		return nil
	}
	if d.trees++; d.trees%cancelCheck == 1 {
		if err := d.ctx.Err(); err != nil {
			return err
		}
	}
	oldEntries, err := d.entries(oldHash, prefix)
	if err != nil {
		return err
	}
	newEntries, err := d.entries(newHash, prefix)
	if err != nil {
		return err
	}
	for name, old := range oldEntries {
		if err := d.pair(prefix+name, old, newEntries[name]); err != nil {
			return err
		}
	}
	for name, entry := range newEntries {
		if _, ok := oldEntries[name]; !ok {
			if err := d.pair(prefix+name, object.TreeEntry{}, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

// pair diffs one name's old and new entries, either empty when absent.
func (d *treeDiff) pair(name string, old, entry object.TreeEntry) error {
	var oldTree, newTree plumbing.Hash
	if old.Mode == filemode.Dir {
		oldTree = old.Hash
	}
	if entry.Mode == filemode.Dir {
		newTree = entry.Hash
	}
	if !oldTree.IsZero() || !newTree.IsZero() {
		if err := d.compare(oldTree, newTree, name+"/"); err != nil {
			return err
		}
	}
	oldFile, newFile := old.Mode.IsFile(), entry.Mode.IsFile()
	switch {
	case oldFile && newFile:
		if old.Hash != entry.Hash || old.Mode != entry.Mode {
			d.modified = append(d.modified, name)
		}
	case oldFile:
		d.deleted = append(d.deleted, namedFile{name: name, recorded: recorded{hash: old.Hash, mode: old.Mode}})
	case newFile:
		d.added = append(d.added, namedFile{name: name, recorded: recorded{hash: entry.Hash, mode: entry.Mode}})
	}
	return nil
}

// entries maps a tree's directory and file entries by name; submodules are
// left out and a zero hash is an empty tree.
func (d *treeDiff) entries(hash plumbing.Hash, prefix string) (map[string]object.TreeEntry, error) {
	result := map[string]object.TreeEntry{}
	if hash.IsZero() {
		return result, nil
	}
	tree, err := object.GetTree(d.repo.storage, hash)
	if err != nil {
		return nil, fmt.Errorf("read tree %s: %w", strings.TrimSuffix(prefix, "/"), err)
	}
	for _, entry := range tree.Entries {
		if entry.Mode == filemode.Dir || entry.Mode.IsFile() {
			result[entry.Name] = entry
		}
	}
	return result, nil
}

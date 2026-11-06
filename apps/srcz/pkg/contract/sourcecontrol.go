package contract

// ChangeStatus is how a file changed. Commits, branches, and ranges hold
// tracked files only, so only the working tree reports untracked.
type ChangeStatus string

const (
	ChangeModified  ChangeStatus = "modified"
	ChangeAdded     ChangeStatus = "added"
	ChangeUntracked ChangeStatus = "untracked"
	ChangeDeleted   ChangeStatus = "deleted"
	ChangeRenamed   ChangeStatus = "renamed"
)

// ChangedFile is one changed path.
//
//contract:rule status == "renamed" => oldPath != ""
type ChangedFile struct {
	Path string `json:"path"`
	// OldPath is a renamed file's previous path.
	OldPath string       `json:"oldPath,omitempty"`
	Status  ChangeStatus `json:"status"`
}

// ChangesResponse is GET /api/changes: the working tree's changes since
// HEAD (decision 0024).
type ChangesResponse struct {
	// Repository is false for a folder project; files are then empty.
	Repository bool          `json:"repository"`
	Branch     string        `json:"branch"`
	Files      []ChangedFile `json:"files"`
	Truncated  bool          `json:"truncated"`
	Warnings   []string      `json:"warnings"`
}

// DiffSide is one side of a diff under the 0004 limits. Content is absent
// when the side is binary or too large to diff.
type DiffSide struct {
	Content  *string `json:"content,omitempty"`
	Size     int64   `json:"size"`
	Binary   bool    `json:"binary"`
	TooLarge bool    `json:"tooLarge"`
}

// DiffResponse is GET /api/diff: one file's sides and, when both are text,
// their line diff. Original is the older side, null for an added file;
// Modified the newer, null for a deleted one.
//
//contract:rule status == "added" => original == null
//contract:rule status == "untracked" => original == null
//contract:rule status == "modified" => original != null
//contract:rule status == "renamed" => original != null
//contract:rule status == "deleted" => original != null
//contract:rule status == "deleted" => modified == null
//contract:rule status != "deleted" => modified != null
type DiffResponse struct {
	Path         string       `json:"path"`
	OldPath      string       `json:"oldPath,omitempty"`
	Status       ChangeStatus `json:"status"`
	DocumentType FileType     `json:"documentType"`
	Original     *DiffSide    `json:"original"`
	Modified     *DiffSide    `json:"modified"`
	// LineDiff is the sides' line diff (decision 0083); null when a side is
	// binary or too large.
	LineDiff *LineDiff `json:"lineDiff"`
}

// LineDiff is a diff's hunks, the unchanged gaps between them, and its line
// counts (decision 0083). Line numbers are 1-based; a side with no lines in
// a hunk or block starts at the line that follows it. With no hunks the
// sides are equal (ignoring whitespace, apart from whitespace) and there are
// no gaps.
type LineDiff struct {
	// IgnoreWhitespace reports that lines compared without their leading
	// and trailing whitespace.
	IgnoreWhitespace bool `json:"ignoreWhitespace"`
	// Additions and Deletions count the changed lines of every hunk.
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Hunks     []DiffHunk `json:"hunks"`
	// Gaps are the unchanged runs a hunk view collapses, in file order.
	Gaps []DiffGap `json:"gaps"`
}

// DiffHunk is a run of changes with up to four unchanged lines around each.
type DiffHunk struct {
	OldStart  int `json:"oldStart"`
	OldCount  int `json:"oldCount"`
	NewStart  int `json:"newStart"`
	NewCount  int `json:"newCount"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	// Blocks are the hunk's rows in order.
	Blocks []DiffBlock `json:"blocks"`
}

// DiffBlockKind tells unchanged lines from a change.
type DiffBlockKind string

const (
	DiffBlockContext DiffBlockKind = "context"
	DiffBlockChange  DiffBlockKind = "change"
)

// DiffBlock is a run of rows: unchanged lines (oldCount equals newCount),
// or one change of oldCount deleted lines then newCount added ones.
type DiffBlock struct {
	Kind     DiffBlockKind `json:"kind"`
	OldStart int           `json:"oldStart"`
	OldCount int           `json:"oldCount"`
	NewStart int           `json:"newStart"`
	NewCount int           `json:"newCount"`
}

// DiffGap is a run of unchanged lines collapsed before hunk Hunk, or after
// the last hunk when Hunk is the hunk count.
type DiffGap struct {
	Hunk     int `json:"hunk"`
	OldStart int `json:"oldStart"`
	NewStart int `json:"newStart"`
	Lines    int `json:"lines"`
}

// CommitRange is base..HEAD ("branch") or HEAD's whole history ("all").
type CommitRange string

const (
	RangeBranch CommitRange = "branch"
	RangeAll    CommitRange = "all"
)

// Author is a commit's author.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Commit is one commit's summary. Shallow commits have no parents.
type Commit struct {
	Hash    string   `json:"hash"`
	Parents []string `json:"parents"`
	Subject string   `json:"subject"`
	// Body is the message after the subject line, "" when there is none.
	Body   string `json:"body"`
	Author Author `json:"author"`
	// AuthorTime is RFC 3339 with the author's offset.
	AuthorTime string `json:"authorTime"`
}

// CommitsResponse is GET /api/commits: a page of history (decision 0027).
type CommitsResponse struct {
	// Repository is false for a folder project; commits are then empty.
	Repository bool   `json:"repository"`
	Branch     string `json:"branch"`
	// Base is the branch the range compares with, "" when none was found.
	Base  string      `json:"base"`
	Range CommitRange `json:"range"`
	// Head is the commit the walk started from; later pages pass it back.
	Head string `json:"head"`
	// Ahead counts base..head, 0 without a base.
	Ahead   int      `json:"ahead"`
	Commits []Commit `json:"commits"`
	More    bool     `json:"more"`
}

// CommitFilesResponse is GET /api/commit-files: a commit's changes against
// its first parent.
type CommitFilesResponse struct {
	Commit    string        `json:"commit"`
	Files     []ChangedFile `json:"files"`
	Truncated bool          `json:"truncated"`
}

// BranchFilesResponse is GET /api/branch-files: the net changes from the
// merge base with base to head, or with uncommitted=1 to the working tree,
// whose files may be untracked (decisions 0030, 0051).
type BranchFilesResponse struct {
	Repository bool   `json:"repository"`
	Branch     string `json:"branch"`
	// Base is "" when no base branch was found.
	Base string `json:"base"`
	// MergeBase is "" while nothing is ahead or there is no base.
	MergeBase string `json:"mergeBase"`
	// Head is "" when HEAD is unborn.
	Head      string        `json:"head"`
	Ahead     int           `json:"ahead"`
	Files     []ChangedFile `json:"files"`
	Truncated bool          `json:"truncated"`
	// Warnings are folders the untracked walk could not read; only a compare
	// with uncommitted changes walks.
	Warnings []string `json:"warnings"`
}

// RangeFilesResponse is GET /api/range-files: the net changes from from's
// first parent to to.
type RangeFilesResponse struct {
	From      string        `json:"from"`
	To        string        `json:"to"`
	Files     []ChangedFile `json:"files"`
	Truncated bool          `json:"truncated"`
}

// BranchRef is a branch a compare may use as its base. Name is git's short
// form: "main" for a local branch, "origin/main" for a remote-tracking one.
type BranchRef struct {
	Name   string `json:"name"`
	Remote bool   `json:"remote"`
}

// RefsResponse is GET /api/refs.
type RefsResponse struct {
	// Repository is false for a folder project; branches are then empty.
	Repository bool `json:"repository"`
	// Branch is the checked-out branch, "" when HEAD is detached.
	Branch string `json:"branch"`
	// DefaultBase is the base chosen when none is given, "" when none exists.
	DefaultBase string `json:"defaultBase"`
	// Branches lists local branches, then remote-tracking ones, each by name.
	Branches  []BranchRef `json:"branches"`
	Truncated bool        `json:"truncated"`
}

// StackLayer is one local branch of a stack (decision 0040).
//
//contract:rule name == "" => current == true
type StackLayer struct {
	// Name is the layer's local branch, "" for a detached HEAD.
	Name string `json:"name"`
	// Aliases are other local branches on the same commit.
	Aliases []string `json:"aliases"`
	Head    string   `json:"head"`
	// Parent is the layer below, or the base branch for the lowest layer.
	Parent string `json:"parent"`
	// Ahead counts the layer's own commits, those its parent does not have.
	Ahead int `json:"ahead"`
	// Current marks HEAD's layer.
	Current bool `json:"current"`
}

// StackResponse is GET /api/branch-stack: the local branches stacked between
// the base branch and HEAD, and those built on top of HEAD.
type StackResponse struct {
	// Repository is false for a folder project; layers are then empty.
	Repository bool `json:"repository"`
	// Branch is the checked-out branch, "" when HEAD is detached.
	Branch string `json:"branch"`
	// Base is "" when no base branch was found.
	Base string `json:"base"`
	// MergeBase is "" while nothing is ahead or there is no base.
	MergeBase string `json:"mergeBase"`
	// Head is "" when HEAD is unborn.
	Head string `json:"head"`
	// Layers run from the top of the stack down to the layer nearest the
	// base; empty while HEAD has nothing ahead of the base.
	Layers    []StackLayer `json:"layers"`
	Truncated bool         `json:"truncated"`
}

// StatsStatus says whether a file's lines were counted, and why not.
type StatsStatus string

const (
	StatsCounted  StatsStatus = "counted"
	StatsBinary   StatsStatus = "binary"
	StatsTooLarge StatsStatus = "tooLarge"
	StatsBudget   StatsStatus = "budget"
)

// FileStats is one compared file's line counts, present only when counted,
// as git diff --numstat counts them. Blob is the git blob hash of the newer
// side, or the older side for a deleted file; empty when that side is in the
// worktree and was not read.
type FileStats struct {
	Path      string      `json:"path"`
	Status    StatsStatus `json:"status"`
	Additions *int        `json:"additions,omitempty"`
	Deletions *int        `json:"deletions,omitempty"`
	Blob      string      `json:"blob"`
}

// CompareStatsResponse is GET /api/compare-stats: per-file line counts for
// one compare, in the order of the compare's file list (decision 0035).
type CompareStatsResponse struct {
	Files []FileStats `json:"files"`
	// BudgetExceeded reports that the read budget left files uncounted.
	BudgetExceeded bool `json:"budgetExceeded"`
}

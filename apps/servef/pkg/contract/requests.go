package contract

// PageRequest selects a project's tree and, optionally, one document. Path
// names the document; Required reports that one must be selected (HTTP: the
// path parameter was sent, even empty). At, a full commit hash, reads the
// document from that commit instead of the working tree.
type PageRequest struct {
	Project  string
	Path     string
	At       string
	Required bool
}

// FormatRequest asks for a formattable document's formatted text, from the
// working tree or, with At, a commit.
type FormatRequest struct {
	Project string
	Path    string
	At      string
}

// EventsRequest follows a project's live updates (decision 0050) at a lease
// level: "open" (the default when empty) or "watched" (decision 0066).
type EventsRequest struct {
	Project string
	Lease   string
}

// DiffKind names what a diff compares.
type DiffKind string

const (
	// DiffWorking compares the working tree with HEAD.
	DiffWorking DiffKind = ""
	// DiffCommit compares a commit with its first parent.
	DiffCommit DiffKind = "commit"
	// DiffBranch compares a branch with its merge base, or with Uncommitted
	// HEAD's branch's base with the working tree.
	DiffBranch DiffKind = "branch"
	// DiffRange compares a commit range's oldest commit's first parent with
	// its newest commit.
	DiffRange DiffKind = "range"
)

// DiffRequest asks for one changed file's diff (decisions 0024, 0027, 0030,
// 0051, 0083). Path names the file and OldPath a rename's old path. Commit,
// a full hash, applies to commit diffs; Base, Head, and Uncommitted to
// branch diffs, where empty Base and Head mean the default base branch and
// HEAD, and Uncommitted adds the working tree to HEAD's branch; From and
// To, full hashes, to range diffs. IgnoreWhitespace compares lines without
// their leading and trailing whitespace.
type DiffRequest struct {
	Project          string
	Kind             DiffKind
	Path             string
	OldPath          string
	Commit           string
	Base             string
	Head             string
	Uncommitted      bool
	From             string
	To               string
	IgnoreWhitespace bool
}

// CommitsRequest pages history (decision 0027). Range defaults to branch;
// Skip below 0 is invalid; Limit 0 means 50, and otherwise must be from 1 to
// 100; Head, a full hash, pins the walk (empty walks from HEAD); Base names
// the base branch (empty finds the default).
type CommitsRequest struct {
	Project string
	Range   CommitRange
	Skip    int
	Limit   int
	Head    string
	Base    string
}

// BranchFilesRequest lists a branch's net changes: Base or else the default
// base branch against Head or else HEAD; with Uncommitted, HEAD's against
// the working tree (decision 0051), which allows no Head.
type BranchFilesRequest struct {
	Project     string
	Base        string
	Head        string
	Uncommitted bool
}

// CompareKind names a compare's file list (decision 0035).
type CompareKind string

const (
	CompareWorking CompareKind = "working"
	CompareBranch  CompareKind = "branch"
	CompareCommit  CompareKind = "commit"
	CompareRange   CompareKind = "range"
)

// CompareStatsRequest counts the changed lines of a compare's files: Base,
// Head, and Uncommitted apply to a branch compare, Commit to a commit
// compare, and From and To to a range compare, as their file lists take
// them.
type CompareStatsRequest struct {
	Project     string
	Compare     CompareKind
	Base        string
	Head        string
	Uncommitted bool
	Commit      string
	From        string
	To          string
}

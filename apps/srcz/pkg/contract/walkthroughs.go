package contract

// WalkthroughSummary lists a walkthrough without its steps, GET
// /api/walkthroughs (an array, most recently updated first; decision 0047).
type WalkthroughSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	StepCount int    `json:"stepCount"`
	// UpdatedAt is an RFC 3339 time.
	UpdatedAt string `json:"updatedAt"`
}

// WalkthroughCompareKind names what a diff step compares, as Source Control
// does.
type WalkthroughCompareKind string

const (
	WalkthroughCompareWorking WalkthroughCompareKind = "working"
	WalkthroughCompareBranch  WalkthroughCompareKind = "branch"
	WalkthroughCompareCommit  WalkthroughCompareKind = "commit"
	WalkthroughCompareRange   WalkthroughCompareKind = "range"
)

// WalkthroughSide names which side's line numbers a diff step's lines use.
type WalkthroughSide string

const (
	WalkthroughSideNew WalkthroughSide = "new"
	WalkthroughSideOld WalkthroughSide = "old"
)

// WalkthroughCompare names a Source Control compare: working (uncommitted
// changes), branch (Base, default the detected base, to Head, default HEAD),
// commit (Commit against its first parent), or range (From's parent to To).
type WalkthroughCompare struct {
	Kind   WalkthroughCompareKind `json:"kind"`
	Base   string                 `json:"base,omitempty"`
	Head   string                 `json:"head,omitempty"`
	Commit string                 `json:"commit,omitempty"`
	From   string                 `json:"from,omitempty"`
	To     string                 `json:"to,omitempty"`
}

// WalkthroughFileTarget is a file in the working tree, or at commit At,
// optionally at Lines.
type WalkthroughFileTarget struct {
	Path string `json:"path"`
	// Lines is an inclusive, 1-based [start, end].
	Lines []int `json:"lines,omitempty"`
	// At is a full commit hash: the file as of that commit.
	At string `json:"at,omitempty"`
	// Expect is the trimmed text the first line started with when written.
	Expect string `json:"expect,omitempty"`
}

// WalkthroughDiffTarget is a file's diff in a compare, optionally at Lines
// of Side. No path opens the compare's review at its first file.
type WalkthroughDiffTarget struct {
	Compare WalkthroughCompare `json:"compare"`
	Path    string             `json:"path,omitempty"`
	Side    WalkthroughSide    `json:"side,omitempty"`
	// Lines is an inclusive, 1-based [start, end].
	Lines  []int  `json:"lines,omitempty"`
	Expect string `json:"expect,omitempty"`
}

// WalkthroughStep is one stop; one with neither file nor diff is a note.
type WalkthroughStep struct {
	Title string                 `json:"title"`
	Body  string                 `json:"body,omitempty"`
	File  *WalkthroughFileTarget `json:"file,omitempty"`
	Diff  *WalkthroughDiffTarget `json:"diff,omitempty"`
	// BodyHTML is the body rendered as a Markdown preview, "" without one.
	BodyHTML string `json:"bodyHTML"`
}

// Walkthrough is GET /api/walkthrough: a stored walkthrough with its
// Markdown rendered.
type Walkthrough struct {
	ID          string            `json:"id"`
	Project     string            `json:"project"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary,omitempty"`
	SummaryHTML string            `json:"summaryHTML"`
	Steps       []WalkthroughStep `json:"steps"`
	// CreatedAt is an RFC 3339 time.
	CreatedAt string `json:"createdAt"`
	// UpdatedAt is an RFC 3339 time.
	UpdatedAt string `json:"updatedAt"`
}

// WalkthroughPublished is POST /api/walkthroughs: the stored ID, its view
// URL, and whether the ID is new rather than replaced.
type WalkthroughPublished struct {
	ID string `json:"id"`
	// URL is the walkthrough's view: absolute at the server's host over
	// HTTP, the path and query (/view?...) in process.
	URL     string `json:"url"`
	Created bool   `json:"created"`
}

package contract

// PreviewKind is the preview a file type renders; empty for none.
type PreviewKind string

const (
	PreviewNone     PreviewKind = ""
	PreviewMarkdown PreviewKind = "markdown"
	PreviewImage    PreviewKind = "image"
)

// DefaultDocumentMode is the view a file type opens in.
type DefaultDocumentMode string

const (
	ModePreview DefaultDocumentMode = "preview"
	ModeSource  DefaultDocumentMode = "source"
)

// FileType is a file family's presentation metadata.
//
//contract:rule previewKind == "" => defaultMode == "source"
//contract:rule previewKind != "" => previewLabel != ""
type FileType struct {
	ID           string              `json:"id"`
	Label        string              `json:"label"`
	Extensions   []string            `json:"extensions"`
	Filenames    []string            `json:"filenames"`
	Language     string              `json:"language"`
	PreviewKind  PreviewKind         `json:"previewKind"`
	PreviewLabel string              `json:"previewLabel"`
	DefaultMode  DefaultDocumentMode `json:"defaultMode"`
	Formattable  bool                `json:"formattable"`
}

// TreeNode is one entry of a project's file tree.
type TreeNode struct {
	FileType string     `json:"fileType"`
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Open     bool       `json:"open"`
	Selected bool       `json:"selected"`
	Children []TreeNode `json:"children"`
}

// PageData is GET /api/page and the shell's embedded page: a project's tree
// and, when one is selected, a document. A failed page still carries its
// project and the failure in Error.
//
//contract:rule hasFile == true => documentType != null
type PageData struct {
	// Roots are the served roots.
	Roots []string `json:"roots"`
	// SessionKey keys browser workspace memory: "daemon", or the roots joined.
	SessionKey string `json:"sessionKey"`
	Project    string `json:"project"`
	// ProjectUnavailable reports that the project itself could not be
	// resolved, as opposed to a file within it.
	ProjectUnavailable bool   `json:"projectUnavailable"`
	RootName           string `json:"rootName"`
	// Repository names the project's repository (origin remote, else main
	// worktree folder), empty for a folder.
	Repository  string      `json:"repository"`
	ProjectKind ProjectKind `json:"projectKind"`
	// Branch is the project's checked-out branch, empty for a folder.
	Branch string `json:"branch"`
	// Worktrees counts the repository's checkouts; above 1 it has worktrees.
	Worktrees int        `json:"worktrees"`
	Tree      []TreeNode `json:"tree"`
	Selected  string     `json:"selected"`
	// At is the full commit hash the selected document was read from, or
	// empty for the working tree. Snapshots have Source and Format only.
	At        string   `json:"at"`
	Content   string   `json:"content"`
	Source    string   `json:"source"`
	FileSize  int64    `json:"fileSize"`
	FileCount int      `json:"fileCount"`
	HasFile   bool     `json:"hasFile"`
	Empty     bool     `json:"empty"`
	Error     string   `json:"error"`
	Warnings  []string `json:"warnings"`
	// DocumentType is the selected document's type; null without one.
	DocumentType        *FileType `json:"documentType"`
	NavigationSupported bool      `json:"navigationSupported"`
	Binary              bool      `json:"binary"`
	TooLarge            bool      `json:"tooLarge"`
	PreviewDisabled     bool      `json:"previewDisabled"`
	// HighlightDisabled: no syntax highlighting (binary, too large, or over
	// the 4 MiB highlight limit); Source shows plain text.
	HighlightDisabled bool   `json:"highlightDisabled"`
	SearchExcluded    bool   `json:"searchExcluded"`
	PreviewError      string `json:"previewError"`
}

// FormatResult is GET /api/format: the formatter's text, which the UI
// highlights as Source.
type FormatResult struct {
	Formatted string `json:"formatted"`
}

// SearchDocument is one searchable file's original text.
type SearchDocument struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

// SearchDocumentsResponse is GET /api/search-documents: every non-ignored,
// non-binary file within the search limit.
type SearchDocumentsResponse struct {
	Documents []SearchDocument `json:"documents"`
	Warnings  []string         `json:"warnings"`
}

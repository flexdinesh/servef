package contract

// ProjectKind tells a linked worktree from a repository or folder.
type ProjectKind string

const (
	ProjectFolder   ProjectKind = "folder"
	ProjectRepo     ProjectKind = "repo"
	ProjectWorktree ProjectKind = "worktree"
)

// Root is a served root directory.
type Root struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	IsRepo bool   `json:"isRepo"`
}

// Project is a project within the served roots. ID is the absolute path, or
// empty for a project that cannot be opened. Members of one repository share
// Group, the path of its main worktree; Root is the served root it, or its
// repository, lives in. Repository names its repository: the origin remote's
// name, else the main worktree's folder; empty for a folder.
//
//contract:rule available == true => id != ""
//contract:rule available == false => id == ""
type Project struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	Kind       ProjectKind `json:"kind"`
	Group      string      `json:"group"`
	Root       string      `json:"root"`
	Branch     string      `json:"branch"`
	Repository string      `json:"repository"`
	Available  bool        `json:"available"`
}

// ProjectsResponse is GET /api/projects.
type ProjectsResponse struct {
	// SessionKey keys the last project remembered for this session: "daemon"
	// under the daemon, else the served roots joined by newlines.
	SessionKey string `json:"sessionKey"`
	// Presence asks a page to hold /api/presence while open: this server's
	// lifetime follows its UIs (decision 0098).
	Presence bool      `json:"presence"`
	Roots    []Root    `json:"roots"`
	Projects []Project `json:"projects"`
	Warnings []string  `json:"warnings"`
}

// Directory is a child directory of a folder within the served roots.
type Directory struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsProject bool   `json:"isProject"`
}

// DirectoriesResponse is GET /api/directories.
type DirectoriesResponse struct {
	Path        string      `json:"path"`
	Directories []Directory `json:"directories"`
}

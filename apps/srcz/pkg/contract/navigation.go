package contract

// NavigationLocation is a definition or reference: a 1-based line and a
// 0-based UTF-16 column, as the browser counts (decision 0018).
type NavigationLocation struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine"`
	EndColumn int    `json:"endColumn"`
	// Kind is "definition" or "reference".
	Kind    string `json:"kind"`
	Snippet string `json:"snippet"`
}

// NavigationResult is GET /api/definition and /api/references: the
// language server's eligible locations. Filtered counts locations dropped
// as outside, ignored, or unreadable; Truncated reports more than the cap.
type NavigationResult struct {
	Symbol    string               `json:"symbol"`
	Locations []NavigationLocation `json:"locations"`
	Server    string               `json:"server"`
	Filtered  int                  `json:"filtered"`
	Truncated bool                 `json:"truncated"`
}

// LanguageServerState is a language server's lifecycle state.
type LanguageServerState string

const (
	LanguageServerMissing  LanguageServerState = "missing"
	LanguageServerStopped  LanguageServerState = "stopped"
	LanguageServerStarting LanguageServerState = "starting"
	LanguageServerReady    LanguageServerState = "ready"
	LanguageServerFailed   LanguageServerState = "failed"
)

// LanguageServerInstall is one host-specific way to install a server.
type LanguageServerInstall struct {
	Label         string   `json:"label"`
	Command       string   `json:"command"`
	Prerequisites []string `json:"prerequisites,omitempty"`
	Docs          string   `json:"docs,omitempty"`
}

// LanguageServerCapabilities are a running server's negotiated methods.
type LanguageServerCapabilities struct {
	Definition bool `json:"definition"`
	References bool `json:"references"`
}

// LanguageServerStatus describes a file family's language server. Memory is
// resident bytes while running, 0 otherwise; Capabilities is null until the
// server has started.
type LanguageServerStatus struct {
	Name         string                      `json:"name"`
	Language     string                      `json:"language"`
	State        LanguageServerState         `json:"state"`
	Message      string                      `json:"message"`
	Install      []LanguageServerInstall     `json:"install"`
	Docs         string                      `json:"docs"`
	Memory       uint64                      `json:"memory"`
	IdleMinutes  int                         `json:"idleMinutes"`
	Capabilities *LanguageServerCapabilities `json:"capabilities"`
}

// NavigationStatus is GET /api/navigation-status: whether a file can
// navigate and its language server's state. It never starts a server.
type NavigationStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Eligible  bool   `json:"eligible"`
	Reason    string `json:"reason"`
	// Server is null when no language server serves the file's family.
	Server *LanguageServerStatus `json:"server"`
}

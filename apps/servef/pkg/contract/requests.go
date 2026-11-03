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

// Package live decides what a project's live-update events (decisions 0050,
// 0066) reread, as the web UI's live-updates.ts does. It is pure.
package live

import (
	"slices"

	"github.com/flexdinesh/servef/pkg/contract"
)

// Targets are what one change rereads.
type Targets struct {
	// Page rereads the tree and the open document.
	Page bool
	// Changes rereads the uncommitted change list.
	Changes bool
}

// For decides what change rereads; document is the open working-tree
// document's path, "" for none. Overflow rereads everything.
func For(change contract.ChangeEvent, document string) Targets {
	all := change.Overflow
	return Targets{
		Page:    all || change.Tree || (document != "" && slices.Contains(change.Paths, document)),
		Changes: all || change.Git || len(change.Paths) > 0,
	}
}

// Diff decides whether change rereads an open working-tree diff of path,
// or of a rename from oldPath: a change to either, a git change (a commit
// or checkout moves HEAD), or overflow.
func Diff(change contract.ChangeEvent, path, oldPath string) bool {
	return change.Overflow || change.Git || slices.Contains(change.Paths, path) || (oldPath != "" && slices.Contains(change.Paths, oldPath))
}

// Everything is what reconnecting rereads: changes the stream may have
// missed.
var Everything = contract.ChangeEvent{Overflow: true}

// Indicator is a status's status-bar label and description.
type Indicator struct {
	Label       string
	Description string
}

// Describe names a status for the status bar.
func Describe(status contract.StatusEvent) Indicator {
	reason := ""
	if status.Reason != "" {
		reason = " " + status.Reason
	}
	switch status.Mode {
	case "watching":
		return Indicator{"Live", "Live updates: watching the project for changes."}
	case "polling":
		return Indicator{"Polling", "Live updates: checking the project for changes every few seconds." + reason}
	}
	return Indicator{"Off", "Live updates are off; the project rereads when you reopen it." + reason}
}

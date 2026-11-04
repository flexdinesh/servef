package shell

// EmptyAction is what the empty pane offers, as the web's
// EmptyWorkspaceAction without the phone drawer's show-view.
type EmptyAction string

const (
	FindFile          EmptyAction = "find-file"
	ReviewUncommitted EmptyAction = "review-uncommitted"
	ShowSidebar       EmptyAction = "show-sidebar"
)

// EmptyInput is what the empty pane follows.
type EmptyInput struct {
	// View is the sidebar's shown view, "" when it is hidden.
	View          View
	SearchPattern string
	// SearchFiles counts files with results for the pattern.
	SearchFiles int
	// Changes is the uncommitted file count, -1 until read; Repository is
	// false for a folder project.
	Changes    int
	Repository bool
	// Walkthroughs is the listed count, -1 until read.
	Walkthroughs int
}

// Empty is the empty pane's text and its action, "" for none.
type Empty struct {
	Heading string
	Hint    string
	Action  EmptyAction
}

// EmptyWorkspace names what the visible sidebar view offers with no tab
// open (decision 0052), as the web's emptyWorkspace.
func EmptyWorkspace(in EmptyInput) Empty {
	switch in.View {
	case "":
		return Empty{"Nothing open", "Open a file, a review, or a walkthrough.", ShowSidebar}
	case Search:
		if trimmed(in.SearchPattern) && in.SearchFiles > 0 {
			return Empty{"Open a match", "Choose a result to open it at that line.", ""}
		}
		return Empty{"Search file contents", "Type in the Search field, then open a match.", ""}
	case SourceControl:
		switch {
		case !in.Repository:
			return Empty{"Not a git repository", "Source Control needs a git repository.", ""}
		case in.Changes > 0:
			noun := "uncommitted changes"
			if in.Changes == 1 {
				noun = "uncommitted change"
			}
			return Empty{"Review your changes", Count(in.Changes) + " " + noun + ". Open a file, a commit, or a branch.", ReviewUncommitted}
		}
		return Empty{"Review history", "No uncommitted changes. Open a commit or a branch.", ""}
	case Walkthroughs:
		if in.Walkthroughs == 0 {
			return Empty{"No walkthroughs yet", "Agents publish guided tours of this project here.", ""}
		}
		return Empty{"Choose a walkthrough", "Open one from the list to start.", ""}
	}
	return Empty{"Choose a file", "Select a file from the folder tree.", FindFile}
}

func trimmed(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// ActionLabel labels an empty pane action and its shortcut, as
// EmptyWorkspace.tsx.
func ActionLabel(action EmptyAction, mac bool) (label, shortcut string) {
	switch action {
	case FindFile:
		label, shortcut = "Find a file", "Ctrl+K"
		if mac {
			shortcut = "⌘K"
		}
	case ReviewUncommitted:
		label = "Review uncommitted changes"
	case ShowSidebar:
		label, shortcut = "Show sidebar", "Ctrl+B"
		if mac {
			shortcut = "⌘B"
		}
	}
	return label, shortcut
}

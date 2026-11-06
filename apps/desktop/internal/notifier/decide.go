package notifier

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/flexdinesh/srcz/pkg/contract"
)

// Kind is what a notice reports; each has its setting.
type Kind string

const (
	KindCommit      Kind = "commit"
	KindBranch      Kind = "branch"
	KindEdits       Kind = "edits"
	KindWalkthrough Kind = "walkthrough"
)

// Settings is which kinds the user wants from one connection.
type Settings struct {
	Commits      bool
	Branches     bool
	Edits        bool
	Walkthroughs bool
}

func (s Settings) enabled(kind Kind) bool {
	switch kind {
	case KindCommit:
		return s.Commits
	case KindBranch:
		return s.Branches
	case KindEdits:
		return s.Edits
	case KindWalkthrough:
		return s.Walkthroughs
	}
	return false
}

// followsOtherProjects reports a kind that fires for projects not on screen.
func (s Settings) followsOtherProjects() bool {
	return s.Commits || s.Branches || s.Walkthroughs
}

// message is a notice before it is addressed to a project and connection.
type message struct {
	kind   Kind
	title  string
	detail string
	// route is the web app's route for a click, without its project.
	route string
	query url.Values
}

// headMessage says what moved between last and next: nothing when last is nil
// (the baseline) or HEAD did not move, or next is unborn. It compares with the
// last head this notifier saw, not the stream's, so a reconnect catches up.
func headMessage(last *contract.HeadRef, next contract.HeadEvent) (message, bool) {
	if last == nil || (last.Branch == next.Branch && last.Commit == next.Commit) {
		return message{}, false
	}
	short, summary := shortCommit(next.Commit), commitSummary(next)
	if last.Branch != next.Branch {
		if next.Branch == "" {
			return message{kind: KindBranch, title: "Detached at " + short, route: "/"}, true
		}
		return message{kind: KindBranch, title: "Switched to " + clean(next.Branch, maxName), route: "/"}, true
	}
	if next.Commit == "" {
		return message{}, false
	}
	route := url.Values{"commit": {next.Commit}}
	if next.Parent != "" && next.Parent == last.Commit {
		title := "New commit"
		if next.Branch != "" {
			title += " on " + clean(next.Branch, maxName)
		}
		return message{kind: KindCommit, title: title, detail: summary, route: "/view", query: route}, true
	}
	title := "HEAD moved"
	if next.Branch != "" {
		title = clean(next.Branch, maxName) + " moved"
	}
	return message{kind: KindCommit, title: title, detail: summary, route: "/view", query: route}, true
}

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return clean(commit[:7], maxName)
	}
	return clean(commit, maxName)
}

func commitSummary(head contract.HeadEvent) string {
	return strings.TrimSpace(shortCommit(head.Commit) + " " + clean(head.Subject, maxText))
}

// changeSet is a working tree's changes: each path's status.
type changeSet map[string]contract.ChangeStatus

func newChangeSet(files []contract.ChangedFile) changeSet {
	set := make(changeSet, len(files))
	for _, file := range files {
		set[file.Path] = file.Status
	}
	return set
}

// differing lists, sorted, the paths whose state differs between the sets.
func differing(previous, current changeSet) []string {
	var paths []string
	for name, status := range current {
		if before, ok := previous[name]; !ok || before != status {
			paths = append(paths, name)
		}
	}
	for name := range previous {
		if _, ok := current[name]; !ok {
			paths = append(paths, name)
		}
	}
	slices.Sort(paths)
	return paths
}

// repositoryEditsMessage reports changes that settled in a repository: nothing
// when the working tree's set of changes is what it was, as after a build that
// wrote only ignored files, or when it is empty, as after a commit.
func repositoryEditsMessage(previous, current changeSet) (message, bool) {
	if len(current) == 0 {
		return message{}, false
	}
	paths := differing(previous, current)
	if len(paths) == 0 {
		return message{}, false
	}
	return editsMessage(paths, false, url.Values{"review": {"working"}}, "/view"), true
}

// folderEditsMessage reports changes that settled in a folder without git,
// which has no set to compare: every path counts.
func folderEditsMessage(paths []string, overflow bool) (message, bool) {
	if len(paths) == 0 && !overflow {
		return message{}, false
	}
	return editsMessage(paths, overflow, nil, "/"), true
}

func editsMessage(paths []string, overflow bool, query url.Values, route string) message {
	title := fmt.Sprintf("%d files changed", len(paths))
	switch {
	case overflow:
		title = "Many files changed"
	case len(paths) == 1:
		title = "1 file changed"
	}
	return message{kind: KindEdits, title: title, detail: pathList(paths), route: route, query: query}
}

// pathList names up to two files by base name: "a.md, b.go, +1".
func pathList(paths []string) string {
	const shown = 2
	names := make([]string, 0, shown+1)
	for _, name := range paths[:min(shown, len(paths))] {
		names = append(names, clean(path.Base(name), maxName))
	}
	if len(paths) > shown {
		names = append(names, fmt.Sprintf("+%d", len(paths)-shown))
	}
	return strings.Join(names, ", ")
}

// walkthroughMessage reports the newest walkthrough changed since previous, a
// map of id to update time; nothing when previous is nil (the baseline). One
// notice covers what a reconnect finds.
func walkthroughMessage(previous map[string]string, current []contract.WalkthroughSummary) (message, bool) {
	if previous == nil {
		return message{}, false
	}
	var newest *contract.WalkthroughSummary
	for i := range current {
		if previous[current[i].ID] == current[i].UpdatedAt {
			continue
		}
		if newest == nil || current[i].UpdatedAt > newest.UpdatedAt {
			newest = &current[i]
		}
	}
	if newest == nil {
		return message{}, false
	}
	title := "New walkthrough"
	if _, existed := previous[newest.ID]; existed {
		title = "Walkthrough updated"
	}
	return message{kind: KindWalkthrough, title: title, detail: clean(newest.Title, maxText), route: "/view", query: url.Values{"wt": {newest.ID}}}, true
}

func walkthroughTimes(summaries []contract.WalkthroughSummary) map[string]string {
	times := make(map[string]string, len(summaries))
	for _, summary := range summaries {
		times[summary.ID] = summary.UpdatedAt
	}
	return times
}

// addressed makes the notice for a project on a connection. The body always
// names the project and, for a remote server, its connection's label, so a
// server cannot pass its notices off as this computer's.
func addressed(m message, connection, label, project string) Notice {
	parts := make([]string, 0, 3)
	if label != "" {
		parts = append(parts, clean(label, maxName))
	}
	parts = append(parts, clean(path.Base(project), maxName))
	if m.detail != "" {
		parts = append(parts, m.detail)
	}
	query := url.Values{"project": {project}}
	for key, values := range m.query {
		query[key] = values
	}
	return Notice{
		ID:      noticeID(connection, project, m.kind),
		Kind:    m.kind,
		Project: project,
		Title:   m.title,
		Body:    strings.Join(parts, " · "),
		Route:   m.route + "?" + query.Encode(),
	}
}

// limiter keeps a project's kind to one notice per perKind and a connection to
// perMinute notices a minute, whatever a server sends.
type limiter struct {
	last   map[string]time.Time
	recent []time.Time
}

const (
	perKind   = 30 * time.Second
	perMinute = 10
)

func (l *limiter) allow(now time.Time, project string, kind Kind) bool {
	key := project + "\x00" + string(kind)
	if last, ok := l.last[key]; ok && now.Sub(last) < perKind {
		return false
	}
	l.recent = slices.DeleteFunc(l.recent, func(at time.Time) bool { return now.Sub(at) >= time.Minute })
	if len(l.recent) >= perMinute {
		return false
	}
	if l.last == nil {
		l.last = map[string]time.Time{}
	}
	l.last[key] = now
	l.recent = append(l.recent, now)
	return true
}

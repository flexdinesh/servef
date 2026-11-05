//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// shownNotice is a line of what the app showed (notices_e2e.go in the app).
type shownNotice struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (w *world) shownNotices() []shownNotice {
	w.t.Helper()
	file, err := os.Open(w.notices)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		w.t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var notices []shownNotice
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		var notice shownNotice
		if err := json.Unmarshal(lines.Bytes(), &notice); err != nil {
			w.t.Fatalf("notice %q: %v", lines.Text(), err)
		}
		notices = append(notices, notice)
	}
	return notices
}

// waitNotice waits for a notice match accepts. retry runs now and again every
// period, for what happens only after the notifier has its baseline.
func (w *world) waitNotice(what string, match func(shownNotice) bool, period time.Duration, retry func()) shownNotice {
	w.t.Helper()
	deadline := time.Now().Add(2 * pageTimeout)
	var next time.Time
	for {
		for _, notice := range w.shownNotices() {
			if match(notice) {
				return notice
			}
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("no notice of %s; shown: %+v", what, w.shownNotices())
		}
		if retry != nil && time.Now().After(next) {
			retry()
			next = time.Now().Add(period)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func gitIn(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=e2e", "-c", "user.email=e2e@example.invalid"}, args...)...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

// exerciseNotices drives each kind of notice for the project in app's current
// window: what a window on this computer and one on a remote server must both
// do (decision 0099). Every notice's body must match addressed; connection is
// its window's scheme host; publish publishes a walkthrough titled "Notice tour"
// the way that server's clients do.
func (w *world) exerciseNotices(app *session, project, connection string, addressed *regexp.Regexp, publish func()) {
	t := w.t
	t.Helper()
	ours := func(notice shownNotice) bool {
		return strings.HasPrefix(notice.ID, connection+"/") && addressed.MatchString(notice.Body)
	}
	titled := func(prefix string) func(shownNotice) bool {
		return func(notice shownNotice) bool { return ours(notice) && strings.HasPrefix(notice.Title, prefix) }
	}
	setFocused := func(focused bool) {
		t.Helper()
		app.eval(fmt.Sprintf(`await window.mygo.call("E2E.SetFocused", %v); return true`, focused))
	}
	click := func(notice shownNotice, query string) {
		t.Helper()
		app.eval(fmt.Sprintf(`await window.mygo.call("E2E.ClickNotice", %q); return true`, notice.ID))
		app.waitEval("the click's page, "+query, fmt.Sprintf(`return location.pathname === "/view" && location.search.includes(%q)`, query))
	}

	// Its settings are here, as they are not in a browser.
	app.eval(`document.dispatchEvent(new KeyboardEvent("keydown", {key: ",", ctrlKey: true, bubbles: true})); return true`)
	app.waitEval("the notification settings", `return document.querySelector('dialog[open] [data-setting="notifyEdits"]') !== null`)
	app.eval(`document.querySelector("dialog[open]").close(); return true`)

	// A branch switch shows the notifier is following: it reports only moves
	// after its baseline, so the first checkouts may pass silently.
	branch := 0
	w.waitNotice("a branch switch", titled("Switched to feat-"), 5*time.Second, func() {
		branch++
		gitIn(t, project, "checkout", "-q", "-b", "feat-"+strconv.Itoa(branch))
	})

	// A commit is silent while the user looks at its project. Had that one been
	// shown, it would also spend the 30 s limit the next one needs.
	setFocused(true)
	gitIn(t, project, "commit", "-q", "--allow-empty", "-m", "seen while focused")
	time.Sleep(4 * time.Second)
	setFocused(false)
	gitIn(t, project, "commit", "-q", "--allow-empty", "-m", "<b>unseen</b>")
	commit := w.waitNotice("a commit", titled("New commit on feat-"), 0, nil)
	// A server's text reaches the notification service as text, not markup.
	if !strings.Contains(commit.Body, "&lt;b&gt;unseen&lt;/b&gt;") {
		t.Errorf("commit notice = %+v", commit)
	}
	for _, notice := range w.shownNotices() {
		if strings.Contains(notice.Body, "seen while focused") {
			t.Errorf("a notice for what the user was looking at: %+v", notice)
		}
	}
	click(commit, "commit=")

	// File Edits is off by default, and holds the project open once on.
	app.eval(`localStorage.setItem("servef-settings", JSON.stringify({notifyEdits: "on"})); setTimeout(() => location.reload(), 0); return true`)
	app.waitEval("the page after its reload", treeShows("README.md"))
	edit := 0
	edits := w.waitNotice("edits", func(notice shownNotice) bool { return ours(notice) && strings.HasSuffix(notice.Title, "changed") }, 15*time.Second, func() {
		edit++
		name := filepath.Join(project, "edit-"+strconv.Itoa(edit)+".md")
		if err := os.WriteFile(name, []byte("# edit\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	click(edits, "review=working")

	publish()
	walkthrough := w.waitNotice("a walkthrough", titled("New walkthrough"), 0, nil)
	if !strings.HasSuffix(walkthrough.Body, " · Notice tour") {
		t.Errorf("walkthrough notice = %+v", walkthrough)
	}
	click(walkthrough, "wt=")
}

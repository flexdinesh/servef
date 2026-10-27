package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/flexdinesh/servef/internal/document"
)

func TestPageReadsDocumentAtCommit(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")
	at := "&at=" + initial.String()

	response, data := requestPageData(t, app, "/api/page?project=.&path=doc.md"+at)
	if response.Code != http.StatusOK || data.Source != "# Before\n" || data.At != initial.String() || !data.HasFile {
		t.Fatalf("doc.md at initial = %d %+v", response.Code, data)
	}
	// A snapshot has no preview content and no navigation; 0004's flags
	// still describe its size.
	if data.PreviewDisabled || data.Content != "" || data.NavigationSupported {
		t.Errorf("snapshot capabilities = preview disabled %v, content %q, navigation %v", data.PreviewDisabled, data.Content, data.NavigationSupported)
	}
	if selectedNode(data.Tree) != "" {
		t.Errorf("snapshot selected tree node %q", selectedNode(data.Tree))
	}
	if _, working := requestPageData(t, app, "/api/page?project=.&path=doc.md"); working.Source != "# Uncommitted\n" || working.At != "" {
		t.Errorf("working doc.md = %q at %q", working.Source, working.At)
	}

	for _, test := range []struct {
		query  string
		source string
	}{
		// Deleted in the working tree, renamed away, or outside the
		// filename scope: commits are repository-scoped.
		{"path=gone.md" + at, "# Gone\n"},
		{"path=old.md" + at, "# Moved\n"},
		{"path=moved.md&at=" + change.String(), "# Moved\n"},
		{"path=scope/x.zzz" + at, "outside the filename scope\n"},
	} {
		if response, data := requestPageData(t, app, "/api/page?project=.&"+test.query); response.Code != http.StatusOK || data.Source != test.source {
			t.Errorf("%s = %d %q", test.query, response.Code, data.Source)
		}
	}

	_, binary := requestPageData(t, app, "/api/page?project=.&path=data.bin"+at)
	if !binary.Binary || binary.Source != "" || binary.FileSize != 11 {
		t.Errorf("binary at commit = %+v", binary)
	}
	_, large := requestPageData(t, app, "/api/page?project=.&path=large.txt"+at)
	if large.TooLarge || !large.HighlightDisabled || len(large.Source) != document.PreviewLimit+1 {
		t.Errorf("large at commit = too large %v, highlight disabled %v, %d bytes", large.TooLarge, large.HighlightDisabled, len(large.Source))
	}
}

func TestPageRejectsUnavailableCommitDocuments(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newTestApp(t, root)

	for _, test := range []struct {
		query   string
		status  int
		message string
	}{
		{"path=added.go&at=" + initial.String(), http.StatusNotFound, commitFileUnavailableMessage},
		{"path=gone.md&at=" + change.String(), http.StatusNotFound, commitFileUnavailableMessage},
		{"path=../doc.md&at=" + initial.String(), http.StatusNotFound, commitFileUnavailableMessage},
		{"path=.git/HEAD&at=" + initial.String(), http.StatusNotFound, commitFileUnavailableMessage},
		{"path=doc.md&at=" + strings.Repeat("1", 40), http.StatusNotFound, commitFileUnavailableMessage},
		{"path=doc.md&at=" + initial.String()[:7], http.StatusBadRequest, invalidCommitMessage},
		{"path=doc.md&at=HEAD", http.StatusBadRequest, invalidCommitMessage},
		{"path=doc.md&at=" + strings.ToUpper(initial.String()), http.StatusBadRequest, invalidCommitMessage},
		{"at=" + initial.String(), http.StatusBadRequest, "No file was selected."},
	} {
		response, data := requestPageData(t, app, "/api/page?project=.&"+test.query)
		if response.Code != test.status || data.Error != test.message {
			t.Errorf("%s = %d %q, want %d %q", test.query, response.Code, data.Error, test.status, test.message)
		}
	}

	folder := t.TempDir()
	writeMarkdown(t, filepath.Join(folder, "doc.md"), "# Folder\n")
	if response, data := requestPageData(t, newTestApp(t, folder), "/api/page?project=.&path=doc.md&at="+initial.String()); response.Code != http.StatusNotFound || data.Error != commitUnavailableMessage {
		t.Errorf("folder at commit = %d %q", response.Code, data.Error)
	}
}

func TestShellPageOpensCommitDocuments(t *testing.T) {
	root, initial, _ := historyRepo(t)
	app := newTestApp(t, root)
	if response := request(t, app, "/view?project=.&path=doc.md&at="+initial.String()); response.Code != http.StatusOK {
		t.Errorf("snapshot view = %d", response.Code)
	}
	if response := request(t, app, "/view?project=.&path=added.go&at="+initial.String()); response.Code != http.StatusNotFound {
		t.Errorf("missing snapshot view = %d", response.Code)
	}
}

func TestFormatReadsDocumentAtCommit(t *testing.T) {
	root := t.TempDir()
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	committed := `{"b":1,"a":[1,2]}`
	writeMarkdown(t, filepath.Join(root, "data.json"), committed)
	writeMarkdown(t, filepath.Join(root, "notes.txt"), "plain\n")
	for _, name := range []string{"data.json", "notes.txt"} {
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	signature := &object.Signature{Name: "servef", Email: "servef@example.com"}
	hash, err := worktree.Commit("initial", &gogit.CommitOptions{Author: signature, Committer: signature})
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdown(t, filepath.Join(root, "data.json"), `{"changed":true}`)
	app := newTestApp(t, root)
	at := "&at=" + hash.String()

	want, err := app.config.Types.Lookup("data.json").Format([]byte(committed))
	if err != nil {
		t.Fatal(err)
	}
	var formatted formatResponse
	if status := requestJSON(t, app, "/api/format?project=.&path=data.json"+at, &formatted); status != http.StatusOK || formatted.Formatted != string(want) {
		t.Errorf("format at commit = %d %q, want %q", status, formatted.Formatted, want)
	}
	for _, test := range []struct {
		query  string
		status int
	}{
		{"path=notes.txt" + at, http.StatusBadRequest},
		{"path=missing.json" + at, http.StatusNotFound},
		{"path=data.json&at=nothex", http.StatusBadRequest},
	} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/format?project=.&"+test.query, &failure); status != test.status {
			t.Errorf("%s = %d %+v, want %d", test.query, status, failure, test.status)
		}
	}
}

// selectedNode returns the path of the tree's selected node, if any.
func selectedNode(nodes []treeNode) string {
	for _, node := range nodes {
		if node.Selected {
			return node.Path
		}
		if path := selectedNode(node.Children); path != "" {
			return path
		}
	}
	return ""
}

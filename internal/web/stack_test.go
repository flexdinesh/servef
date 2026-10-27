package web

import (
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/flexdinesh/servef/internal/git"
)

// stackRepo is historyRepo with a branch above, one commit past feature that
// adds above.md, and twin at feature's tip. It returns the root, initial,
// change, and above's commit.
func stackRepo(t *testing.T) (string, plumbing.Hash, plumbing.Hash, plumbing.Hash) {
	t.Helper()
	root, initial, change := historyRepo(t)
	repo, err := gogit.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	store := func(value object.Object) plumbing.Hash {
		obj := repo.Storer.NewEncodedObject()
		if err := value.Encode(obj); err != nil {
			t.Fatal(err)
		}
		hash, err := repo.Storer.SetEncodedObject(obj)
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	parent, err := repo.CommitObject(change)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := parent.Tree()
	if err != nil {
		t.Fatal(err)
	}
	blob := repo.Storer.NewEncodedObject()
	blob.SetType(plumbing.BlobObject)
	writer, err := blob.Writer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("# Above\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := repo.Storer.SetEncodedObject(blob)
	if err != nil {
		t.Fatal(err)
	}
	// "above.md" sorts before every entry historyRepo commits.
	entries := append([]object.TreeEntry{{Name: "above.md", Mode: filemode.Regular, Hash: content}}, tree.Entries...)
	signature := object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(1700000200, 0)}
	above := store(&object.Commit{
		Author: signature, Committer: signature, Message: "Above\n",
		TreeHash: store(&object.Tree{Entries: entries}), ParentHashes: []plumbing.Hash{change},
	})
	for name, hash := range map[string]plumbing.Hash{"refs/heads/above": above, "refs/heads/twin": change, "refs/remotes/origin/above": above} {
		if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(name), hash)); err != nil {
			t.Fatal(err)
		}
	}
	return root, initial, change, above
}

func TestBranchStackListsLayers(t *testing.T) {
	root, initial, change, above := stackRepo(t)
	app := newScopedApp(t, root, "*.md")

	response := request(t, app, "/api/branch-stack?project=.")
	want := `{"repository":true,"branch":"feature","base":"master","mergeBase":"` + initial.String() + `","head":"` + change.String() + `","layers":[` +
		`{"name":"above","aliases":[],"head":"` + above.String() + `","parent":"feature","ahead":1,"current":false},` +
		`{"name":"feature","aliases":["twin"],"head":"` + change.String() + `","parent":"master","ahead":1,"current":true}],"truncated":false}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("branch stack = %d %s\nwant %s", response.Code, response.Body.String(), want)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}

	// Against twin, nothing is ahead: no layers and no merge base.
	body := request(t, app, "/api/branch-stack?project=.&base=twin").Body.String()
	if want := `{"repository":true,"branch":"feature","base":"twin","mergeBase":"","head":"` + change.String() + `","layers":[],"truncated":false}`; strings.TrimSpace(body) != want {
		t.Fatalf("base=twin = %s", body)
	}

	for _, test := range []struct {
		target string
		want   errorResponse
	}{
		{"/api/branch-stack?project=.&base=missing", errorResponse{Error: unknownBaseMessage, Code: unknownBaseCode}},
		{"/api/branch-stack?project=.&base=origin/HEAD", errorResponse{Error: unknownBaseMessage, Code: unknownBaseCode}},
		{"/api/branch-stack", errorResponse{Error: noProjectMessage}},
	} {
		var failure errorResponse
		if status := requestJSON(t, app, test.target, &failure); status != http.StatusBadRequest || failure != test.want {
			t.Errorf("%s = %d %+v", test.target, status, failure)
		}
	}
}

func TestBranchStackInFolders(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "# Folder")
	app := newTestApp(t, root)

	response := request(t, app, "/api/branch-stack?project=.")
	want := `{"repository":false,"branch":"","base":"","mergeBase":"","head":"","layers":[],"truncated":false}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("folder branch stack = %d %s", response.Code, response.Body.String())
	}
}

func TestHeadParameterChoosesTheHeadBranch(t *testing.T) {
	root, initial, change, above := stackRepo(t)
	app := newScopedApp(t, root, "*.md")
	markdown := app.config.Types.Lookup("above.md").Type()

	// A layer against its parent branch lists only its own changes.
	var layer branchFilesResponse
	if status := requestJSON(t, app, "/api/branch-files?project=.&base=feature&head=above", &layer); status != http.StatusOK {
		t.Fatalf("base=feature&head=above status = %d", status)
	}
	if want := (branchFilesResponse{Repository: true, Branch: "above", Base: "feature", MergeBase: change.String(), Head: above.String(), Ahead: 1, Files: []git.Change{{Path: "above.md", Status: git.Added}}, Warnings: []string{}}); !reflect.DeepEqual(layer, want) {
		t.Fatalf("base=feature&head=above = %+v\nwant %+v", layer, want)
	}
	// A layer against the base lists the whole stack below it.
	var full branchFilesResponse
	if status := requestJSON(t, app, "/api/branch-files?project=.&head=above", &full); status != http.StatusOK || full.Branch != "above" || full.Base != "master" || full.MergeBase != initial.String() || full.Head != above.String() || full.Ahead != 2 || len(full.Files) != 8 {
		t.Fatalf("head=above = %d %+v", status, full)
	}
	// Naming HEAD's branch answers as HEAD does.
	var byDefault, named branchFilesResponse
	requestJSON(t, app, "/api/branch-files?project=.", &byDefault)
	if status := requestJSON(t, app, "/api/branch-files?project=.&head=feature", &named); status != http.StatusOK || !reflect.DeepEqual(named, byDefault) {
		t.Fatalf("head=feature = %d %+v, default %+v", status, named, byDefault)
	}

	if got, want := diff(t, app, "branch=1&base=feature&head=above&path=above.md"), (diffResponse{Path: "above.md", Status: git.Added, DocumentType: markdown, Modified: text("# Above\n")}); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff head=above = %s", describeDiff(got))
	}
	var unchanged errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&base=feature&head=above&path=doc.md", &unchanged); status != http.StatusNotFound || unchanged.Code != unchangedCode {
		t.Fatalf("diff head=above doc.md = %d %+v", status, unchanged)
	}
	assertStats(t, "stats head=above", compareStats(t, app, "compare=branch&base=feature&head=above"), compareStatsResponse{Files: []fileStats{
		counted("above.md", 1, 0, blobHash("# Above\n")),
	}})

	for _, target := range []string{
		"/api/branch-files?project=.&head=missing",
		"/api/branch-files?project=.&head=origin/above",
		"/api/branch-files?project=.&head=refs/heads/above",
		"/api/diff?project=.&branch=1&head=missing&path=doc.md",
		"/api/diff?project=.&branch=1&head=missing&path=../doc.md",
		"/api/compare-stats?project=.&compare=branch&head=missing",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, target, &failure); status != http.StatusBadRequest || failure != (errorResponse{Error: unknownHeadMessage, Code: unknownHeadCode}) {
			t.Errorf("%s = %d %+v", target, status, failure)
		}
	}
	for _, target := range []string{
		"/api/diff?project=.&head=above&path=doc.md",
		"/api/diff?project=.&commit=" + change.String() + "&head=above&path=doc.md",
		"/api/compare-stats?project=.&compare=working&head=above",
		"/api/compare-stats?project=.&compare=commit&commit=" + change.String() + "&head=above",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, target, &failure); status != http.StatusBadRequest || failure.Error == "" || failure.Code != "" {
			t.Errorf("%s = %d %+v", target, status, failure)
		}
	}
}

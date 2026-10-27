package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/walkthrough"
)

// walkthroughApp serves root with a private walkthrough store and records
// the URLs it is asked to open.
func walkthroughApp(t *testing.T, root string) (*App, chan string) {
	t.Helper()
	opened := make(chan string, 4)
	app, err := New(Config{
		Roots:                []string{root},
		Depth:                5,
		Exclusions:           []string{".git", "node_modules", "vendor"},
		WalkthroughDirectory: filepath.Join(t.TempDir(), "walkthroughs"),
		OpenBrowser:          func(target string) error { opened <- target; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return app, opened
}

func publish(t *testing.T, app *App, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/walkthroughs", strings.NewReader(body))
	req.Host = "127.0.0.1:7971"
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, req)
	return response
}

func draftJSON(t *testing.T, draft walkthrough.Draft) string {
	t.Helper()
	encoded, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func decodeError(t *testing.T, response *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body: %v\n%s", err, response.Body.String())
	}
	return body
}

func TestPublishChecksTargetsAndServesRenderedWalkthrough(t *testing.T) {
	root, initial, change := historyRepo(t)
	app, opened := walkthroughApp(t, root)
	lines := func(start, end int) *walkthrough.Lines { return &walkthrough.Lines{start, end} }
	draft := walkthrough.Draft{
		Project: root,
		ID:      "tour",
		Title:   "Tour",
		Summary: "Start **here**.",
		Open:    true,
		Steps: []walkthrough.Step{
			{Title: "Intro", Body: "See [the doc](doc.md) <script>alert(1)</script>"},
			{Title: "Working file", File: &walkthrough.FileTarget{Path: "doc.md", Lines: lines(1, 1)}},
			{Title: "File at a commit", File: &walkthrough.FileTarget{Path: "old.md", Lines: lines(1, 1), At: initial.String()[:7]}},
			{Title: "Whole file", File: &walkthrough.FileTarget{Path: "same.md", At: initial.String()}},
			{Title: "Uncommitted", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "working"}, Path: "doc.md", Lines: lines(1, 1)}},
			{Title: "Branch old side", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "branch", Base: "master"}, Path: "doc.md", Side: "old", Lines: lines(1, 1)}},
			{Title: "Rename by old path", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "commit", Commit: change.String()[:8]}, Path: "old.md"}},
			{Title: "Commit lines", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "commit", Commit: change.String()}, Path: "added.go", Lines: lines(1, 1)}},
			{Title: "Range", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "range", From: change.String()[:6], To: change.String()[:6]}}},
			{Title: "Agent expect kept", File: &walkthrough.FileTarget{Path: "doc.md", Lines: lines(1, 1), Expect: "# Something else"}},
		},
	}
	response := publish(t, app, draftJSON(t, draft), map[string]string{"Origin": "http://127.0.0.1:7971"})
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d\n%s", response.Code, response.Body.String())
	}
	var published publishResponse
	if err := json.Unmarshal(response.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	wantURL := "http://127.0.0.1:7971/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode()
	if published.ID != "tour" || !published.Created || published.URL != wantURL {
		t.Fatalf("published = %+v, want URL %s", published, wantURL)
	}
	select {
	case target := <-opened:
		if target != wantURL {
			t.Errorf("opened %s", target)
		}
	case <-time.After(5 * time.Second):
		t.Error("open: true did not open the browser")
	}

	var got walkthroughResponse
	if status := requestJSON(t, app, "/api/walkthrough?project=.&id=tour", &got); status != http.StatusOK {
		t.Fatalf("get status = %d", status)
	}
	steps := got.Steps
	if got.Project != root || got.Title != "Tour" || !strings.Contains(got.SummaryHTML, "<strong>here</strong>") {
		t.Errorf("walkthrough = %+v", got)
	}
	assertContains(t, steps[0].BodyHTML, "&lt;script&gt;", `href="/view?project=`+url.QueryEscape(root)+`&amp;path=doc.md"`)
	if strings.Contains(steps[0].BodyHTML, "<script>") {
		t.Errorf("narration HTML is not escaped: %s", steps[0].BodyHTML)
	}
	for i, want := range map[int]string{1: "# Uncommitted", 4: "# Uncommitted", 5: "# Before", 9: "# Something else"} {
		var expect string
		if steps[i].File != nil {
			expect = steps[i].File.Expect
		} else {
			expect = steps[i].Diff.Expect
		}
		if expect != want {
			t.Errorf("steps[%d] expect = %q, want %q", i, expect, want)
		}
	}
	if steps[2].File.At != initial.String() || steps[2].File.Expect != "" {
		t.Errorf("commit file = %+v; want the full hash and no recorded expect", steps[2].File)
	}
	if diff := steps[6].Diff; diff.Path != "moved.md" || diff.Compare.Commit != change.String() {
		t.Errorf("rename step = %+v; want the new path and full hash", diff)
	}
	if diff := steps[7].Diff; diff.Expect != "" {
		t.Errorf("commit diff recorded expect %q; commits never drift", diff.Expect)
	}
	if compare := steps[8].Diff.Compare; compare.From != change.String() || compare.To != change.String() {
		t.Errorf("range = %+v", compare)
	}

	// Republishing replaces and is 200.
	draft.Open = false
	draft.Title = "Tour again"
	if response := publish(t, app, draftJSON(t, draft), nil); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"created":false`) {
		t.Fatalf("republish = %d %s", response.Code, response.Body.String())
	}
	var list []walkthrough.Summary
	if status := requestJSON(t, app, "/api/walkthroughs?project=.", &list); status != http.StatusOK || len(list) != 1 || list[0].Title != "Tour again" || list[0].StepCount != len(draft.Steps) {
		t.Fatalf("list = %d %+v", status, list)
	}
}

func TestPublishReportsEveryTargetProblem(t *testing.T) {
	root, _, change := historyRepo(t)
	app, _ := walkthroughApp(t, root)
	lines := func(start, end int) *walkthrough.Lines { return &walkthrough.Lines{start, end} }
	response := publish(t, app, draftJSON(t, walkthrough.Draft{
		Project: root,
		Title:   "Broken",
		Steps: []walkthrough.Step{
			{Title: "missing file", File: &walkthrough.FileTarget{Path: "nope.md"}},
			{Title: "past the end", File: &walkthrough.FileTarget{Path: "doc.md", Lines: lines(1, 9)}},
			{Title: "binary", File: &walkthrough.FileTarget{Path: "data.bin", Lines: lines(1, 1)}},
			{Title: "unknown commit", File: &walkthrough.FileTarget{Path: "doc.md", At: "0000000"}},
			{Title: "not at commit", File: &walkthrough.FileTarget{Path: "added.go", At: change.String()[:7]}},
			{Title: "unchanged", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "working"}, Path: "added.go"}},
			{Title: "unknown base", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "branch", Base: "nope"}}},
			{Title: "added has no old side", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "commit", Commit: change.String()}, Path: "added.go", Side: "old", Lines: lines(1, 1)}},
			{Title: "deleted has no new side", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "commit", Commit: change.String()}, Path: "gone.md", Lines: lines(1, 1)}},
			{Title: "too large", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "commit", Commit: change.String()}, Path: "large.txt", Lines: lines(1, 1)}},
		},
	}), nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d\n%s", response.Code, response.Body.String())
	}
	body := decodeError(t, response)
	if body.Code != "invalidWalkthrough" {
		t.Errorf("code = %q", body.Code)
	}
	for _, want := range []string{
		"steps[0].file.path: is not a file servef shows",
		"steps[1].file.lines: end 9 is past the file's last line, 1",
		"steps[2].file.lines: can't be shown",
		"steps[3].file.at: no commit matches 0000000",
		"steps[5].diff.path: is not changed in this compare",
		"steps[6].diff.compare.base: is not a branch servef lists",
		"steps[7].diff.side: is old, but the file is added",
		"steps[8].diff.side: is new, but the file is deleted",
		"steps[9].diff.lines: can't be shown",
	} {
		if !strings.Contains(body.Error, want) {
			t.Errorf("error lacks %q:\n%s", want, body.Error)
		}
	}
	// added.go exists at the change commit, so step 4 has no problem.
	if strings.Contains(body.Error, "steps[4]") {
		t.Errorf("unexpected steps[4] problem:\n%s", body.Error)
	}
	var list []walkthrough.Summary
	if requestJSON(t, app, "/api/walkthroughs?project=.", &list); len(list) != 0 {
		t.Errorf("rejected walkthrough stored: %+v", list)
	}
}

func TestPublishRejectsProjectsOutsideTheRootsAndGitStepsWithoutGit(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "a.md"), "# A\n")
	app, _ := walkthroughApp(t, root)
	response := publish(t, app, draftJSON(t, walkthrough.Draft{Project: t.TempDir(), Title: "t", Steps: []walkthrough.Step{{Title: "s"}}}), nil)
	if response.Code != http.StatusBadRequest || !strings.Contains(decodeError(t, response).Error, "project: ") {
		t.Errorf("outside project = %d %s", response.Code, response.Body.String())
	}
	response = publish(t, app, draftJSON(t, walkthrough.Draft{Project: root, Title: "t", Steps: []walkthrough.Step{
		{Title: "s", Diff: &walkthrough.DiffTarget{Compare: walkthrough.Compare{Kind: "working"}}},
		{Title: "s", File: &walkthrough.FileTarget{Path: "a.md", At: "abcdef0"}},
	}}), nil)
	body := decodeError(t, response)
	for _, want := range []string{"steps[0].diff.compare: needs a git repository", "steps[1].file.at: needs a git repository"} {
		if !strings.Contains(body.Error, want) {
			t.Errorf("error lacks %q: %s", want, body.Error)
		}
	}
}

func TestPublishGuardsAgainstOtherSitesAndBadBodies(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "a.md"), "# A\n")
	app, _ := walkthroughApp(t, root)
	valid := draftJSON(t, walkthrough.Draft{Project: root, Title: "t", Steps: []walkthrough.Step{{Title: "s"}}})

	for _, test := range []struct {
		name    string
		body    string
		headers map[string]string
		status  int
		code    string
	}{
		{"text/plain", valid, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType, "unsupportedMediaType"},
		{"form", valid, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusUnsupportedMediaType, "unsupportedMediaType"},
		{"foreign origin", valid, map[string]string{"Origin": "https://attacker.example"}, http.StatusForbidden, "foreignOrigin"},
		{"null origin", valid, map[string]string{"Origin": "null"}, http.StatusForbidden, "foreignOrigin"},
		{"other port", valid, map[string]string{"Origin": "http://127.0.0.1:8080"}, http.StatusForbidden, "foreignOrigin"},
		{"too large", `{"project":"` + strings.Repeat("a", walkthrough.MaxRequestBytes) + `"}`, nil, http.StatusRequestEntityTooLarge, "tooLarge"},
		{"invalid JSON", `{"project":`, nil, http.StatusBadRequest, "invalidWalkthrough"},
		{"unknown field", `{"project":"/","title":"t","steps":[],"extra":1}`, nil, http.StatusBadRequest, "invalidWalkthrough"},
	} {
		response := publish(t, app, test.body, test.headers)
		if response.Code != test.status || decodeError(t, response).Code != test.code {
			t.Errorf("%s = %d %s; want %d %s", test.name, response.Code, response.Body.String(), test.status, test.code)
		}
	}
	for _, headers := range []map[string]string{nil, {"Origin": "http://127.0.0.1:7971"}, {"Content-Type": "application/json; charset=utf-8"}} {
		if response := publish(t, app, valid, headers); response.Code != http.StatusCreated {
			t.Errorf("headers %v = %d %s", headers, response.Code, response.Body.String())
		}
	}
}

func TestWalkthroughsListGetAndDelete(t *testing.T) {
	root := t.TempDir()
	app, opened := walkthroughApp(t, root)
	for _, id := range []string{"one", "two"} {
		if response := publish(t, app, draftJSON(t, walkthrough.Draft{Project: root, ID: id, Title: id, Steps: []walkthrough.Step{{Title: "s"}}}), nil); response.Code != http.StatusCreated {
			t.Fatalf("publish %s = %d %s", id, response.Code, response.Body.String())
		}
	}
	select {
	case target := <-opened:
		t.Errorf("opened %s without open: true", target)
	default:
	}
	var list []walkthrough.Summary
	if requestJSON(t, app, "/api/walkthroughs?project=.", &list); len(list) != 2 || list[0].ID != "two" {
		t.Fatalf("list = %+v", list)
	}

	remove := func(target string, origin string) *httptest.ResponseRecorder {
		target = rootProject.ReplaceAllString(target, "project="+url.QueryEscape(app.roots[0])+"$1")
		req := localRequest(http.MethodDelete, target)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		response := httptest.NewRecorder()
		app.ServeHTTP(response, req)
		return response
	}
	if response := remove("/api/walkthroughs?project=.&id=one", "https://attacker.example"); response.Code != http.StatusForbidden {
		t.Errorf("foreign delete = %d", response.Code)
	}
	if response := remove("/api/walkthroughs?project=.&id=one", "http://127.0.0.1:7971"); response.Code != http.StatusNoContent {
		t.Errorf("delete = %d %s", response.Code, response.Body.String())
	}
	if response := remove("/api/walkthroughs?project=.&id=one", ""); response.Code != http.StatusNotFound || decodeError(t, response).Code != "walkthroughUnavailable" {
		t.Errorf("second delete = %d %s", response.Code, response.Body.String())
	}
	if response := request(t, app, "/api/walkthrough?project=.&id=one"); response.Code != http.StatusNotFound {
		t.Errorf("get deleted = %d", response.Code)
	}
	if response := request(t, app, "/api/walkthroughs?project="+url.QueryEscape(t.TempDir())); response.Code == http.StatusOK {
		t.Errorf("list outside the roots = %d", response.Code)
	}
}

func TestWalkthroughShellPageAndSchema(t *testing.T) {
	root := t.TempDir()
	app, _ := walkthroughApp(t, root)
	if response := request(t, app, "/view?project=.&wt=tour"); response.Code != http.StatusOK {
		t.Errorf("walkthrough shell page = %d", response.Code)
	}
	if response := request(t, app, "/view?project=.&wt="); response.Code != http.StatusBadRequest {
		t.Errorf("empty wt shell page = %d, want a file required", response.Code)
	}
	response := request(t, app, "/api/walkthrough-schema")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/schema+json" || !bytes.Equal(response.Body.Bytes(), walkthrough.Schema) {
		t.Errorf("schema = %d %q", response.Code, response.Header().Get("Content-Type"))
	}
}

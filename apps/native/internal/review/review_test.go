package review

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/packages/mygo/diffs"
	"github.com/flexdinesh/servef/pkg/contract"
)

// rows renders a layout's unified rows as "-old", "+new", " both", "~n"
// collapsed lines, or a backslash for no newline, from its sides' text.
func rows(layout Layout, full bool) []string {
	var out []string
	d := layout.Diff
	for _, row := range d.Layout(diffs.Unified, nil, full) {
		switch {
		case row.Kind == diffs.SeparatorRow:
			out = append(out, fmt.Sprintf("~%d", row.Hidden))
		case row.Kind == diffs.NoNewlineRow:
			out = append(out, `\`)
		case row.Old.Kind == diffs.Unchanged:
			out = append(out, " "+d.Text(diffs.New, row.New.Line))
		case row.Old.Kind == diffs.Deleted:
			out = append(out, "-"+d.Text(diffs.Old, row.Old.Line))
		default:
			out = append(out, "+"+d.Text(diffs.New, row.New.Line))
		}
	}
	return out
}

// The hunks, rows, and gaps are the server's line diff for the fixture's
// sides (testdata/api/default/diff-hunks.json, checked against the server).
func TestLayHunksFromTheServerLineDiff(t *testing.T) {
	layout := Lay(fakeservef.Diff(t, "diff-hunks.json"))
	if layout.Kind != Text || layout.Additions != 4 || layout.Deletions != 4 || layout.Language != "text" {
		t.Fatalf("layout: %+v", layout)
	}
	d := layout.Diff
	if len(d.Hunks) != 3 || len(d.Gaps) != 2 || len(d.Old) != 40 || len(d.New) != 40 {
		t.Fatalf("hunks %d, gaps %d, sides %d %d", len(d.Hunks), len(d.Gaps), len(d.Old), len(d.New))
	}
	got := rows(layout, false)
	if want := []string{" line 1", "-line 2", "+line 2 changed", " line 3", " line 4", " line 5", " line 6", "~9"}; !slices.Equal(got[:8], want) {
		t.Fatalf("first hunk %q, want %q", got[:8], want)
	}
	// The second hunk's deletion, then lines numbered one less on the new
	// side; the one-line gap before the last hunk always shows.
	second := d.Hunks[1]
	if second.Blocks[1] != (diffs.Block{Kind: diffs.Change, OldStart: 20, OldCount: 1, NewStart: 20, NewCount: 0}) {
		t.Fatalf("second hunk blocks %+v", second.Blocks)
	}
	if slices.Contains(got, "~1") || !slices.Contains(got, " line 25") {
		t.Fatalf("one-line gap %q", got)
	}
	// The last line lost its final newline; whitespace-only changes show.
	if want := []string{"-line 30", "+  line 30"}; !slices.Contains(got, want[0]) || !slices.Contains(got, want[1]) {
		t.Fatalf("rows %q", got)
	}
	if want := []string{"+inserted", " line 39", "-line 40", "+line 40", `\`}; !slices.Equal(got[len(got)-5:], want) {
		t.Fatalf("last hunk end %q", got[len(got)-5:])
	}
	// Full file: every line, no separator.
	full := rows(layout, true)
	if slices.ContainsFunc(full, func(r string) bool { return strings.HasPrefix(r, "~") }) || !slices.Contains(full, " line 12") {
		t.Fatalf("full %q", full)
	}
}

func TestLayKeepsTheLanguage(t *testing.T) {
	if layout := Lay(fakeservef.Diff(t, "diff-modified.json")); layout.Language != "go" || layout.Diff.Hunks[0].OldStart != 3 {
		t.Fatalf("layout %+v", layout)
	}
}

func TestLayKinds(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		kind    Kind
		message string
	}{
		{"diff-binary.json", Binary, "Binary file changed · 12.0 KB → 14.0 KB"},
		{"diff-too-large.json", TooLarge, "Large file (6.3 MB): diff is off."},
		{"diff-renamed.json", Unchanged, "File contents are unchanged."},
		{"diff-added.json", Text, ""},
		{"diff-deleted.json", Text, ""},
		{"diff-untracked.json", Text, ""},
	} {
		layout := Lay(fakeservef.Diff(t, tc.fixture))
		if layout.Kind != tc.kind || layout.Message != tc.message {
			t.Errorf("%s: kind %d %q, want %d %q", tc.fixture, layout.Kind, layout.Message, tc.kind, tc.message)
		}
	}
	added := Lay(fakeservef.Diff(t, "diff-added.json")).Diff
	if len(added.Old) != 0 || len(added.New) != 2 || added.Hunks[0].Blocks[0].Kind != diffs.Change {
		t.Fatalf("added: %+v", added)
	}
	deleted := Lay(fakeservef.Diff(t, "diff-deleted.json")).Diff
	if len(deleted.Old) != 8 || len(deleted.New) != 0 {
		t.Fatalf("deleted: %+v", deleted)
	}
}

func TestLayWithoutLineDiffOrHunks(t *testing.T) {
	diff := fakeservef.Diff(t, "diff-modified.json")
	diff.LineDiff = nil
	if layout := Lay(diff); layout.Kind != Unavailable || layout.Message != "Couldn't compute this diff." {
		t.Fatalf("no line diff: %+v", layout)
	}
	diff.LineDiff = &contract.LineDiff{IgnoreWhitespace: true}
	if layout := Lay(diff); layout.Kind != Unchanged || layout.Message != "No changes apart from whitespace." {
		t.Fatalf("whitespace only: %+v", layout)
	}
	added := contract.DiffSide{Size: 4, Binary: true}
	if got := binaryLabel(nil, &added); got != "Binary file added · 4 B" {
		t.Fatalf("added binary: %q", got)
	}
	if got := binaryLabel(&added, nil); got != "Binary file deleted · 4 B" {
		t.Fatalf("deleted binary: %q", got)
	}
}

// Rows never index past a side, even if a line diff disagrees with it.
func TestLayToleratesShortSides(t *testing.T) {
	text := "a\r\nb\r\n"
	diff := contract.DiffResponse{
		Status:   contract.ChangeModified,
		Original: &contract.DiffSide{Content: &text}, Modified: &contract.DiffSide{Content: &text},
		LineDiff: &contract.LineDiff{Hunks: []contract.DiffHunk{{OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3, Blocks: []contract.DiffBlock{{Kind: contract.DiffBlockContext, OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3}}}}},
	}
	if got := rows(Lay(diff), false); !slices.Equal(got, []string{" a", " b", " "}) {
		t.Fatalf("rows %q", got)
	}
}

func TestLabels(t *testing.T) {
	renamed := contract.ChangedFile{Path: "b.md", OldPath: "a.md", Status: contract.ChangeRenamed}
	if Label(renamed) != "a.md → b.md" || Letter(renamed.Status) != "R" || StatusName(renamed.Status) != "Renamed" {
		t.Fatal("renamed labels")
	}
	for status, letter := range map[contract.ChangeStatus]string{contract.ChangeModified: "M", contract.ChangeAdded: "A", contract.ChangeUntracked: "U", contract.ChangeDeleted: "D"} {
		if Letter(status) != letter {
			t.Errorf("%s: %s", status, Letter(status))
		}
	}
	if ChangeCount(1) != "1 change" || ChangeCount(3) != "3 changes" {
		t.Fatal(ChangeCount(1), ChangeCount(3))
	}
	if Counts(3, 1) != "+3 −1" {
		t.Fatal(Counts(3, 1))
	}
}
